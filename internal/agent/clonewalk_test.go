package agent

// A first sync of a big account lists the folders the server won't list whole
// one at a time, which on Adam's laptop on 4 Nov 2026 took hours (Projects and
// To Sort, tens of thousands of folders at the listing pace). One network blip
// failed the whole first sync, the retry listed everything again from the
// start, and nothing downloaded until each top-level folder was listed to the
// end, so it looked stuck for hours (Deck #1111).

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/otherworld/nimbo/internal/activity"
	"github.com/otherworld/nimbo/internal/engine"
	"github.com/otherworld/nimbo/internal/transport"
)

// restartEngine stands in for the app being closed and started again: a new
// Engine on the same state database, with nothing carried over in memory.
func restartEngine(t *testing.T, old *Engine, server string) *Engine {
	t.Helper()
	old.closeStore()
	e := &Engine{
		Account:   old.Account,
		dirs:      old.dirs,
		client:    transport.New(server, "alice", "pw"),
		recorder:  activity.New(),
		blocked:   make(map[string][]engine.Blocked),
		conflicts: make(map[string][]ConflictItem),
	}
	if err := e.reloadGuardState(); err != nil {
		t.Fatal(err)
	}
	if _, err := e.getStore(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.closeStore)
	return e
}

// A first sync tried again does not list the folders the last try already
// listed: those listings are kept, and used while the folder's etag says
// nothing in it has changed.
func TestARetriedFirstSyncDoesNotListFoldersAgain(t *testing.T) {
	f := newFakeDAV(cloneTree("Big/x/x1", "Big/y/y1", "Big/z/w/w1", "C/c1"))
	f.setFailInf("Big", http.StatusInternalServerError)
	f.setFailPF("Big/z/w", http.StatusForbidden) // the first try fails here
	srv := httptest.NewServer(f)
	defer srv.Close()
	e, st := newHookEngine(t, srv.URL)
	local := t.TempDir()
	pair := Pair{LocalDir: local}

	if _, err := e.SyncOnce(context.Background(), pair); err == nil {
		t.Fatal("first try: no error with Big/z/w refused")
	}
	f.clearFailPF("Big/z/w")
	if _, err := e.SyncOnce(context.Background(), pair); err != nil {
		t.Fatalf("second try: %v", err)
	}
	for _, d := range []string{"Big/x", "Big/y", "Big/z"} {
		if n := f.pfCount(d); n != 1 {
			t.Errorf("%s was listed %d times over two tries, want once", d, n)
		}
	}
	for _, p := range []string{"Big/x/x1", "Big/y/y1", "Big/z/w/w1", "C/c1"} {
		if b, err := os.ReadFile(filepath.Join(local, filepath.FromSlash(p))); err != nil || string(b) != "body of "+p {
			t.Errorf("%s: %q, %v", p, b, err)
		}
	}
	if status, _ := st.CloneStatus(PairKey(local, "")); status != "done" {
		t.Errorf("clone status = %q, want done", status)
	}
}

// A folder that changed on the server since the last try is listed again,
// and the file added to it arrives.
func TestARetriedFirstSyncListsAChangedFolderAgain(t *testing.T) {
	f := newFakeDAV(cloneTree("Big/x/x1", "Big/z/w/w1"))
	f.setFailInf("Big", http.StatusInternalServerError)
	f.setFailPF("Big/z/w", http.StatusForbidden)
	srv := httptest.NewServer(f)
	defer srv.Close()
	e, _ := newHookEngine(t, srv.URL)
	local := t.TempDir()
	pair := Pair{LocalDir: local}

	if _, err := e.SyncOnce(context.Background(), pair); err == nil {
		t.Fatal("first try: no error with Big/z/w refused")
	}
	// A file lands in Big/x: its etag and every etag above it change.
	f.setNode("Big/x/x2", davNode{etag: "e-Big/x/x2", body: "body of Big/x/x2"})
	for _, d := range []string{"", "Big", "Big/x"} {
		f.setNode(d, davNode{isDir: true, etag: "e2-" + d})
	}
	f.clearFailPF("Big/z/w")
	if _, err := e.SyncOnce(context.Background(), pair); err != nil {
		t.Fatalf("second try: %v", err)
	}
	if n := f.pfCount("Big/x"); n != 2 {
		t.Errorf("Big/x was listed %d times, want twice (it changed)", n)
	}
	if n := f.pfCount("Big/z"); n != 1 {
		t.Errorf("Big/z was listed %d times, want once (it did not change)", n)
	}
	if b, err := os.ReadFile(filepath.Join(local, "Big", "x", "x2")); err != nil || string(b) != "body of Big/x/x2" {
		t.Errorf("Big/x/x2: %q, %v", b, err)
	}
}

