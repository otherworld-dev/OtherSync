package main

import (
	"os"
	"testing"

	"github.com/otherworld/nimbo/internal/account"
	"github.com/otherworld/nimbo/internal/agent"
	"github.com/otherworld/nimbo/internal/config"
)

func TestAcctEngineNeverFallsBack(t *testing.T) {
	ea := &agent.Engine{Account: account.Account{ID: "a"}}
	eb := &agent.Engine{Account: account.Account{ID: "b"}}
	a := &App{eng: ea}
	a.addSecondary("b", &secondaryEngine{eng: eb})
	a.addSecondary("c", &secondaryEngine{})

	for _, id := range []string{"", "a"} {
		if got, err := a.acctEngine(id); err != nil || got != ea {
			t.Fatalf("acctEngine(%q) = %v, %v; want A", id, got, err)
		}
	}
	if got, err := a.acctEngine("b"); err != nil || got != eb {
		t.Fatalf("acctEngine(b) = %v, %v; want B", got, err)
	}
	for _, id := range []string{"c", "zz"} {
		if got, err := a.acctEngine(id); got != nil || err != errAccountNotRunning {
			t.Fatalf("acctEngine(%q) = %v, %v; want nil, errAccountNotRunning", id, got, err)
		}
	}
	if !a.isShown(ea) || a.isShown(eb) || a.isShown(nil) {
		t.Fatal("isShown wrong")
	}
}

func TestAcctEngineSignedOut(t *testing.T) {
	if got, err := (&App{}).acctEngine(""); got != nil || err == nil {
		t.Fatalf("got %v, %v; want nil and an error", got, err)
	}
}

func TestPairsAndBaseDirFromDisk(t *testing.T) {
	d := config.Dirs{Config: t.TempDir(), Data: t.TempDir()}
	_ = d.WithAccount("b").SavePairs([]config.SyncPair{{LocalDir: `C:\B\Docs`, RemoteRoot: "/Docs"}})
	_ = d.WithAccount("b").UpdateAccountState(func(s *config.AccountState) { s.BaseDir = `C:\B` })

	got := pairsFromDisk(d, "b")
	if len(got) != 1 || got[0].LocalDir != `C:\B\Docs` {
		t.Fatalf("pairsFromDisk(b) = %v", got)
	}
	if len(pairsFromDisk(d, "a")) != 0 {
		t.Fatal("account a should have no pairs")
	}
	if g := baseDirFromDisk(d, "b"); g != `C:\B` {
		t.Fatalf("baseDir = %q", g)
	}
	_ = d.WithAccount("b").SavePairs([]config.SyncPair{
		{LocalDir: `C:\B\Docs`, RemoteRoot: "/Docs"},
		{LocalDir: `C:\B\All`, RemoteRoot: ""},
	})
	if g := baseDirFromDisk(d, "b"); g != `C:\B\All` {
		t.Fatalf("baseDir with whole-account pair = %q", g)
	}
	// "/" is the whole account too, as baseDirOf and healedBaseDir read it.
	_ = d.WithAccount("b").SavePairs([]config.SyncPair{
		{LocalDir: `C:\B\Docs`, RemoteRoot: "/Docs"},
		{LocalDir: `C:\B\Root`, RemoteRoot: "/"},
	})
	if g := baseDirFromDisk(d, "b"); g != `C:\B\Root` {
		t.Fatalf(`baseDir with a "/" whole-account pair = %q`, g)
	}
}

// A write aimed at an account with no running engine must refuse before it
// touches any engine. A's engine is a zero value, so reaching it would panic.
func TestWritesOnNotRunningAccountRefuse(t *testing.T) {
	a := &App{eng: &agent.Engine{Account: account.Account{ID: "a"}}}
	want := errAccountNotRunning.Error()
	for name, got := range map[string]func() string{
		"AddSyncPair":      func() string { return a.AddSyncPair("b", `C:\x`, "/x") },
		"DeselectFolder":   func() string { return a.DeselectFolder("b", `C:\x`, "y", false) },
		"MoveSyncFolder":   func() string { return a.MoveSyncFolder("b", `C:\x`, `C:\y`) },
		"SaveLocalAddress": func() string { return a.SaveLocalAddress("b", "", "") },
		"SetOfflinePin":    func() string { return a.SetOfflinePin("b", "x", true) },
		"resumeFrozen":     func() string { return a.resumeFrozen("b", `C:\x`) },
	} {
		if g := got(); g != want {
			t.Errorf("%s(b) = %q; want %q", name, g, want)
		}
	}
	a.RemoveSyncFolder("b", "/x", false)
	a.RemoveExclude("b", `C:\x`, "y")
	a.SetBaseDir("b", `C:\x`)
}

