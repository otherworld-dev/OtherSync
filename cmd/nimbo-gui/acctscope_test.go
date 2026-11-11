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
