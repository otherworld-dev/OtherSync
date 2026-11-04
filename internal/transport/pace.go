package transport

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/otherworld/nimbo/internal/pace"
)

// Listing pacing (Deck #1001). Each Client owns one pace.Pacer: every folder
// listing a scan makes for this account goes through PacedPropFind, so all
// concurrent scans share one rate, and the pacer slows that rate when the
// server slows down or errors. See package pace for the numbers.
//
// What is paced: the recursive listings (engine.RemoteScan for full and delta
// scans, the #949 relist, the on-demand adopt crawl, the escape sweep; the
// clone's listings; vfs reconcile and folder population through
// Engine.Browse). What is not: single-path Stat/depth-0 lookups (SyncPaths),
// transfers, OCS calls — those are not where the request floods came from, and
// pacing them would slow ordinary edits.

// newListPacer is the default pacer for a client of server.
func newListPacer(server string) *pace.Pacer {
	return pace.New(pace.Config{Label: hostLabel(server)})
}

// hostLabel names server in log lines: its host, or the URL as given.
func hostLabel(server string) string {
	if u, err := url.Parse(server); err == nil && u.Host != "" {
		return u.Host
	}
	return server
}

// Download gating (Deck #1055). Each Client also owns one pace.Gate: every
// sync download (transfer.Download, so the executor and a first sync's
// clone) goes through PacedGetFrom, which holds one of the gate's slots until
// the body is closed. A download keeps a PHP worker busy for as long as it
// streams, so the gate bounds how many of the server's workers this account
// can take, and narrows when the server is slow to start answering.
//
// What is not gated: on-demand hydration (agent.OpenRange/ReadFile), which an
// app opening a file is waiting on, and the CLI's one-off get.

// newDownloadGate is the default download gate for a client of server.
func newDownloadGate(server string) *pace.Gate {
	return pace.NewGate(pace.GateConfig{Label: hostLabel(server)})
}

// DownloadGate returns the client's download gate (nil when gating is disabled).
func (c *Client) DownloadGate() *pace.Gate { return c.downGate.Load() }

// SetDownloadGate replaces the download gate; nil disables gating. For tests
// and tuning: downloads already holding a slot of the old gate give it back
// to the old gate.
func (c *Client) SetDownloadGate(g *pace.Gate) { c.downGate.Store(g) }

// PacedGetFrom is GetFrom through the account's download gate: it waits for a
// slot, reports how long the server took to start answering (and whether it
// answered with an error) so the gate can adapt, and keeps the slot until the
// returned body is closed.
func (c *Client) PacedGetFrom(ctx context.Context, remotePath string, offset int64) (io.ReadCloser, http.Header, int, error) {
	g := c.DownloadGate()
	if err := g.Acquire(ctx); err != nil {
		return nil, nil, 0, err
	}
	start := g.Now()
	body, hdr, status, err := c.GetFrom(ctx, remotePath, offset)
	g.Observe(g.Now().Sub(start), paceOutcome(err))
	if err != nil {
		g.Release()
		return nil, nil, status, err
	}
	return &gatedBody{ReadCloser: body, release: g.Release}, hdr, status, nil
}

// gatedBody gives its gate slot back on the first Close.
type gatedBody struct {
	io.ReadCloser
	once    sync.Once
	release func()
}

func (b *gatedBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.release)
	return err
}

// ListPacer returns the client's listing pacer (nil when pacing is disabled).
func (c *Client) ListPacer() *pace.Pacer { return c.pacer.Load() }

// SetListPacer replaces the listing pacer; nil disables pacing. For tests and
// tuning: swapping it while listings run is safe but splits their accounting.
func (c *Client) SetListPacer(p *pace.Pacer) { c.pacer.Store(p) }

// PacedPropFind is PropFind through the account's listing pacer: it waits for
// a slot, then reports the response time and outcome back so the pace adapts.
// The scan's PropFinder seam picks this up automatically (engine.RemoteScan).
func (c *Client) PacedPropFind(ctx context.Context, remotePath string, depth int) ([]Entry, error) {
	return c.paced(ctx, true, func() ([]Entry, error) { return c.PropFind(ctx, remotePath, depth) })
}

// PacedPropFindRecursive is PropFindRecursive through the listing pacer. One
// Depth: infinity listing takes one slot, like any other listing. Its errors
// count as strain, but its response time does not feed the latency baseline:
// a whole-subtree listing legitimately takes minutes, and comparing that with
// one-folder listings would read as a server in trouble.
func (c *Client) PacedPropFindRecursive(ctx context.Context, remotePath string) ([]Entry, error) {
	return c.paced(ctx, false, func() ([]Entry, error) { return c.PropFindRecursive(ctx, remotePath) })
}

func (c *Client) paced(ctx context.Context, timed bool, list func() ([]Entry, error)) ([]Entry, error) {
	p := c.ListPacer()
	if err := p.Wait(ctx); err != nil {
		return nil, err
	}
	start := p.Now()
	entries, err := list()
	o := paceOutcome(err)
	if o == pace.OK && !timed {
		o = pace.Neutral
	}
	p.Observe(p.Now().Sub(start), o)
	return entries, err
}

// paceOutcome reads a listing's error as a load signal: server distress (5xx,
// 429, an exhausted retry, a timeout) slows the pace; refusals, missing paths
// and cancellations say nothing about load. Do retries network failures, so
// being offline also ends as an exhausted retry and slows the pace — harmless,
// it recovers once listings succeed again.
func paceOutcome(err error) pace.Outcome {
	switch {
	case err == nil:
		return pace.OK
	case errors.Is(err, context.Canceled), errors.Is(err, ErrNotFound):
		return pace.Neutral
	case errors.Is(err, ErrRetriesExhausted), errors.Is(err, context.DeadlineExceeded):
		return pace.Overloaded
	}
	if code := StatusCode(err); code != 0 {
		if code == http.StatusTooManyRequests || (code >= 500 && code != http.StatusNotImplemented && code != http.StatusInsufficientStorage) {
			return pace.Overloaded
		}
		return pace.Neutral
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return pace.Overloaded
	}
	return pace.Neutral
}

// notePushback hands a server's Retry-After on a 429/5xx — from ANY request,
// listing or not — to the listing pacer, so every scan worker stops for it,
// not just the one request Do happens to be retrying.
func (c *Client) notePushback(resp *http.Response) {
	if resp == nil || (resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500) {
		return
	}
	if d := retryAfterHint(resp.Header.Get("Retry-After"), time.Now()); d > 0 {
		c.ListPacer().Hold(d)
	}
}

// retryAfterHint parses either Retry-After form (delta-seconds or HTTP-date),
// uncapped (the pacer caps it). 0 = none or unparsable.
func retryAfterHint(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs <= 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := t.Sub(now); d > 0 {
			return d
		}
	}
	return 0
}
