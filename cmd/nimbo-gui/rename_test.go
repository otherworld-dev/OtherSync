package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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

// An install that ran Nimbo is told once; the next launch does nothing. Its
// accounts are left as they are: their automatic sidebar names follow the
// product name, so they say OtherSync now, and a chosen name stays.
func TestMigrateRenameExistingInstall(t *testing.T) {
	d := renameDirs(t)
	seedAccounts(t, d,
		account.Account{ID: "a", ServerURL: "https://one.example.com", LoginName: "adam"},
		account.Account{ID: "b", ServerURL: "https://two.example.com", LoginName: "bob", Label: "Work"},
	)
	before, err := os.ReadFile(d.AccountsFile())
	if err != nil {
		t.Fatal(err)
	}
	show, err := migrateRename(d, renamed)
	if err != nil || !show {
		t.Fatalf("first launch: show=%v err=%v, want the notice", show, err)
	}
	if after, _ := os.ReadFile(d.AccountsFile()); string(after) != string(before) {
		t.Errorf("accounts file changed: %s, was: %s", after, before)
	}
	if s, _ := d.LoadSettings(); s.RenamedFrom != "Nimbo" {
		t.Errorf("RenamedFrom = %q, want Nimbo", s.RenamedFrom)
	}
	st, _ := account.LoadStore(d.AccountsFile())
	for id, want := range map[string]string{"a": "OtherSync - adam", "b": "Work"} {
		if acc, _ := st.Find(id); sidebarLabel(acc, st.Accounts) != want {
			t.Errorf("account %s label %q, want %q", id, sidebarLabel(acc, st.Accounts), want)
		}
	}

	if show, err = migrateRename(d, renamed); err != nil || show {
		t.Fatalf("second launch: show=%v err=%v, want nothing", show, err)
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

// A settings file that can't be read can't say whether the rename was already
// handled, so nothing is recorded as done and no notice is due; a later launch
// with readable settings finishes the job (once).
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
	if data, _ := os.ReadFile(d.SettingsFile()); string(data) != "{not json" {
		t.Errorf("the unreadable settings file was overwritten: %q", data)
	}

	// Settings readable again: the notice is shown then, once.
	if err := os.Remove(d.SettingsFile()); err != nil {
		t.Fatal(err)
	}
	if show, err := migrateRename(d, renamed); err != nil || !show {
		t.Fatalf("after repair: show=%v err=%v, want the notice", show, err)
	}
}

// The update helper and the sidebar helper used to leave nimbo-named files in
// the home folder. A renamed build removes the stale ones, but leaves anything
// the old version's update helper may still be writing (recent files), and
// never touches a file that isn't one of its own.
func TestRemoveFormerHelperFiles(t *testing.T) {
	home := t.TempDir()
	now := time.Now()
	old := now.Add(-2 * time.Hour)
	write := func(name string, mod time.Time) string {
		p := filepath.Join(home, name)
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mod, mod); err != nil {
			t.Fatal(err)
		}
		return p
	}
	stale := []string{
		write("nimbo-update.ps1", old), write("nimbo-update.vbs", old),
		write("nimbo-update-task.xml", old), write("nimbo-sidebar.log", old),
	}
	fresh := write("nimbo-update.log", now) // the old helper may still be writing it
	other := write("nimbo-notes.txt", old)
	ours := write("othersync-update.log", old)

	removeFormerHelperFiles(home, renamed, now)

	for _, p := range stale {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s should have been removed (err %v)", filepath.Base(p), err)
		}
	}
	for _, p := range []string{fresh, other, ours} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s should have been kept: %v", filepath.Base(p), err)
		}
	}
}

// A build that hasn't been renamed has no former files to remove.
func TestRemoveFormerHelperFilesNotRenamed(t *testing.T) {
	home := t.TempDir()
	p := filepath.Join(home, "othersync-update.log")
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	_ = os.Chtimes(p, old, old)
	removeFormerHelperFiles(home, brand.Brand{Name: "OtherSync", AppID: "Nimbo"}, time.Now())
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("kept file removed: %v", err)
	}
}