// Reads for a configured account with no engine come from its saved settings
// on disk, never from the shown account's engine (a zero value here, which
// would panic if touched). An id that isn't a configured account reads nothing.
func TestReadsOnNotRunningAccount(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	d, err := config.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	seedAccounts(t, d,
		account.Account{ID: "a", ServerURL: "https://one.example.com", LoginName: "adam"},
		account.Account{ID: "b", ServerURL: "https://two.example.com", LoginName: "bob"},
	)
	_ = d.WithAccount("b").SavePairs([]config.SyncPair{{LocalDir: `C:\B\Docs`, RemoteRoot: "/Docs"}})
	_ = d.WithAccount("b").UpdateAccountState(func(s *config.AccountState) { s.BaseDir = `C:\B` })

	a := &App{eng: &agent.Engine{Account: account.Account{ID: "a"}}}
	if got := a.GetPairs("b"); len(got) != 1 || got[0].LocalDir != `C:\B\Docs` {
		t.Errorf("GetPairs(b) = %v", got)
	}
	if got := a.SyncedRemotes("b"); len(got) != 1 || got[0] != "Docs" {
		t.Errorf("SyncedRemotes(b) = %v", got)
	}
	if got := a.GetBaseDir("b"); got != `C:\B` {
		t.Errorf("GetBaseDir(b) = %q", got)
	}
	if got := a.GetPairs("zz"); got != nil {
		t.Errorf("GetPairs(zz) = %v; want nil", got)
	}
	if got := a.GetBaseDir("zz"); got != "" {
		t.Errorf("GetBaseDir(zz) = %q; want empty", got)
	}
	if got := a.BrowseRemote("b", "/"); got != nil {
		t.Errorf("BrowseRemote(b) = %v; want nil", got)
	}
	if got := a.BrowseOffline("b", ""); got != nil {
		t.Errorf("BrowseOffline(b) = %v; want nil", got)
	}
	if got := a.TestLocalAddress("b", "nc.lan", ""); got.Result != "error" || got.Message != errAccountNotRunning.Error() {
		t.Errorf("TestLocalAddress(b) = %+v", got)
	}
}

func TestParseGuardResume(t *testing.T) {
	for _, c := range []struct{ in, acct, dir string }{
		{`C:\Users\x\Nimbo`, "", `C:\Users\x\Nimbo`},
		{"b\n" + `C:\B\Docs`, "b", `C:\B\Docs`},
		{"\n" + `C:\A`, "", `C:\A`},
	} {
		if acct, dir := parseGuardResume(c.in); acct != c.acct || dir != c.dir {
			t.Errorf("parseGuardResume(%q) = %q, %q; want %q, %q", c.in, acct, dir, c.acct, c.dir)
		}
	}
}

// healedBaseDir picks the whole-account pair's folder when the stored baseDir
// has drifted from it, and leaves everything else alone.
func TestHealedBaseDir(t *testing.T) {
	whole := []config.SyncPair{
		{LocalDir: `C:\B\Docs`, RemoteRoot: "/Docs"},
		{LocalDir: `D:\B`, RemoteRoot: "/"},
	}
	if dir, ok := healedBaseDir(whole, `C:\B`); !ok || dir != `D:\B` {
		t.Errorf("drifted: got %q, %v; want D:\\B, true", dir, ok)
	}
	if dir, ok := healedBaseDir(whole, `D:\B`); ok || dir != "" {
		t.Errorf("in step: got %q, %v; want no change", dir, ok)
	}
	if dir, ok := healedBaseDir(whole[:1], `C:\B`); ok || dir != "" {
		t.Errorf("no whole-account pair: got %q, %v; want no change", dir, ok)
	}
	if dir, ok := healedBaseDir(nil, ""); ok || dir != "" {
		t.Errorf("no pairs: got %q, %v; want no change", dir, ok)
	}
}

func TestAccountEntryFromStoreWithoutEngine(t *testing.T) {
	ac := account.Account{ID: "b", LoginName: "bob", ServerURL: "https://x",
		Local: &account.LocalRoute{Address: "192.168.0.5", Pin: "ab"}}
	e := accountEntry(ac, []account.Account{ac}, "a", "Sign in again", nil)
	if e.Running || e.Active || e.Syncing {
		t.Fatalf("running/active/syncing = %v/%v/%v; want all false", e.Running, e.Active, e.Syncing)
	}
	if e.Status != "Sign in again" {
		t.Fatalf("Status = %q", e.Status)
	}
	if e.LocalAddress != "192.168.0.5" || !e.LocalPinned {
		t.Fatalf("local = %q pinned %v", e.LocalAddress, e.LocalPinned)
	}
	if e.Route != "" || e.RouteReason != "" {
		t.Fatalf("route = %q / %q; want empty", e.Route, e.RouteReason)
	}
}

func TestForgetAccountFilesKeepsOrClearsData(t *testing.T) {
	for _, clearData := range []bool{false, true} {
		d := config.Dirs{Config: t.TempDir(), Data: t.TempDir()}
		err := account.Update(d.AccountsFile(), func(s *account.Store) error {
			s.Upsert(account.Account{ID: "a", ServerURL: "https://a.example", LoginName: "a"})
			s.Upsert(account.Account{ID: "b", ServerURL: "https://b.example", LoginName: "b"})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		da, db := d.WithAccount("a"), d.WithAccount("b")
		for _, p := range []string{da.PairsFile(), db.PairsFile(), db.AccountStateFile()} {
			if err := os.WriteFile(p, []byte("{}"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		var deleted []string
		fakeDel := func(id string) error { deleted = append(deleted, id); return nil }
		if err := forgetAccountFiles(d, "b", clearData, fakeDel, (&App{}).clearSyncData); err != nil {
			t.Fatal(err)
		}
		st, err := account.LoadStore(d.AccountsFile())
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := st.Find("b"); ok {
			t.Fatalf("clearData=%v: b still in the store", clearData)
		}
		if _, ok := st.Find("a"); !ok {
			t.Fatalf("clearData=%v: a was removed", clearData)
		}
		if len(deleted) != 1 || deleted[0] != "b" {
			t.Fatalf("clearData=%v: deleted secrets %v, want [b]", clearData, deleted)
		}
		exists := func(p string) bool { _, err := os.Stat(p); return err == nil }
		if !exists(da.PairsFile()) {
			t.Fatalf("clearData=%v: a's pairs file was removed", clearData)
		}
		if got := exists(db.PairsFile()); got == clearData {
			t.Fatalf("clearData=%v: b's pairs file exists=%v", clearData, got)
		}
		if got := exists(db.AccountStateFile()); got == clearData {
			t.Fatalf("clearData=%v: b's account state exists=%v", clearData, got)
		}
	}
}
