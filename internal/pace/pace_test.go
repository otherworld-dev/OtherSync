package pace

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// fakeClock is a virtual clock: Sleep advances it instantly, so the tests
// measure pacing in virtual time and never really wait.
type fakeClock struct {
	mu    sync.Mutex
	now   time.Time
	slept time.Duration
}

func newFakeClock() *fakeClock { return &fakeClock{now: time.Unix(1_700_000_000, 0)} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.advance(d)
	c.mu.Lock()
	c.slept += d
	c.mu.Unlock()
	return nil
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

type logLine struct {
	msg  string
	args []any
}

type logSink struct {
	mu    sync.Mutex
	lines []logLine
}

func (s *logSink) log(msg string, args ...any) {
	s.mu.Lock()
	s.lines = append(s.lines, logLine{msg, args})
	s.mu.Unlock()
}

func (s *logSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.lines)
}

func newTest(cfg Config) (*Pacer, *fakeClock, *logSink) {
	clk := newFakeClock()
	sink := &logSink{}
	cfg.Clock = clk
	cfg.Log = sink.log
	return New(cfg), clk, sink
}

func waitN(t *testing.T, p *Pacer, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := p.Wait(context.Background()); err != nil {
			t.Fatalf("Wait %d: %v", i, err)
		}
	}
}

// approx reports whether got is within 2% (plus a millisecond) of want.
func approx(got, want time.Duration) bool {
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	return diff <= want/50+time.Millisecond
}

func TestBurstIsFree(t *testing.T) {
	p, clk, _ := newTest(Config{Rate: 10, Burst: 20})
	waitN(t, p, 20)
	if clk.slept != 0 {
		t.Fatalf("the first %d requests should not wait, slept %v", 20, clk.slept)
	}
}

func TestRateCapAfterBurst(t *testing.T) {
	p, clk, _ := newTest(Config{Rate: 10, Burst: 20})
	waitN(t, p, 120)
	// 100 requests beyond the burst at 10/s.
	if want := 10 * time.Second; !approx(clk.slept, want) {
		t.Fatalf("slept %v, want about %v", clk.slept, want)
	}
}

func TestBucketRefillsWhileIdle(t *testing.T) {
	p, clk, _ := newTest(Config{Rate: 10, Burst: 20})
	waitN(t, p, 20)
	clk.advance(time.Hour) // a poll interval later the bucket is full again, not over-full
	before := clk.slept
	waitN(t, p, 30)
	if got, want := clk.slept-before, time.Second; !approx(got, want) {
		t.Fatalf("after idling, 30 requests slept %v, want about %v (burst 20 then 10 at 10/s)", got, want)
	}
}

func TestSharedAcrossGoroutines(t *testing.T) {
	// Two "scans" pulling from one pacer get the rate between them, not each.
	p, clk, _ := newTest(Config{Rate: 10, Burst: 10})
	var wg sync.WaitGroup
	for g := 0; g < 2; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				_ = p.Wait(context.Background())
			}
		}()
	}
	wg.Wait()
	// Sleeps overlap in real time but each one is the waiter's place in the
	// shared queue; the LAST reservation must sit (100-10)/10 = 9s out, so the
	// total virtual time the clock was pushed forward is at least that.
	if clk.slept < 9*time.Second {
		t.Fatalf("100 requests from two goroutines slept only %v; the cap is per pacer, want >= 9s", clk.slept)
	}
}

func TestCancelledWaitReturnsError(t *testing.T) {
	p, _, _ := newTest(Config{Rate: 1, Burst: 1})
	waitN(t, p, 1)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.Wait(ctx); err == nil {
		t.Fatal("Wait on a cancelled context with no token should fail")
	}
}

func TestNilPacerIsUnlimited(t *testing.T) {
	var p *Pacer
	if err := p.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	p.Observe(time.Second, Overloaded)
	p.Hold(time.Minute)
	if p.Rate() != 0 {
		t.Fatal("nil pacer rate should read 0")
	}
}

func observeN(p *Pacer, clk *fakeClock, n int, lat time.Duration, o Outcome) {
	for i := 0; i < n; i++ {
		clk.advance(lat)
		p.Observe(lat, o)
	}
}

func TestOverloadHalvesRate(t *testing.T) {
	p, _, sink := newTest(Config{Rate: 8, MinRate: 0.5})
	p.Observe(100*time.Millisecond, Overloaded)
	if got := p.Rate(); got != 4 {
		t.Fatalf("rate after a 5xx = %v, want 4", got)
	}
	if sink.count() != 1 {
		t.Fatalf("want one log line for the slow-down, got %d", sink.count())
	}
}

