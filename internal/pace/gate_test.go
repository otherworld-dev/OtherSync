package pace

import (
	"context"
	"sync"
	"testing"
	"time"
)

func newTestGate(cfg GateConfig) (*Gate, *fakeClock, *logSink) {
	clk := newFakeClock()
	sink := &logSink{}
	cfg.Clock = clk
	cfg.Log = sink.log
	return NewGate(cfg), clk, sink
}

func gateObserveN(g *Gate, clk *fakeClock, n int, lat time.Duration, o Outcome) {
	for i := 0; i < n; i++ {
		clk.advance(lat)
		g.Observe(lat, o)
	}
}

func TestGateDefaultsMatchTheServerBudget(t *testing.T) {
	// 6 at once leaves most of a 16-worker php-fpm pool to everything else;
	// 32 took all of it (3 Nov 2026).
	g := NewGate(GateConfig{})
	if g.Limit() != 6 {
		t.Fatalf("default limit %d, want 6", g.Limit())
	}
}

func TestGateCapsDownloadsAtOnce(t *testing.T) {
	g, _, _ := newTestGate(GateConfig{Max: 3})
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := g.Acquire(ctx); err != nil {
			t.Fatal(err)
		}
	}
	got := make(chan struct{})
	go func() {
		if err := g.Acquire(ctx); err == nil {
			close(got)
		}
	}()
	select {
	case <-got:
		t.Fatal("a fourth download started while three were running under a limit of 3")
	case <-time.After(50 * time.Millisecond):
	}
	g.Release()
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("a freed slot did not let the waiting download start")
	}
	if g.InFlight() != 3 {
		t.Fatalf("in flight %d, want 3", g.InFlight())
	}
}

func TestGateNeverExceedsLimitUnderLoad(t *testing.T) {
	g, _, _ := newTestGate(GateConfig{Max: 4})
	var mu sync.Mutex
	running, peak := 0, 0
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := g.Acquire(context.Background()); err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			running++
			if running > peak {
				peak = running
			}
			mu.Unlock()
			time.Sleep(time.Millisecond)
			mu.Lock()
			running--
			mu.Unlock()
			g.Release()
		}()
	}
	wg.Wait()
	if peak > 4 {
		t.Fatalf("%d downloads ran at once, limit 4", peak)
	}
}

func TestGateCancelledAcquire(t *testing.T) {
	g, _, _ := newTestGate(GateConfig{Max: 1})
	if err := g.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := g.Acquire(ctx); err == nil {
		t.Fatal("a cancelled wait took a slot")
	}
	if g.InFlight() != 1 {
		t.Fatalf("in flight %d after a cancelled wait, want 1", g.InFlight())
	}
}

func TestNilGateIsUnlimited(t *testing.T) {
	var g *Gate
	if err := g.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	g.Observe(time.Second, Overloaded)
	g.Release()
	if g.Limit() != 0 {
		t.Fatal("nil gate has a limit")
	}
}

func TestGateNarrowsOnOverload(t *testing.T) {
	g, clk, _ := newTestGate(GateConfig{Max: 6})
	g.Observe(time.Second, Overloaded)
	if g.Limit() != 3 {
		t.Fatalf("limit %d after a 5xx, want 3", g.Limit())
	}
	g.Observe(time.Second, Overloaded) // inside the cooldown: counts once
	if g.Limit() != 3 {
		t.Fatalf("limit %d, a second error inside the cooldown cut again", g.Limit())
	}
	clk.advance(10 * time.Second)
	g.Observe(time.Second, Overloaded)
	clk.advance(10 * time.Second)
	g.Observe(time.Second, Overloaded)
	if g.Limit() != 1 {
		t.Fatalf("limit %d, want the floor of 1", g.Limit())
	}
}

func TestGateNarrowsWhenTheServerSlowsDown(t *testing.T) {
	// The 3 Nov 2026 stall: replies that usually start in ~50 ms took seconds.
	g, clk, _ := newTestGate(GateConfig{Max: 6})
	gateObserveN(g, clk, 20, 50*time.Millisecond, OK)
	if g.Limit() != 6 {
		t.Fatalf("healthy replies changed the limit to %d", g.Limit())
	}
	gateObserveN(g, clk, 20, 3*time.Second, OK)
	if g.Limit() >= 6 {
		t.Fatalf("limit stayed at %d with replies 60x slower", g.Limit())
	}
}

func TestGateCeilingCatchesALateStarter(t *testing.T) {
	// A client that starts while the server is already struggling learns slow
	// replies as its baseline; the 1 s ceiling still narrows it.
	g, clk, _ := newTestGate(GateConfig{Max: 6})
	gateObserveN(g, clk, 20, 2*time.Second, OK)
	if g.Limit() >= 6 {
		t.Fatalf("limit %d with every reply over the ceiling", g.Limit())
	}
}

func TestGateWidensAgainSlowly(t *testing.T) {
	g, clk, _ := newTestGate(GateConfig{Max: 6, RecoverEvery: 10 * time.Second})
	gateObserveN(g, clk, 10, 50*time.Millisecond, OK)
	g.Observe(time.Second, Overloaded) // -> 3
	// 20 s of healthy replies: two steps, not straight back to 6.
	gateObserveN(g, clk, 200, 100*time.Millisecond, OK)
	if got := g.Limit(); got != 5 {
		t.Fatalf("after 20 s healthy, limit %d, want 5", got)
	}
	gateObserveN(g, clk, 600, 100*time.Millisecond, OK)
	if got := g.Limit(); got != 6 {
		t.Fatalf("after a minute healthy, limit %d, want the 6 ceiling", got)
	}
}

func TestGateWideningWakesAWaiter(t *testing.T) {
	g, clk, _ := newTestGate(GateConfig{Max: 2, RecoverEvery: time.Second})
	gateObserveN(g, clk, 10, 50*time.Millisecond, OK)
	g.Observe(time.Second, Overloaded) // -> 1
	if err := g.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := make(chan struct{})
	go func() {
		if err := g.Acquire(context.Background()); err == nil {
			close(got)
		}
	}()
	time.Sleep(20 * time.Millisecond)
	gateObserveN(g, clk, 40, 50*time.Millisecond, OK) // recovers to 2
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("raising the limit did not start the waiting download")
	}
}

func TestGateLogsChanges(t *testing.T) {
	g, clk, sink := newTestGate(GateConfig{Max: 6, Label: "example.com"})
	g.Observe(time.Second, Overloaded)
	if sink.count() != 1 {
		t.Fatalf("%d log lines after a cut, want 1", sink.count())
	}
	if sink.lines[0].msg != "Fewer downloads at once" {
		t.Fatalf("log line %q", sink.lines[0].msg)
	}
	clk.advance(time.Second)
	g.Observe(time.Second, Neutral)
	if sink.count() != 1 {
		t.Fatal("a neutral outcome logged a line")
	}
}
