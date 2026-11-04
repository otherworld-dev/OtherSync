package pace

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Gate caps how many downloads an account runs at once, and lowers that cap
// when the server shows strain (Deck #1055).
//
// Why it exists: every download keeps one of the server's PHP workers busy
// for as long as it streams, and a first sync ran 32 of them at once. On
// 3 Nov 2026 a laptop's first live-mode sync did that against a server whose
// php-fpm pool had been capped at 16 workers two days before: every worker
// was serving the laptop, so everything else queued. status.php took 244 s,
// the desktop's routine remote scan went from under a second to 4m40s, and
// the web UI stopped answering. Most of the files were small (12,000 of
// 14,000 under 64 KB), so a rate on bytes would not have helped: what costs
// the server is requests in flight.
//
// The listing Pacer limits a RATE; a download can run for minutes, so the
// Gate limits a COUNT instead, with the same latency test (latencyJudge)
// deciding when to narrow:
//
//   - Max 6 at once. That leaves most of a small pool (16 workers here) to the
//     web UI, phones and other PCs, and still fills a LAN link with big files.
//   - The latency is the time until the server starts answering (headers
//     received), not the whole transfer: that is PHP start-up plus any wait
//     for a free worker, which is exactly what grows when the pool is full,
//     and it does not depend on how big the file is.
//   - Slow replies (same thresholds as listings, 1 s ceiling included) cut
//     the cap to 70%, errors and timeouts halve it; at most one cut per 5 s.
//   - Floor 1, so a struggling server still sees the sync finish.
//   - Recovery: one more download per 10 s of healthy replies, up to Max.
//
// A narrowed cap never stops a download that is already running; new ones
// wait until the count is under it.
type Gate struct {
	cfg GateConfig

	mu       sync.Mutex
	limit    int
	inFlight int
	wake     chan struct{} // closed (and replaced) when a slot may have opened

	judge      latencyJudge
	lastCut    time.Time
	lastChange time.Time

	lastLog     time.Time
	loggedLimit int
	pendingWhy  string
}

// Defaults; see the Gate comment for why.
const (
	DefaultGateMax = 6
	DefaultGateMin = 1
)

// GateConfig tunes a Gate; zero fields take the defaults (the latency
// thresholds and timings are the listing Pacer's).
type GateConfig struct {
	Max           int // ceiling: downloads at once
	Min           int // floor
	SlowFactor    float64
	SlowMargin    time.Duration
	SlowCeiling   time.Duration
	HealthyFactor float64
	CutCooldown   time.Duration
	RecoverEvery  time.Duration
	LogEvery      time.Duration
	Label         string // identifies the server in log lines
	Clock         Clock
	Log           func(msg string, args ...any) // default slog.Info
}

func (c GateConfig) withDefaults() GateConfig {
	if c.Max <= 0 {
		c.Max = DefaultGateMax
	}
	if c.Min <= 0 {
		c.Min = DefaultGateMin
	}
	if c.Min > c.Max {
		c.Min = c.Max
	}
	if c.SlowFactor <= 0 {
		c.SlowFactor = DefaultSlowFactor
	}
	if c.SlowMargin <= 0 {
		c.SlowMargin = DefaultSlowMargin
	}
	if c.SlowCeiling <= 0 {
		c.SlowCeiling = DefaultSlowCeiling
	}
	if c.HealthyFactor <= 0 {
		c.HealthyFactor = DefaultHealthyFactor
	}
	if c.CutCooldown <= 0 {
		c.CutCooldown = DefaultCutCooldown
	}
	if c.RecoverEvery <= 0 {
		c.RecoverEvery = DefaultRecoverEvery
	}
	if c.LogEvery <= 0 {
		c.LogEvery = DefaultLogEvery
	}
	if c.Clock == nil {
		c.Clock = realClock{}
	}
	if c.Log == nil {
		c.Log = slog.Info
	}
	return c
}