func TestCutsAreRateLimitedByCooldown(t *testing.T) {
	p, clk, _ := newTest(Config{Rate: 8, MinRate: 0.5, CutCooldown: 5 * time.Second})
	// A burst of failures from the in-flight workers counts once.
	for i := 0; i < 4; i++ {
		p.Observe(10*time.Millisecond, Overloaded)
	}
	if got := p.Rate(); got != 4 {
		t.Fatalf("4 simultaneous failures cut to %v, want a single halving to 4", got)
	}
	clk.advance(5 * time.Second)
	p.Observe(10*time.Millisecond, Overloaded)
	if got := p.Rate(); got != 2 {
		t.Fatalf("a failure after the cooldown should cut again: %v, want 2", got)
	}
}

func TestRateNeverBelowFloor(t *testing.T) {
	p, clk, _ := newTest(Config{Rate: 8, MinRate: 0.5, CutCooldown: time.Second})
	for i := 0; i < 20; i++ {
		clk.advance(time.Second)
		p.Observe(time.Second, Overloaded)
	}
	if got := p.Rate(); got != 0.5 {
		t.Fatalf("rate %v, want the 0.5 floor", got)
	}
}

func TestRisingLatencyCutsRate(t *testing.T) {
	p, clk, _ := newTest(Config{Rate: 8, MinRate: 0.5})
	observeN(p, clk, 20, 100*time.Millisecond, OK) // baseline ~100ms
	if got := p.Rate(); got != 8 {
		t.Fatalf("healthy latency changed the rate to %v", got)
	}
	observeN(p, clk, 20, 900*time.Millisecond, OK) // 9x slower, well past 3x and +250ms
	if got := p.Rate(); got >= 8 {
		t.Fatalf("rate stayed at %v despite latency 9x the baseline", got)
	}
}

func TestOccasionalBigDirectoryDoesNotCut(t *testing.T) {
	p, clk, _ := newTest(Config{Rate: 8, MinRate: 0.5})
	for i := 0; i < 64; i++ {
		lat := 100 * time.Millisecond
		if i%5 == 0 {
			lat = 4 * time.Second // a folder with thousands of files
		}
		clk.advance(lat)
		p.Observe(lat, OK)
	}
	if got := p.Rate(); got != 8 {
		t.Fatalf("one slow listing in five cut the rate to %v; the median should ignore it", got)
	}
}

func TestSmallLatencyWobbleDoesNotCut(t *testing.T) {
	// On a fast LAN the baseline can be 5ms; 30ms is 6x that but nothing a
	// server would notice. The absolute margin keeps it from counting.
	p, clk, _ := newTest(Config{Rate: 8, MinRate: 0.5})
	observeN(p, clk, 20, 5*time.Millisecond, OK)
	observeN(p, clk, 40, 30*time.Millisecond, OK)
	if got := p.Rate(); got != 8 {
		t.Fatalf("a 25ms wobble cut the rate to %v", got)
	}
}

func TestRecoversSlowlyWhenHealthy(t *testing.T) {
	p, clk, _ := newTest(Config{Rate: 8, MinRate: 0.5, RecoverEvery: 10 * time.Second, RecoverStep: 0.5})
	observeN(p, clk, 10, 100*time.Millisecond, OK)
	p.Observe(100*time.Millisecond, Overloaded) // -> 4
	if p.Rate() != 4 {
		t.Fatalf("setup: rate %v, want 4", p.Rate())
	}
	// 30s of healthy responses: three +0.5 steps, not straight back to 8.
	for i := 0; i < 300; i++ {
		clk.advance(100 * time.Millisecond)
		p.Observe(100*time.Millisecond, OK)
	}
	if got := p.Rate(); got < 5 || got > 5.5 {
		t.Fatalf("after 30s healthy, rate %v, want about 5.5 (additive recovery)", got)
	}
	// Long enough and it is back at the ceiling, never above.
	for i := 0; i < 3000; i++ {
		clk.advance(100 * time.Millisecond)
		p.Observe(100*time.Millisecond, OK)
	}
	if got := p.Rate(); got != 8 {
		t.Fatalf("after 5 minutes healthy, rate %v, want the 8 ceiling", got)
	}
}

func TestNeutralOutcomeIgnored(t *testing.T) {
	p, clk, _ := newTest(Config{Rate: 8})
	observeN(p, clk, 50, 10*time.Second, Neutral) // e.g. 404s, cancellations
	if got := p.Rate(); got != 8 {
		t.Fatalf("neutral outcomes changed the rate to %v", got)
	}
}

func TestHoldBlocksAndCuts(t *testing.T) {
	p, clk, _ := newTest(Config{Rate: 10, Burst: 50})
	p.Hold(30 * time.Second) // Retry-After: 30
	if got := p.Rate(); got != 5 {
		t.Fatalf("rate after Retry-After = %v, want halved to 5", got)
	}
	before := clk.slept
	waitN(t, p, 1)
	if got := clk.slept - before; got < 30*time.Second {
		t.Fatalf("first request after Retry-After 30 waited only %v", got)
	}
}

