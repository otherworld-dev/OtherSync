package main

import (
	"os"
	"testing"

	"github.com/otherworld/nimbo/internal/account"
	"github.com/otherworld/nimbo/internal/brand"
	"github.com/otherworld/nimbo/internal/config"
)

var renamed = brand.Brand{Name: "OtherSync", FormerName: "Nimbo", AppID: "Nimbo"}

func renameDirs(t *testing.T) config.Dirs {
	t.Helper()
	return config.Dirs{Config: t.TempDir(), Data: t.TempDir()}
}

func seedAccounts(t *testing.T, d config.Dirs, accs ...account.Account) {
	t.Helper()
	if err := account.Update(d.AccountsFile(), func(st *account.Store) error {
		for _, a := range accs {
			st.Upsert(a)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// An install that ran Nimbo is told once, and its accounts keep "Nimbo -
// <login>" as their sidebar name; a chosen name or an earlier record is left
// alone. The next launch does nothing.
func TestMigrateRenameExistingInstall(t *testing.T) {
	d := renameDirs(t)
	seedAccounts(t, d,
		account.Account{ID: "a", ServerURL: "https://one.example.com", LoginName: "adam"},
		account.Account{ID: "b", ServerURL: "https://two.example.com", LoginName: "bob", Label: "Work"},
	)
	show, err := migrateRename(d, renamed)
	if err != nil || !show {
		t.Fatalf("first launch: show=%v err=%v, want the notice", show, err)
	}
	st, _ := account.LoadStore(d.AccountsFile())
	for _, id := range []string{"a", "b"} {
		if acc, _ := st.Find(id); acc.LabelBrand != "Nimbo" {
			t.Errorf("account %s LabelBrand = %q, want Nimbo", id, acc.LabelBrand)
		}
	}
	if acc, _ := st.Find("b"); acc.Label != "Work" {
		t.Errorf("chosen name changed: %q", acc.Label)
	}
	if acc, _ := st.Find("a"); sidebarLabel(acc, st.Accounts) != "Nimbo - adam" {
		t.Errorf("sidebar label changed: %q", sidebarLabel(acc, st.Accounts))
	}

	// An account added after the rename gets the new name, and is not marked
	// by a later launch.
	seedAccounts(t, d, account.Account{ID: "c", ServerURL: "https://one.example.com", LoginName: "cat"})
	show, err = migrateRename(d, renamed)
	if err != nil || show {
		t.Fatalf("second launch: show=%v err=%v, want nothing", show, err)
	}
	st, _ = account.LoadStore(d.AccountsFile())
	if acc, _ := st.Find("c"); acc.LabelBrand != "" || sidebarLabel(acc, st.Accounts) != "OtherSync - cat" {
		t.Errorf("new account: LabelBrand %q, label %q", acc.LabelBrand, sidebarLabel(acc, st.Accounts))
	}
}

// Settings without accounts (signed out, or never signed in) is still an
// install that ran Nimbo.
func TestMigrateRenameSettingsOnly(t *testing.T) {
	d := renameDirs(t)
	if err := d.UpdateSettings(func(s *config.Settings) { s.Theme = "dark" }); err != nil {
		t.Fatal(err)
	}
	if show, err := migrateRename(d, renamed); err != nil || !show {
		t.Fatalf("show=%v err=%v, want the notice", show, err)
	}
	if _, err := os.Stat(d.AccountsFile()); !os.IsNotExist(err) {
		t.Errorf("an accounts file was created: %v", err)
	}
}

// A fresh install is never told about a name it never knew, and isn't told
// later either, once it has settings and accounts of its own.
func TestMigrateRenameFreshInstall(t *testing.T) {
	d := renameDirs(t)
	if show, err := migrateRename(d, renamed); err != nil || show {
		t.Fatalf("fresh: show=%v err=%v, want nothing", show, err)
	}
	seedAccounts(t, d, account.Account{ID: "a", ServerURL: "https://one.example.com", LoginName: "adam"})
	if show, err := migrateRename(d, renamed); err != nil || show {
		t.Fatalf("after sign-in: show=%v err=%v, want nothing", show, err)
	}
	st, _ := account.LoadStore(d.AccountsFile())
	if acc, _ := st.Find("a"); acc.LabelBrand != "" {
		t.Errorf("fresh install's account marked: %q", acc.LabelBrand)
	}
}

// A brand that was never renamed (a white-label build) touches nothing.
func TestMigrateRenameNotRenamed(t *testing.T) {
	d := renameDirs(t)
	seedAccounts(t, d, account.Account{ID: "a", ServerURL: "https://one.example.com", LoginName: "adam"})
	if show, err := migrateRename(d, brand.Brand{Name: "AcmeCloud Sync", AppID: "AcmeCloud"}); err != nil || show {
		t.Fatalf("show=%v err=%v", show, err)
	}
	if _, err := os.Stat(d.SettingsFile()); !os.IsNotExist(err) {
		t.Errorf("settings written: %v", err)
	}
}

// An accounts file that can't be read is not taken as "no accounts": nothing
// is recorded, so the next launch tries again.
func TestMigrateRenameUnreadableAccounts(t *testing.T) {
	d := renameDirs(t)
	if err := os.WriteFile(d.AccountsFile(), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := migrateRename(d, renamed); err == nil {
		t.Fatal("want an error")
	}
	if s, _ := d.LoadSettings(); s.RenamedFrom != "" {
		t.Errorf("recorded as done: %q", s.RenamedFrom)
	}
}

func TestRenameNoticeText(t *testing.T) {
	if got, want := renameNotice(renamed), "Nimbo is now OtherSync. Same app, nothing else has changed."; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A settings file that can't be read must not skip the account marking: the
// accounts mount right after, and an unmarked one would be re-registered as
// "OtherSync - <login>" now and back to "Nimbo - <login>" once the settings
// read again. Nothing is recorded as done and no notice is due, so a later
// launch with readable settings finishes the job.
func TestMigrateRenameCorruptSettings(t *testing.T) {
	d := renameDirs(t)
	seedAccounts(t, d, account.Account{ID: "a", ServerURL: "https://one.example.com", LoginName: "adam"})
	if err := os.WriteFile(d.SettingsFile(), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	show, err := migrateRename(d, renamed)
	if err == nil || show {
		t.Fatalf("show=%v err=%v, want an error and no notice", show, err)
	}
	st, _ := account.LoadStore(d.AccountsFile())
	if acc, _ := st.Find("a"); acc.LabelBrand != "Nimbo" || sidebarLabel(acc, st.Accounts) != "Nimbo - adam" {
		t.Fatalf("account not marked: LabelBrand %q", acc.LabelBrand)
	}

	// Settings readable again: the notice is shown then, once.
	if err := os.Remove(d.SettingsFile()); err != nil {
		t.Fatal(err)
	}
	if show, err := migrateRename(d, renamed); err != nil || !show {
		t.Fatalf("after repair: show=%v err=%v, want the notice", show, err)
	}
}
