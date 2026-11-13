package agent

// Remote-scan pacing against fakeDAV (Deck #1001): engine.RemoteScan driven
// with a real transport.Client, so the listings go through the client's own
// listing pacer exactly as every production caller's do (sync scans, relist,
// the on-demand adopt crawl via Engine.RemoteTree, the escape sweep).
//
// Real clock, small numbers: each test runs in about a second at most.

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/otherworld/nimbo/internal/engine"
	"github.com/otherworld/nimbo/internal/pace"
	"github.com/otherworld/nimbo/internal/transport"
)

// pacingTree builds a root dir plus, under each of tops, n empty subfolders —
// one PROPFIND per folder to crawl.
func pacingTree(n int, tops ...string) map[string]davNode {
	nodes := map[string]davNode{"": {isDir: true, etag: "root"}}
	for _, top := range tops {
		nodes[top] = davNode{isDir: true, etag: "e-" + top}
		for i := 0; i < n; i++ {
			p := fmt.Sprintf("%s/d%03d", top, i)
			nodes[p] = davNode{isDir: true, etag: "e-" + p}
		}
	}
	return nodes
}

func scanDirs(t *testing.T, got map[string]engine.RemoteState, want int) {
	t.Helper()
	dirs := 0
	for _, r := range got {
		if r.IsDir {
			dirs++
		}
	}
	if dirs != want {
		t.Fatalf("scan returned %d folders, want %d", dirs, want)
	}
}

// (a) Two scans at once share one rate: the account's pacer caps their
// combined PROPFINDs, it does not hand each scan a budget of its own.
func TestRemoteScanRateCapSharedAcrossConcurrentScans(t *testing.T) {
	const n, rate, burst = 30, 50.0, 4
	f := newFakeDAV(pacingTree(n, "a", "b"))
	var mu sync.Mutex
	var stamps []time.Time
	f.onPROPFIND = func(string) {
		mu.Lock()
		stamps = append(stamps, time.Now())
		mu.Unlock()
	}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := transport.New(srv.URL, "alice", "pw")
	c.SetListPacer(pace.New(pace.Config{Rate: rate, Burst: burst, Log: func(string, ...any) {}}))

	start := time.Now()
	var wg sync.WaitGroup
	results := make([]map[string]engine.RemoteState, 2)
	errs := make([]error, 2)
	for i, root := range []string{"a", "b"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], errs[i] = engine.RemoteScan(context.Background(), c, root, engine.ScanOpts{})
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)
	for i := range errs {
		if errs[i] != nil {
			t.Fatalf("scan %d: %v", i, errs[i])
		}
		scanDirs(t, results[i], n)
	}

	total := len(stamps)
	if total != 2*(n+1) {
		t.Fatalf("%d PROPFINDs, want %d", total, 2*(n+1))
	}
	// One shared bucket: (62 - 4) / 50 = 1.16 s at the least. Two separate
	// buckets would finish in about half that.
	minWant := time.Duration(float64(total-burst) / rate * float64(time.Second))
	if elapsed < minWant*9/10 {
		t.Fatalf("2 concurrent scans of %d PROPFINDs took %v; the shared %v/s cap needs >= %v", total, elapsed, rate, minWant)
	}
	// And no stretch of the run beat the cap: in any 500 ms window, at most
	// rate/2 + burst requests (+1 for the window edges).
	mu.Lock()
	defer mu.Unlock()
	limit := int(rate/2) + burst + 1
	for i := range stamps {
		inWin := 0
		for j := i; j < len(stamps) && stamps[j].Sub(stamps[i]) < 500*time.Millisecond; j++ {
			inWin++
		}
		if inWin > limit {
			t.Fatalf("%d PROPFINDs within 500ms (from request %d); cap allows %d", inWin, i, limit)
		}
	}
}

