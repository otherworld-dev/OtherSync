package main

import (
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