// NewGate returns a gate open to Max downloads.
func NewGate(cfg GateConfig) *Gate {
	cfg = cfg.withDefaults()
	return &Gate{cfg: cfg, limit: cfg.Max, loggedLimit: cfg.Max, wake: make(chan struct{})}
}

// Now reads the gate's clock, for measuring a reply's latency.
func (g *Gate) Now() time.Time {
	if g == nil {
		return time.Now()
	}
	return g.cfg.Clock.Now()
}

// Limit returns how many downloads may run at once now (0 for nil).
func (g *Gate) Limit() int {
	if g == nil {
		return 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.limit
}

// InFlight returns how many downloads hold a slot.
func (g *Gate) InFlight() int {
	if g == nil {
		return 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.inFlight
}

// Acquire blocks until a download may start, or ctx ends. Every nil return
// must be paired with exactly one Release, once the download is finished.
func (g *Gate) Acquire(ctx context.Context) error {
	if g == nil {
		return nil
	}
	for {
		g.mu.Lock()
		if g.inFlight < g.limit {
			g.inFlight++
			g.mu.Unlock()
			return nil
		}
		ch := g.wake
		g.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

// Release gives back a slot taken by Acquire.
func (g *Gate) Release() {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.inFlight > 0 {
		g.inFlight--
	}
	g.signal()
}

// signal wakes every waiter to re-check. Callers hold mu.
func (g *Gate) signal() {
	close(g.wake)
	g.wake = make(chan struct{})
}

// Observe reports how long the server took to start answering a download, and
// what the reply said about its load.
func (g *Gate) Observe(latency time.Duration, o Outcome) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	now := g.cfg.Clock.Now()
	switch o {
	case Overloaded:
		g.cut(now, overloadCut, "the server returned errors or timed out")
	case OK:
		jp := judgeParams{SlowFactor: g.cfg.SlowFactor, SlowMargin: g.cfg.SlowMargin,
			SlowCeiling: g.cfg.SlowCeiling, HealthyFactor: g.cfg.HealthyFactor}
		v, m := g.judge.add(latency, jp)
		switch {
		case v == slow:
			g.cut(now, latencyCut, "the server is slow to start answering (typically "+
				m.Round(time.Millisecond).String()+", usually "+g.judge.baseline.Round(time.Millisecond).String()+")")
		case v == healthy && g.limit < g.cfg.Max && now.Sub(g.lastChange) >= g.cfg.RecoverEvery:
			g.limit++
			g.lastChange = now
			g.pendingWhy = "the server is answering normally again"
			g.signal()
		}
	}
	g.maybeLog(now)
}

// cut lowers the limit by factor, at most once per CutCooldown. Callers hold mu.
func (g *Gate) cut(now time.Time, factor float64, why string) {
	if !g.lastCut.IsZero() && now.Sub(g.lastCut) < g.cfg.CutCooldown {
		return
	}
	nl := int(float64(g.limit) * factor)
	if nl < g.cfg.Min {
		nl = g.cfg.Min
	}
	g.lastCut = now
	g.lastChange = now
	g.judge.reset()
	if nl == g.limit {
		return // already at the floor
	}
	g.limit = nl
	g.pendingWhy = why
}

// maybeLog writes one line when the limit differs from the last one logged and
// LogEvery has passed since the last line. Callers hold mu.
func (g *Gate) maybeLog(now time.Time) {
	if g.limit == g.loggedLimit {
		return
	}
	if !g.lastLog.IsZero() && now.Sub(g.lastLog) < g.cfg.LogEvery {
		return
	}
	msg := "More downloads at once"
	if g.limit < g.loggedLimit {
		msg = "Fewer downloads at once"
	}
	args := []any{"at_once", g.limit, "was", g.loggedLimit, "max", g.cfg.Max, "why", g.pendingWhy}
	if g.cfg.Label != "" {
		args = append(args, "server", g.cfg.Label)
	}
	g.cfg.Log(msg, args...)
	g.lastLog = now
	g.loggedLimit = g.limit
}
