// Package pace limits how fast Nimbo lists folders on a server, and slows the
// listing down further when the server shows strain (Deck #1001).
//
// Why it exists: a remote scan lists every changed folder with one PROPFIND.
// Concurrency was already capped (4 workers, after the July 2026 incident),
// but nothing capped the RATE: against a server that answers quickly, four
// workers ran at about 50 PROPFINDs a second, and kept that up for as long as
// the tree lasted. On 31 Oct 2026 a cold on-demand setup listing of a
// 46,076-folder account sent ~46,000 PROPFINDs an hour to a small Nextcloud
// host (an LXC with ~23 PHP workers) and took it down; the retry/backoff only
// reacts to errors, and latency climbs long before the first 5xx.
//
// A Pacer is a token bucket plus AIMD adaptation, one per account (owned by the
// transport.Client), shared by every listing that account runs, so two scans
// at once split the rate instead of doubling it.
//
// The numbers (all overridable through Config; tests use their own):
//
//   - Rate 6 PROPFINDs/s. The 31 Oct crawl was latency-bound at ~13/s with 4
//     workers, i.e. that host was saturated at about that rate (~300 ms of PHP
//     and database work per listing). 6/s keeps it under half that, and still
//     lists a 46k-folder tree in about 2h08m (46,076 / 6 s): long for a
//     one-off cold crawl, but it is background work and the server stays up.
//   - Burst 60. A normal delta poll does ~10-60 PROPFINDs; the bucket holds
//     enough for one to run at full speed (still bounded by the 4 workers),
//     and refills in 10 s, far inside the 5-minute poll interval. Interactive
//     single listings (Explorer opening a folder) never notice the pacer
//     unless a big crawl is draining it, and then wait at most a few hundred
//     milliseconds.
//   - Slow-down on latency: the median of the last 16 successful listings is
//     compared with a baseline (the lowest median seen, creeping up very
//     slowly so a permanently slower server is eventually accepted). A median
//     above 3x the baseline AND more than 250 ms above it cuts the rate to
//     70%. The median, not the mean, so a few big folders (a listing of 5,000
//     files takes seconds on a healthy server) don't count as strain; the
//     250 ms margin so a fast LAN's 5 ms -> 30 ms wobble doesn't either.
//   - Slow-down on a fixed ceiling: a median over 1 s cuts the rate however
//     slow the baseline is. The baseline is per client, so a PC that starts
//     its first sync while other clients are already loading the server
//     learns the slow answers as normal and would never back off on the
//     relative test; the ceiling catches it. A healthy Nextcloud lists a
//     typical folder in well under a second, and the median keeps the odd
//     big folder from tripping it.
//   - Slow-down on errors: a 5xx/429, an exhausted retry or a timeout halves
//     the rate; a Retry-After also stops all listings for that long (capped at
//     5 minutes). At most one cut per 5 s, so four workers failing at once
//     count once, and the latency window restarts after each cut (the next
//     judgement is on responses at the new rate).
//   - Floor 0.5/s, so a struggling server still sees the scan finish.
//   - Recovery: +0.5/s per 10 s of healthy responses, back up to Rate. From
//     one halving (3/s) that is a minute; from the floor about two.
//
// When the pace changes, a plain line goes to the log at most every 30 s
// ("Slowing down server folder listings …" / "Speeding up …"), so an incident
// like 31 Oct shows up in nimbo.log.
package pace

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"
)

// Defaults; see the package comment for why.
const (
	DefaultRate          = 6.0
	DefaultBurst         = 60
	DefaultMinRate       = 0.5
	DefaultSlowFactor    = 3.0
	DefaultSlowMargin    = 250 * time.Millisecond
	DefaultSlowCeiling   = time.Second
	DefaultHealthyFactor = 1.5
	DefaultCutCooldown   = 5 * time.Second
	DefaultRecoverEvery  = 10 * time.Second
	DefaultRecoverStep   = 0.5
	DefaultMaxHold       = 5 * time.Minute
	DefaultLogEvery      = 30 * time.Second

	latencyWindow = 16    // successful listings the median is taken over
	minSamples    = 8     // listings needed before latency is judged at all
	latencyCut    = 0.7   // multiplier when responses slow down
	overloadCut   = 0.5   // multiplier on 5xx/429/timeout/Retry-After
	baselineCreep = 0.002 // per observation: how fast the baseline follows a slower median
)