func TestHoldIsCapped(t *testing.T) {
	p, clk, _ := newTest(Config{Rate: 10, MaxHold: 5 * time.Minute})
	p.Hold(24 * time.Hour)
	waitN(t, p, 1)
	if clk.slept > 5*time.Minute+time.Second {
		t.Fatalf("a hostile Retry-After held listings for %v; want capped at 5m", clk.slept)
	}
}

func TestLogLinesAreRateLimited(t *testing.T) {
	p, clk, sink := newTest(Config{Rate: 64, MinRate: 0.5, CutCooldown: time.Second, LogEvery: 30 * time.Second})
	for i := 0; i < 6; i++ { // a cut every second for 6s
		clk.advance(time.Second)
		p.Observe(time.Second, Overloaded)
	}
	if n := sink.count(); n != 1 {
		t.Fatalf("6 cuts in 6s logged %d lines, want 1 (rate-limited)", n)
	}
	// Once the interval passes, the next observation reports where it ended up.
	clk.advance(30 * time.Second)
	p.Observe(time.Second, Neutral)
	if n := sink.count(); n != 2 {
		t.Fatalf("after the log interval, want the pending change logged (2 lines), got %d", n)
	}
	last := fmt.Sprint(sink.lines[1].args...)
	if want := fmt.Sprintf("%.1f/s", p.Rate()); !contains(last, want) {
		t.Fatalf("flushed line %q does not report the current rate %s", last, want)
	}
	// Nothing new to say: no further line.
	clk.advance(time.Minute)
	p.Observe(time.Second, Neutral)
	if n := sink.count(); n != 2 {
		t.Fatalf("no change, but a line was logged anyway (%d lines)", n)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestDefaultsMatchTheDocumentedBudget(t *testing.T) {
	// The numbers the package comment justifies: a 46k-folder cold crawl
	// takes about two hours, a 60-PROPFIND delta fits in the burst.
	p := New(Config{})
	if p.Rate() != DefaultRate {
		t.Fatalf("default rate %v, want %v", p.Rate(), DefaultRate)
	}
	folders := 46076.0
	crawl := time.Duration(folders / DefaultRate * float64(time.Second))
	if crawl < time.Hour || crawl > 3*time.Hour {
		t.Fatalf("a 46k-folder crawl at the default rate takes %v, want 1-3h", crawl)
	}
	if DefaultBurst < 50 {
		t.Fatalf("burst %d does not cover a normal delta poll (~10-60 PROPFINDs)", DefaultBurst)
	}
}

// A client that starts while the server is already loaded learns the slow
// answers as its baseline, so the relative test never fires; the fixed
// ceiling still cuts the rate and keeps it from recovering.
func TestCeilingCatchesALateStarter(t *testing.T) {
	observe := func(p *Pacer, clk *fakeClock, n int, lat time.Duration) {
		for i := 0; i < n; i++ {
			clk.advance(200 * time.Millisecond)
			p.Observe(lat, OK)
		}
	}

	p, clk, _ := newTest(Config{})
	observe(p, clk, 16, 1500*time.Millisecond) // baseline is 1.5 s from the first sample
	if got := p.Rate(); got >= DefaultRate {
		t.Fatalf("rate after 1.5 s answers from the start = %.2f, want a cut below %.1f", got, DefaultRate)
	}
	cut := p.Rate()
	clk.advance(time.Minute)
	observe(p, clk, 16, 1500*time.Millisecond)
	if got := p.Rate(); got > cut {
		t.Fatalf("rate recovered to %.2f while answers stayed over the ceiling", got)
	}

	// Same history with the ceiling out of the way: the relative test alone
	// never notices, which is the gap the ceiling closes.
	q, qclk, _ := newTest(Config{SlowCeiling: time.Hour})
	observe(q, qclk, 16, 1500*time.Millisecond)
	if got := q.Rate(); got != DefaultRate {
		t.Fatalf("without the ceiling rate = %.2f, want it untouched at %.1f", got, DefaultRate)
	}
}

// Answers under the ceiling on a slow-but-steady server are left alone.
func TestCeilingLeavesSteadyAnswersAlone(t *testing.T) {
	p, clk, _ := newTest(Config{})
	for i := 0; i < 32; i++ {
		clk.advance(200 * time.Millisecond)
		p.Observe(700*time.Millisecond, OK)
	}
	if got := p.Rate(); got != DefaultRate {
		t.Fatalf("rate with steady 700 ms answers = %.2f, want %.1f", got, DefaultRate)
	}
}
