package engine

import (
	"context"
	"sort"
	"sync"
	"testing"

	"github.com/otherworld/nimbo/internal/transport"
)

// Deck #1003. The on-demand adopt scan crawled the whole server to classify the
// files of a local folder that was often empty. Descend lets the caller keep
// the crawl to the folders it actually needs; the server-only rest is left to
// cfapi's lazy population.
func TestRemoteScanDescendGatesListing(t *testing.T) {
	f := newFakeTree()
	out, err := RemoteScan(context.Background(), f, "", ScanOpts{
		Descend: func(rel string) bool { return false },
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := f.callsFor("a"); n != 0 {
		t.Errorf("a listed %d times with Descend false, want 0", n)
	}
	if n := f.callsFor(""); n != 1 {
		t.Errorf("root listed %d times, want 1 (the root is always listed)", n)
	}
	// The undescended folder itself is still recorded from its parent listing.
	if r, ok := out["a"]; !ok || !r.IsDir || r.ETag != "ea" {
		t.Errorf("out[a] = %+v, %v; want the folder recorded from the root listing", r, ok)
	}
	if _, ok := out["a/f2"]; ok {
		t.Error("a/f2 reported although a was never listed")
	}
}

func TestRemoteScanDescendFollowsOnlyTheNeededChain(t *testing.T) {
	f := newFakeTree()
	// Add a sibling the caller doesn't need.
	f.dirs[""] = append(f.dirs[""], d("big", "ebig"))
	f.dirs["big"] = []transport.Entry{d("big", "ebig"), fi("big/x", "ex", 1)}
	want := map[string]bool{"a": true, "a/b": true}
	out, err := RemoteScan(context.Background(), f, "", ScanOpts{
		Descend: func(rel string) bool { return want[rel] },
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := f.callsFor("big"); n != 0 {
		t.Errorf("server-only folder big listed %d times, want 0", n)
	}
	if _, ok := out["a/b/f3"]; !ok {
		t.Error("a/b/f3 missing: the needed chain must be listed")
	}
}

func TestRemoteScanOnListedReportsCompleteFolders(t *testing.T) {
	f := newFakeTree()
	var mu sync.Mutex
	var listed []string
	_, err := RemoteScan(context.Background(), f, "", ScanOpts{
		Descend:  func(rel string) bool { return rel == "a" },
		OnListed: func(rel string) { mu.Lock(); listed = append(listed, rel); mu.Unlock() },
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(listed)
	if got, want := listed, []string{"", "a"}; !equalStrings(got, want) {
		t.Errorf("OnListed = %q, want %q (a/b was never listed)", got, want)
	}
}

// A folder reproduced from the baseline has all its children in the result
// without being listed, so it counts as listed too — every folder beneath it.
func TestRemoteScanOnListedIncludesBaselineRebuilds(t *testing.T) {
	f := newFakeTree()
	base := map[string]BaselineState{
		"a":      {Path: "a", IsDir: true, RemoteETag: "ea"},
		"a/b":    {Path: "a/b", IsDir: true, RemoteETag: "eb"},
		"a/f2":   {Path: "a/f2", RemoteETag: "e2"},
		"a/b/f3": {Path: "a/b/f3", RemoteETag: "e3"},
	}
	var mu sync.Mutex
	var listed []string
	_, err := RemoteScan(context.Background(), f, "", ScanOpts{
		Base:     base,
		OnListed: func(rel string) { mu.Lock(); listed = append(listed, rel); mu.Unlock() },
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(listed)
	if got, want := listed, []string{"", "a", "a/b"}; !equalStrings(got, want) {
		t.Errorf("OnListed = %q, want %q", got, want)
	}
	if n := f.callsFor("a"); n != 0 {
		t.Errorf("a listed %d times, want 0 (baseline prune)", n)
	}
}

func TestRemoteListingLookup(t *testing.T) {
	l := RemoteListing{
		Entries: map[string]RemoteState{
			"a":      {Path: "a", IsDir: true},
			"a/f":    {Path: "a/f"},
			"b":      {Path: "b", IsDir: true},
			"secret": {Path: "secret", IsDir: true},
		},
		Listed: map[string]bool{"": true, "a": true},
		Opaque: map[string]bool{"secret": true},
	}
	cases := []struct {
		rel  string
		want Presence
	}{
		{"a/f", Present},
		{"a/g", Absent},         // a was listed and has no g
		{"gone/deep/x", Absent}, // the root was listed and has no "gone"
		{"b/x", Unknown},        // b exists but was never listed
		{"b/c/x", Unknown},      // under an unlisted folder
		{"secret/x", Unknown},   // end-to-end encrypted: never enumerable
		{"a/sub/x", Absent},     // a listed, no "sub" in it
		{"top", Absent},         // root listed, no "top"
	}
	for _, c := range cases {
		if got := l.Lookup(c.rel); got != c.want {
			t.Errorf("Lookup(%q) = %v, want %v", c.rel, got, c.want)
		}
	}
	// Nil Listed is the legacy full crawl: everything is known.
	full := RemoteListing{Entries: map[string]RemoteState{"a/f": {}}}
	if got := full.Lookup("x/y"); got != Absent {
		t.Errorf("full crawl Lookup(x/y) = %v, want Absent", got)
	}
	if got := full.Lookup("a/f"); got != Present {
		t.Errorf("full crawl Lookup(a/f) = %v, want Present", got)
	}
	// An unlisted root knows nothing.
	none := RemoteListing{Entries: map[string]RemoteState{}, Listed: map[string]bool{}}
	if got := none.Lookup("x"); got != Unknown {
		t.Errorf("unlisted root Lookup(x) = %v, want Unknown", got)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
