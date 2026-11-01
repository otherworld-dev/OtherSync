package agent

import (
	"context"
	"errors"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// withStagger swaps the crawl pause for the test and counts how often it is
// taken.
func withStagger(t *testing.T, d time.Duration) *atomic.Int32 {
	t.Helper()
	var calls atomic.Int32
	old := crawlStagger
	crawlStagger = func() time.Duration { calls.Add(1); return d }
	t.Cleanup(func() { crawlStagger = old })
	return &calls
}

// A first sync and the one-off folder check wait before listing the server;
// ordinary passes do not.
func TestStaggerOnlyBeforeWholeTreeCrawls(t *testing.T) {
	calls := withStagger(t, time.Millisecond)
	f := newFakeDAV(cloneTree("A/a1", "B/b1"))
	srv := httptest.NewServer(f)
	defer srv.Close()
	e, st := newHookEngine(t, srv.URL)
	local := t.TempDir()
	pair := Pair{LocalDir: local, RemoteRoot: ""}
	ctx := context.Background()

	if _, err := e.SyncOnce(ctx, pair); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("first sync paused %d times, want 1", n)
	}
	for i := 0; i < 2; i++ {
		if _, err := e.syncRemoteDelta(ctx, pair); err != nil {
			t.Fatal(err)
		}
	}
	if n := calls.Load(); n != 1 {
		t.Fatalf("ordinary passes paused (%d pauses in all), want none", n-1)
	}

	// The pair as an older build left it: the folder check runs, and waits first.
	if err := st.SetPairMeta(PairKey(local, ""), relistMeta, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := e.syncRemoteDelta(ctx, pair); err != nil {
		t.Fatal(err)
	}
	if n := calls.Load(); n != 2 {
		t.Fatalf("the folder check paused %d times, want 1", n-1)
	}
}

// Quitting during the pause ends it at once, before anything is listed.
func TestStaggerEndsWhenTheEngineStops(t *testing.T) {
	withStagger(t, time.Hour)
	f := newFakeDAV(cloneTree("A/a1"))
	srv := httptest.NewServer(f)
	defer srv.Close()
	e, _ := newHookEngine(t, srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := e.SyncOnce(ctx, Pair{LocalDir: t.TempDir(), RemoteRoot: ""})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SyncOnce during the pause = %v, want the context's error", err)
	}
	if el := time.Since(start); el > 5*time.Second {
		t.Fatalf("SyncOnce took %s to stop", el)
	}
	if n := f.pfCount(""); n != 0 {
		t.Fatalf("%d listings sent during the pause, want 0", n)
	}
}
