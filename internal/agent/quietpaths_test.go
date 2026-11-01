package agent

// Deck #1011: straight after a first download, the watcher reports every file
// Nimbo wrote as a local change, and the scoped sync asked the server about
// each one (70 single-path PROPFINDs for a 26-file test tree on the VM; on an
// account the size of the desktop's, hundreds of thousands).

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// totalPF is every PROPFIND the fake server has answered so far.
func (f *fakeDAV) totalPF() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.pfCalls {
		n += c
	}
	return n
}

// A file whose local copy is exactly what was last synced has nothing to send,
// so the scoped sync leaves it alone and asks the server nothing about it. A
// change on the server is still the push's or the poll's to find. Folders are
// still asked about (the outage guards of Deck #691 work on that answer).
func TestSyncPathsLeavesAloneWhatItJustWrote(t *testing.T) {
	files := []string{"A/a1", "A/deep/d1", "B/b1"}
	f := newFakeDAV(cloneTree(files...))
	srv := httptest.NewServer(f)
	defer srv.Close()
	e, _ := newHookEngine(t, srv.URL)
	local := t.TempDir()
	p := Pair{LocalDir: local}
	if _, err := e.SyncOnce(context.Background(), p); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	before := f.totalPF()
	// What the watcher reports after the first sync: every file and folder it made.
	if _, err := e.SyncPaths(context.Background(), p, append(files, "A", "A/deep", "B")); err != nil {
		t.Fatalf("scoped sync: %v", err)
	}
	for _, file := range files {
		if n := f.pfCount(file); n != 0 {
			t.Errorf("the scoped sync asked the server about %s, which it had just written", file)
		}
	}
	if n := f.totalPF() - before; n != 3 {
		t.Errorf("the scoped sync sent %d PROPFINDs, want one per folder (3)", n)
	}
	if puts := f.putPaths(); len(puts) != 0 {
		t.Errorf("uploaded %v, want nothing", puts)
	}
}

// A file that really did change locally in the same batch is still synced.
func TestSyncPathsStillSendsAFileThatChanged(t *testing.T) {
	files := []string{"A/a1", "B/b1"}
	f := newFakeDAV(cloneTree(files...))
	srv := httptest.NewServer(f)
	defer srv.Close()
	e, _ := newHookEngine(t, srv.URL)
	local := t.TempDir()
	p := Pair{LocalDir: local}
	if _, err := e.SyncOnce(context.Background(), p); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if err := os.WriteFile(filepath.Join(local, "A", "a1"), []byte("edited locally, a different size"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := e.SyncPaths(context.Background(), p, files); err != nil {
		t.Fatalf("scoped sync: %v", err)
	}
	if got := f.putBody("A/a1"); got != "edited locally, a different size" {
		t.Errorf("A/a1 on the server = %q, want the local edit", got)
	}
	if n := f.pfCount("B/b1"); n != 0 {
		t.Errorf("unchanged B/b1 was asked about %d times, want none", n)
	}
}
