package agent

// The first sync lists each top-level folder in one Depth: infinity PROPFIND.
// On a big folder the server builds the whole answer in memory and can run out
// doing so: the desktop's first sync of E:\Nextcloud got a 500 for Projects and
// To Sort every few minutes for a day, each one holding a PHP worker at its
// memory limit, until the server's host killed PHP (Deck #1004).

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A folder the server won't list whole is listed one folder at a time instead,
// and the first sync finishes.
func TestCloneListsAFolderTheServerWontListWhole(t *testing.T) {
	f := newFakeDAV(cloneTree("A/a1", "Big/x/x1", "Big/y1", "Big/Sub/deep/s2", "C/c1"))
	f.setFailInf("Big", http.StatusInternalServerError)
	srv := httptest.NewServer(f)
	defer srv.Close()
	e, st := newHookEngine(t, srv.URL)
	local := t.TempDir()

	if _, err := e.SyncOnce(context.Background(), Pair{LocalDir: local}); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	for _, p := range []string{"A/a1", "Big/x/x1", "Big/y1", "Big/Sub/deep/s2", "C/c1"} {
		if b, err := os.ReadFile(filepath.Join(local, filepath.FromSlash(p))); err != nil || string(b) != "body of "+p {
			t.Errorf("%s: %q, %v", p, b, err)
		}
	}
	if n := f.infCount("Big"); n != 1 {
		t.Errorf("Big was asked for whole %d times, want once", n)
	}
	for _, d := range []string{"Big/x", "Big/Sub", "Big/Sub/deep"} {
		if f.pfCount(d) == 0 {
			t.Errorf("%s was never listed on its own", d)
		}
	}
	if status, _ := st.CloneStatus(PairKey(local, "")); status != "done" {
		t.Errorf("clone status = %q, want done", status)
	}
}

// The folder-by-folder listing does not go into an excluded folder: the
// whole-tree listing brought those back for nothing, but here each one would
// be a request of its own (node_modules alone can be thousands of folders).
func TestCloneFallbackSkipsExcludedFolders(t *testing.T) {
	f := newFakeDAV(cloneTree("Big/app/main.go", "Big/app/node_modules/left-pad/index.js", "Big/app/node_modules/x/y/z.js"))
	f.setFailInf("Big", http.StatusInternalServerError)
	srv := httptest.NewServer(f)
	defer srv.Close()
	e, _ := newHookEngine(t, srv.URL)
	local := t.TempDir()

	if _, err := e.SyncOnce(context.Background(), Pair{LocalDir: local, Excludes: []string{"node_modules"}}); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(local, "Big", "app", "main.go")); err != nil {
		t.Fatalf("Big/app/main.go did not arrive: %v", err)
	}
	for _, d := range []string{"Big/app/node_modules", "Big/app/node_modules/left-pad", "Big/app/node_modules/x"} {
		if n := f.pfCount(d); n != 0 {
			t.Errorf("excluded %s was listed %d times", d, n)
		}
	}
}

// Once the server has refused to list a folder whole, the next try of the
// first sync goes straight to listing it folder by folder. Asking again cost
// the server another minute at its memory limit for an answer already known.
func TestCloneDoesNotAskAgainForAFolderTheServerWontListWhole(t *testing.T) {
	f := newFakeDAV(cloneTree("A/a1", "Big/x/x1", "C/c1"))
	f.setFailInf("Big", http.StatusInternalServerError)
	f.setFailPF("C", http.StatusForbidden) // the first try fails here, so the sync is tried again
	srv := httptest.NewServer(f)
	defer srv.Close()
	e, st := newHookEngine(t, srv.URL)
	local := t.TempDir()
	pair := Pair{LocalDir: local}

	if _, err := e.SyncOnce(context.Background(), pair); err == nil {
		t.Fatal("first try: no error with C refused")
	}
	f.clearFailPF("C")
	if _, err := e.SyncOnce(context.Background(), pair); err != nil {
		t.Fatalf("second try: %v", err)
	}
	if n := f.infCount("Big"); n != 1 {
		t.Errorf("Big was asked for whole %d times over two tries, want once", n)
	}
	if b, err := os.ReadFile(filepath.Join(local, "Big", "x", "x1")); err != nil || string(b) != "body of Big/x/x1" {
		t.Errorf("Big/x/x1: %q, %v", b, err)
	}
	if status, _ := st.CloneStatus(PairKey(local, "")); status != "done" {
		t.Errorf("clone status = %q, want done", status)
	}
}

// A first sync into a folder that already held files keeps them however many
// tries it takes. Only the first try counted as taking the folder over; a
// retry treated a local file of a different size as its own half-finished
// download and fetched the server's copy over it, losing the user's version.
func TestARetriedTakeoverKeepsALocalFileThatDiffers(t *testing.T) {
	f := newFakeDAV(cloneTree("A/a1", "C/c1"))
	f.setFailPF("C", http.StatusForbidden) // the first try fails, so there is a retry
	srv := httptest.NewServer(f)
	defer srv.Close()
	e, _ := newHookEngine(t, srv.URL)
	local := t.TempDir()
	mine := filepath.Join(local, "A", "a1")
	if err := os.MkdirAll(filepath.Dir(mine), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mine, []byte("my own edit, which the server has not seen"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(mine, old, old); err != nil {
		t.Fatal(err)
	}
	pair := Pair{LocalDir: local}

	if _, err := e.SyncOnce(context.Background(), pair); err == nil {
		t.Fatal("first try: no error with C refused")
	}
	f.clearFailPF("C")
	if _, err := e.SyncOnce(context.Background(), pair); err != nil {
		t.Fatalf("second try: %v", err)
	}
	// The normal sync after the clone may keep both copies as a conflict;
	// what must not happen is the edit being gone.
	found := false
	_ = filepath.WalkDir(filepath.Join(local, "A"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if b, rerr := os.ReadFile(p); rerr == nil && strings.Contains(string(b), "my own edit") {
				found = true
			}
		}
		return nil
	})
	if !found {
		t.Fatal("the local edit to A/a1 was overwritten by the retried first sync")
	}
}