// After the app is restarted, the first sync neither asks again for a folder
// the server would not list whole nor lists again what it already listed.
func TestARestartedFirstSyncCarriesOn(t *testing.T) {
	f := newFakeDAV(cloneTree("Big/x/x1", "Big/z/w/w1"))
	f.setFailInf("Big", http.StatusInternalServerError)
	f.setFailPF("Big/z/w", http.StatusForbidden)
	srv := httptest.NewServer(f)
	defer srv.Close()
	e, _ := newHookEngine(t, srv.URL)
	local := t.TempDir()
	pair := Pair{LocalDir: local}

	if _, err := e.SyncOnce(context.Background(), pair); err == nil {
		t.Fatal("first try: no error with Big/z/w refused")
	}
	e = restartEngine(t, e, srv.URL)
	f.clearFailPF("Big/z/w")
	if _, err := e.SyncOnce(context.Background(), pair); err != nil {
		t.Fatalf("after the restart: %v", err)
	}
	if n := f.infCount("Big"); n != 1 {
		t.Errorf("Big was asked for whole %d times, want once", n)
	}
	if n := f.pfCount("Big/x"); n != 1 {
		t.Errorf("Big/x was listed %d times, want once", n)
	}
	if _, err := os.Stat(filepath.Join(local, "Big", "z", "w", "w1")); err != nil {
		t.Errorf("Big/z/w/w1 did not arrive: %v", err)
	}
}

// Files start downloading as soon as their folder is listed, not once the
// whole top-level folder around them has been.
func TestAFirstSyncDownloadsWhileItLists(t *testing.T) {
	f := newFakeDAV(cloneTree("Big/a/a1", "Big/b/b1", "Big/zz/z1"))
	f.setFailInf("Big", http.StatusInternalServerError)
	var (
		got     = make(chan struct{})
		gotOnce sync.Once
		mu      sync.Mutex
		early   bool
	)
	f.onGET = func(rel string) {
		if rel == "Big/a/a1" {
			gotOnce.Do(func() { close(got) })
		}
	}
	f.onPROPFIND = func(rel string) {
		if rel != "Big/zz" {
			return
		}
		select {
		case <-got:
			mu.Lock()
			early = true
			mu.Unlock()
		case <-time.After(5 * time.Second):
		}
	}
	srv := httptest.NewServer(f)
	defer srv.Close()
	e, _ := newHookEngine(t, srv.URL)
	e.cloneTestPools = [2]int{1, 2}
	local := t.TempDir()

	if _, err := e.SyncOnce(context.Background(), Pair{LocalDir: local}); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if !early {
		t.Error("Big/a/a1 only downloaded after the listing of Big had finished")
	}
}

// A folder whose listing fails for longer than the transport's own retries
// (a laptop's wifi dropping for a moment) is tried again a little later, and
// the first sync carries on instead of failing.
func TestAFirstSyncWaitsOutANetworkBlip(t *testing.T) {
	old := walkRetryDelays
	walkRetryDelays = []time.Duration{10 * time.Millisecond, 10 * time.Millisecond}
	defer func() { walkRetryDelays = old }()

	f := newFakeDAV(cloneTree("Big/x/x1", "Big/y/y1"))
	f.setFailInf("Big", http.StatusInternalServerError)
	var (
		mu    sync.Mutex
		calls int
	)
	f.onPROPFIND = func(rel string) {
		if rel != "Big/x" {
			return
		}
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n <= 4 { // every attempt of the transport's first request
			f.setFailPF("Big/x", http.StatusServiceUnavailable)
		} else {
			f.clearFailPF("Big/x")
		}
	}
	srv := httptest.NewServer(f)
	defer srv.Close()
	e, st := newHookEngine(t, srv.URL)
	local := t.TempDir()

	if _, err := e.SyncOnce(context.Background(), Pair{LocalDir: local}); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(local, "Big", "x", "x1")); err != nil {
		t.Errorf("Big/x/x1 did not arrive: %v", err)
	}
	if status, _ := st.CloneStatus(PairKey(local, "")); status != "done" {
		t.Errorf("clone status = %q, want done", status)
	}
}