// Outcome classifies a finished request for the adaptation.
type Outcome int

const (
	// OK: the server answered normally; its latency feeds the baseline.
	OK Outcome = iota
	// Overloaded: 5xx, 429, a timeout or an exhausted retry — server strain.
	Overloaded
	// Neutral: says nothing about load (404, a cancelled request, offline).
	Neutral
)

// Clock is the pacer's time source; tests inject a virtual one.
type Clock interface {
	Now() time.Time
	Sleep(ctx context.Context, d time.Duration) error
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }
func (realClock) Sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Config tunes a Pacer; zero fields take the Default values.
type Config struct {
	Rate          float64 // ceiling, requests per second
	Burst         int     // bucket size
	MinRate       float64 // floor
	SlowFactor    float64 // median above baseline*SlowFactor ...
	SlowMargin    time.Duration
	SlowCeiling   time.Duration // a median above this is slow whatever the baseline
	HealthyFactor float64       // median at or under baseline*HealthyFactor counts as healthy
	CutCooldown   time.Duration
	RecoverEvery  time.Duration
	RecoverStep   float64
	MaxHold       time.Duration
	LogEvery      time.Duration
	Label         string // identifies the server in log lines
	Clock         Clock
	Log           func(msg string, args ...any) // default slog.Info
}

func (c Config) withDefaults() Config {
	if c.Rate <= 0 {
		c.Rate = DefaultRate
	}
	if c.Burst <= 0 {
		c.Burst = DefaultBurst
	}
	if c.MinRate <= 0 {
		c.MinRate = DefaultMinRate
	}
	if c.MinRate > c.Rate {
		c.MinRate = c.Rate
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
	if c.RecoverStep <= 0 {
		c.RecoverStep = DefaultRecoverStep
	}
	if c.MaxHold <= 0 {
		c.MaxHold = DefaultMaxHold
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

func (c Config) judgeParams() judgeParams {
	return judgeParams{SlowFactor: c.SlowFactor, SlowMargin: c.SlowMargin, SlowCeiling: c.SlowCeiling, HealthyFactor: c.HealthyFactor}
}

// Pacer is safe for concurrent use. A nil *Pacer paces nothing.
type Pacer struct {
	cfg Config

	mu     sync.Mutex
	rate   float64
	tokens float64   // may go negative: outstanding reservations
	last   time.Time // tokens are accounted up to here; in the future during a hold

	judge latencyJudge

	lastCut    time.Time
	lastChange time.Time

	lastLog    time.Time
	loggedRate float64
	pendingWhy string
}

// New returns a pacer with a full bucket.
func New(cfg Config) *Pacer {
	cfg = cfg.withDefaults()
	return &Pacer{
		cfg:        cfg,
		rate:       cfg.Rate,
		tokens:     float64(cfg.Burst),
		last:       cfg.Clock.Now(),
		loggedRate: cfg.Rate,
	}
}

// Now reads the pacer's clock, for measuring a request's latency.
func (p *Pacer) Now() time.Time {
	if p == nil {
		return time.Now()
	}
	return p.cfg.Clock.Now()
}

// Rate returns the current allowed rate in requests per second (0 for nil).
func (p *Pacer) Rate() float64 {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.rate
}

// refill credits tokens earned since p.last at the current rate. Callers hold mu.
func (p *Pacer) refill(now time.Time) {
	if !now.After(p.last) {
		return // during a hold p.last is in the future: nothing earned yet
	}
	p.tokens += now.Sub(p.last).Seconds() * p.rate
	if b := float64(p.cfg.Burst); p.tokens > b {
		p.tokens = b
	}
	p.last = now
}

// Wait blocks until one request may be sent, or ctx ends. Waiters take turns:
// each reserves the next slot, so concurrent scans share one rate.
func (p *Pacer) Wait(ctx context.Context) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	now := p.cfg.Clock.Now()
	p.refill(now)
	p.tokens--
	var delay time.Duration
	if p.last.After(now) {
		delay = p.last.Sub(now) // held (Retry-After)
	}
	if p.tokens < 0 {
		delay += time.Duration(-p.tokens / p.rate * float64(time.Second))
	}
	p.mu.Unlock()
	if delay <= 0 {
		return nil
	}
	if err := p.cfg.Clock.Sleep(ctx, delay); err != nil {
		p.mu.Lock()
		p.tokens++ // give the unused slot back
		p.mu.Unlock()
		return err
	}
	return nil
}

// Hold stops all listings for d (a server's Retry-After), capped at MaxHold,
// and counts as an overload signal.
func (p *Pacer) Hold(d time.Duration) {
	if p == nil || d <= 0 {
		return
	}
	if d > p.cfg.MaxHold {
		d = p.cfg.MaxHold
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.cfg.Clock.Now()
	p.refill(now)
	if until := now.Add(d); until.After(p.last) {
		p.last = until
		if p.tokens > 0 {
			p.tokens = 0
		}
	}
	p.cut(now, overloadCut, fmt.Sprintf("the server asked for a %s pause (Retry-After)", d.Round(time.Second)))
	p.maybeLog(now)
}

// Observe reports a finished request: how long it took and what it said about
// the server's load.
func (p *Pacer) Observe(latency time.Duration, o Outcome) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.cfg.Clock.Now()
	switch o {
	case Overloaded:
		p.cut(now, overloadCut, "the server returned errors or timed out")
	case OK:
		p.observeLatency(now, latency)
	}
	p.maybeLog(now)
}

func (p *Pacer) observeLatency(now time.Time, lat time.Duration) {
	v, m := p.judge.add(lat, p.cfg.judgeParams())
	switch {
	case v == slow:
		p.cut(now, latencyCut, fmt.Sprintf("the server is answering slowly (typically %s, usually %s)",
			m.Round(time.Millisecond), p.judge.baseline.Round(time.Millisecond)))
	case v == healthy && p.rate < p.cfg.Rate && now.Sub(p.lastChange) >= p.cfg.RecoverEvery:
		p.rate += p.cfg.RecoverStep
		if p.rate > p.cfg.Rate {
			p.rate = p.cfg.Rate
		}
		p.lastChange = now
		p.note("the server is answering normally again")
	}
}

// cut lowers the rate by factor, at most once per CutCooldown. Callers hold mu.
func (p *Pacer) cut(now time.Time, factor float64, why string) {
	if !p.lastCut.IsZero() && now.Sub(p.lastCut) < p.cfg.CutCooldown {
		return
	}
	p.refill(now) // settle tokens at the old rate first
	nr := p.rate * factor
	if nr < p.cfg.MinRate {
		nr = p.cfg.MinRate
	}
	p.lastCut = now
	p.lastChange = now
	// Judge the next window on responses at the new rate only.
	p.judge.reset()
	if nr == p.rate {
		return // already at the floor
	}
	p.rate = nr
	p.note(why)
}

func (p *Pacer) note(why string) { p.pendingWhy = why }

// maybeLog writes one line when the rate differs from the last one logged and
// LogEvery has passed since the last line. Callers hold mu.
func (p *Pacer) maybeLog(now time.Time) {
	if p.rate == p.loggedRate {
		return
	}
	if !p.lastLog.IsZero() && now.Sub(p.lastLog) < p.cfg.LogEvery {
		return
	}
	msg := "Speeding up server folder listings"
	if p.rate < p.loggedRate {
		msg = "Slowing down server folder listings"
	}
	args := []any{
		"rate", fmt.Sprintf("%.1f/s", p.rate),
		"was", fmt.Sprintf("%.1f/s", p.loggedRate),
		"max", fmt.Sprintf("%.1f/s", p.cfg.Rate),
		"why", p.pendingWhy,
	}
	if p.cfg.Label != "" {
		args = append(args, "server", p.cfg.Label)
	}
	p.cfg.Log(msg, args...)
	p.lastLog = now
	p.loggedRate = p.rate
}

func median(ds []time.Duration) time.Duration {
	s := append([]time.Duration(nil), ds...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s[len(s)/2]
}
