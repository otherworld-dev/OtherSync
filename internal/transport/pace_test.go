package transport

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/otherworld/nimbo/internal/pace"
)

// Every client gets its own default listing pacer (one per account).
func TestClientHasDefaultListPacer(t *testing.T) {
	a, b := New("https://a.example", "u", "p"), New("https://b.example", "u", "p")
	if a.ListPacer() == nil || b.ListPacer() == nil {
		t.Fatal("a new client has no listing pacer")
	}
	if a.ListPacer() == b.ListPacer() {
		t.Fatal("two clients share a pacer; it must be per account")
	}
	if got := a.ListPacer().Rate(); got != pace.DefaultRate {
		t.Fatalf("default pacer rate %v, want %v", got, pace.DefaultRate)
	}
}

func TestPaceOutcome(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want pace.Outcome
	}{
		{"ok", nil, pace.OK},
		{"404", fmt.Errorf("x: %w", ErrNotFound), pace.Neutral},
		{"403", statusError("PROPFIND", "x", fakeResp(403, "403 Forbidden")), pace.Neutral},
		{"423", statusError("PROPFIND", "x", fakeResp(423, "423 Locked")), pace.Neutral},
		{"429", statusError("PROPFIND", "x", fakeResp(429, "429 Too Many Requests")), pace.Overloaded},
		{"503", statusError("PROPFIND", "x", fakeResp(503, "503 Service Unavailable")), pace.Overloaded},
		{"exhausted", RetriesExhausted(4, errors.New("server returned 500")), pace.Overloaded},
		{"deadline", context.DeadlineExceeded, pace.Overloaded},
		{"cancel", context.Canceled, pace.Neutral},
		{"offline", errors.New("dial tcp: no such host"), pace.Neutral},
	}
	for _, c := range cases {
		if got := paceOutcome(c.err); got != c.want {
			t.Errorf("%s: outcome %v, want %v", c.name, got, c.want)
		}
	}
}

// recordingPacer builds a pacer whose log lines are captured, on a real clock
// (the requests here are real HTTP), with a generous budget so it never
// actually throttles these tests.
func testPacer(log func(string, ...any)) *pace.Pacer {
	return pace.New(pace.Config{Rate: 1000, Burst: 1000, MinRate: 1, Log: log})
}

// A 429 with Retry-After is not retried by Do, but the server's request to
// back off must still reach the account's pacer.
func TestRetryAfterHoldsListings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := New(srv.URL, "alice", "pw")
	var mu sync.Mutex
	var lines []string
	p := testPacer(func(msg string, args ...any) {
		mu.Lock()
		lines = append(lines, msg+" "+fmt.Sprint(args...))
		mu.Unlock()
	})
	c.SetListPacer(p)

	if _, err := c.PacedPropFind(context.Background(), "", 1); err == nil {
		t.Fatal("a 429 listing should fail")
	}
	if got := p.Rate(); got >= 1000 {
		t.Fatalf("pacer rate %v after a 429 with Retry-After; want it cut", got)
	}
	// The hold: the next paced listing waits out (most of) the 2 s.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := p.Wait(ctx); err == nil {
		t.Fatal("a listing right after Retry-After: 2 went through without waiting")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(lines) == 0 {
		t.Fatal("the slow-down was not logged")
	}
}

// A paced listing feeds its outcome to the pacer; an unpaced one does not.
func TestPacedPropFindReportsOverload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden) // not retried, and not strain
	}))
	defer srv.Close()
	c := New(srv.URL, "alice", "pw")
	p := testPacer(func(string, ...any) {})
	c.SetListPacer(p)
	_, _ = c.PacedPropFind(context.Background(), "x", 1)
	if got := p.Rate(); got != 1000 {
		t.Fatalf("a 403 cut the rate to %v; refusals are not strain", got)
	}

	srv5 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotImplemented) // 501: not retried, returned as a status error
	}))
	defer srv5.Close()
	c5 := New(srv5.URL, "alice", "pw")
	p5 := testPacer(func(string, ...any) {})
	c5.SetListPacer(p5)
	_, _ = c5.PropFind(context.Background(), "x", 1) // unpaced
	if got := p5.Rate(); got != 1000 {
		t.Fatalf("an unpaced PropFind touched the pacer (rate %v)", got)
	}
}

func TestSetListPacerNilDisables(t *testing.T) {
	c := New("https://a.example", "u", "p")
	c.SetListPacer(nil)
	if c.ListPacer() != nil {
		t.Fatal("SetListPacer(nil) should disable pacing")
	}
}

// Every client gets its own download gate, open to the default 6 at once.
func TestClientHasDefaultDownloadGate(t *testing.T) {
	a, b := New("https://a.example", "u", "p"), New("https://b.example", "u", "p")
	if a.DownloadGate() == nil || a.DownloadGate() == b.DownloadGate() {
		t.Fatal("each client needs a download gate of its own")
	}
	if got := a.DownloadGate().Limit(); got != pace.DefaultGateMax {
		t.Fatalf("default gate limit %d, want %d", got, pace.DefaultGateMax)
	}
}

// A download holds its slot until the body is closed, not just until the
// headers arrive: the server's worker is busy for as long as it streams.
func TestPacedGetFromHoldsSlotUntilClose(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()
	c := New(srv.URL, "alice", "pw")
	g := pace.NewGate(pace.GateConfig{Max: 1, Log: func(string, ...any) {}})
	c.SetDownloadGate(g)

	body, _, status, err := c.PacedGetFrom(context.Background(), "f.txt", 0)
	if err != nil || status != http.StatusOK {
		t.Fatalf("PacedGetFrom: status %d, err %v", status, err)
	}
	if g.InFlight() != 1 {
		t.Fatalf("in flight %d while the body is open, want 1", g.InFlight())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, _, _, err := c.PacedGetFrom(ctx, "g.txt", 0); err == nil {
		t.Fatal("a second download started while the only slot was held")
	}
	_ = body.Close()
	_ = body.Close() // a second Close must not free a slot it doesn't hold
	if g.InFlight() != 0 {
		t.Fatalf("in flight %d after Close, want 0", g.InFlight())
	}
}

// A failed download gives its slot straight back, and a 5xx narrows the gate.
func TestPacedGetFromErrorReleasesAndNarrows(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotImplemented) // 501: not retried, not strain
	}))
	defer srv.Close()
	c := New(srv.URL, "alice", "pw")
	g := pace.NewGate(pace.GateConfig{Max: 6, Log: func(string, ...any) {}})
	c.SetDownloadGate(g)
	if _, _, _, err := c.PacedGetFrom(context.Background(), "f.txt", 0); err == nil {
		t.Fatal("a 501 download succeeded")
	}
	if g.InFlight() != 0 {
		t.Fatalf("in flight %d after a failed download, want 0", g.InFlight())
	}
	if g.Limit() != 6 {
		t.Fatalf("a 501 narrowed the gate to %d; it is not strain", g.Limit())
	}
	g.Observe(0, paceOutcome(fmt.Errorf("x: %w", ErrRetriesExhausted)))
	if g.Limit() != 3 {
		t.Fatalf("an exhausted retry left the gate at %d, want 3", g.Limit())
	}
}
