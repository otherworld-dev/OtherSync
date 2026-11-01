package agent

// A random pause before a crawl that lists a whole server tree (Deck #1001).
//
// The pacer is per client, so it cannot stop a crowd: a company rolling the
// app out to fifty PCs on the same morning, or every client coming back after
// a server outage, would start their first syncs together and each open with
// a full burst. Waiting a random 2–30 s first spreads them out, so the server
// sees them arrive over half a minute instead of at once.
//
// Only the crawls that list everything wait: a first sync (cloneRemote) and
// the one-off folder check (relist). Ordinary passes, Explorer browsing and
// the on-demand set-up the user is watching do not.

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"time"
)

const (
	staggerMin = 2 * time.Second
	staggerMax = 30 * time.Second
)

// crawlStagger returns how long to wait before a whole-tree crawl. A variable
// so tests can turn it off.
var crawlStagger = func() time.Duration {
	return staggerMin + rand.N(staggerMax-staggerMin)
}

// staggerCrawl waits a random moment before a whole-tree crawl, showing why in
// the status line. It returns early with ctx's error if the engine stops.
func (e *Engine) staggerCrawl(ctx context.Context, what string) error {
	d := crawlStagger()
	if d <= 0 {
		return nil
	}
	slog.Info("waiting a moment before listing every server folder", "for", what, "wait", d.Round(time.Second))
	e.noteCrawlWait(time.Now().Add(d)) // shown in "Right now" with a countdown
	defer e.noteCrawlWait(time.Time{})
	e.status("Starting shortly…")
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