// (b) Responses slowing down well past the baseline make the scan back off,
// before the server ever returns an error.
func TestRemoteScanBacksOffWhenLatencyRises(t *testing.T) {
	const n = 60
	f := newFakeDAV(pacingTree(n, "c"))
	var served atomic.Int64
	f.onPROPFIND = func(string) {
		if served.Add(1) > 20 {
			time.Sleep(60 * time.Millisecond) // the server starts to struggle
		} else {
			time.Sleep(time.Millisecond)
		}
	}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := transport.New(srv.URL, "alice", "pw")
	var logMu sync.Mutex
	var lines []string
	p := pace.New(pace.Config{
		Rate: 200, Burst: 5, MinRate: 1,
		SlowMargin:  10 * time.Millisecond, // localhost scale; production uses 250ms
		CutCooldown: 50 * time.Millisecond,
		LogEvery:    time.Millisecond,
		Log: func(msg string, args ...any) {
			logMu.Lock()
			lines = append(lines, msg+" "+fmt.Sprint(args...))
			logMu.Unlock()
		},
	})
	c.SetListPacer(p)

	got, err := engine.RemoteScan(context.Background(), c, "c", engine.ScanOpts{})
	if err != nil {
		t.Fatal(err)
	}
	scanDirs(t, got, n)
	if r := p.Rate(); r >= 200 {
		t.Fatalf("latency rose 60x mid-scan but the rate stayed at %v", r)
	}
	logMu.Lock()
	defer logMu.Unlock()
	slowed := false
	for _, l := range lines {
		if strings.Contains(l, "Slowing down") && strings.Contains(l, "answering slowly") {
			slowed = true
		}
	}
	if !slowed {
		t.Fatalf("no slow-down line logged; got %q", lines)
	}
}

// (c) A small, normal-sized scan is not slowed by the default pacer: its
// PROPFINDs fit in the burst, so it runs as fast as the 4 workers allow.
func TestRemoteScanSmallTreeNotSlowed(t *testing.T) {
	const n = 40 // 41 PROPFINDs, a busy delta poll; unpaced bursts would take ~7s at 6/s
	f := newFakeDAV(pacingTree(n, "s"))
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := transport.New(srv.URL, "alice", "pw") // the production default pacer
	if r := c.ListPacer().Rate(); r != pace.DefaultRate {
		t.Fatalf("default pacer rate %v", r)
	}
	start := time.Now()
	got, err := engine.RemoteScan(context.Background(), c, "s", engine.ScanOpts{})
	if err != nil {
		t.Fatal(err)
	}
	scanDirs(t, got, n)
	if el := time.Since(start); el > time.Second {
		t.Fatalf("a %d-folder scan took %v with the default pacer; a normal scan must not be slowed", n+1, el)
	}
	if r := c.ListPacer().Rate(); r != pace.DefaultRate {
		t.Fatalf("a healthy small scan changed the rate to %v", r)
	}
}

// The on-demand adopt crawl (the 31 Oct trigger) lists through the same
// account pacer as everything else.
func TestRemoteTreeIsPaced(t *testing.T) {
	const n = 20
	f := newFakeDAV(pacingTree(n, "t"))
	srv := httptest.NewServer(f)
	defer srv.Close()
	e, _ := newHookEngine(t, srv.URL)
	e.client.SetListPacer(pace.New(pace.Config{Rate: 40, Burst: 1, Log: func(string, ...any) {}}))
	// RemoteTree lists only the server folders holding local files (Deck
	// #1003), so give every folder one: all 21 listings still happen.
	local := t.TempDir()
	for i := 0; i < n; i++ {
		d := filepath.Join(local, fmt.Sprintf("d%03d", i))
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, "f.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now()
	got, err := e.RemoteTree(context.Background(), local, "t", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	scanDirs(t, got.Entries, n)
	// 21 PROPFINDs, burst 1, 40/s: at least 0.5 s.
	if el := time.Since(start); el < 450*time.Millisecond {
		t.Fatalf("RemoteTree's %d listings took %v; it bypassed the account pacer", n+1, el)
	}
}
