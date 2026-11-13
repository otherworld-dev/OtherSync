package agent

import (
	"context"
	"log/slog"
	"path"
	"sync"
	"time"

	"github.com/otherworld/nimbo/internal/transport"
)

// Deck d1103: every upload of ours brings the server's push notice for it a
// moment later, and that notice ran a whole remote-delta round (the baseline
// load, then the listing down to the file) only to find what we had just
// written. notify_push can say which files changed, so we remember the file
// ids our uploads got and skip a push round whose files were all ours.
//
// Nextcloud reports two ids for one upload: the file's, and its folder's,
// whose stored mtime the upload corrects. The folder's id is not in the
// upload's reply, so it is looked up (one small request) only when a push
// carries an id we don't know beside one of ours.

// ownWriteTTL is how long an upload stays recognisable. The notice comes
// within seconds (notify_push holds file notices back 1-15 s), but the round
// it triggers waits behind any pass already running.
const ownWriteTTL = 2 * time.Minute

// ownWrite is one upload of ours, by the file id the server gave it.
type ownWrite struct {
	path string // files-root-relative, as uploaded
	at   time.Time
}

// pushSeen is what the server reported for one pair since its last push round.
type pushSeen struct {
	ids     map[uint64]struct{}
	unknown bool // a notice without ids, or a trigger of our own: not skippable
}

type ownPush struct {
	mu     sync.Mutex
	writes map[uint64]ownWrite
	seen   map[string]*pushSeen // pair key -> reported since its last push round
	// folderID looks up a folder's file id; nil asks the server. Tests set it.
	folderID func(ctx context.Context, remotePath string) (uint64, bool)
}

// noteOwnWrite records an upload of ours (the transport's wrote hook).
func (e *Engine) noteOwnWrite(remotePath, fileID string) {
	id, ok := transport.NumericFileID(fileID)
	if !ok {
		return
	}
	now := time.Now()
	o := &e.own
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.writes == nil {
		o.writes = make(map[uint64]ownWrite)
	}
	for k, w := range o.writes {
		if now.Sub(w.at) > ownWriteTTL {
			delete(o.writes, k)
		}
	}
	o.writes[id] = ownWrite{path: remotePath, at: now}
}

// notePushed records, for every watched pair, what a push reported: the
// changed files' ids, or (ids nil) that something changed without saying what.
func (e *Engine) notePushed(ids []uint64) {
	e.watchMu.Lock()
	keys := make([]string, 0, len(e.triggers))
	for k := range e.triggers {
		keys = append(keys, k)
	}
	e.watchMu.Unlock()
	o := &e.own
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.seen == nil {
		o.seen = make(map[string]*pushSeen)
	}
	for _, k := range keys {
		s := o.seen[k]
		if s == nil {
			s = &pushSeen{ids: make(map[uint64]struct{})}
			o.seen[k] = s
		}
		if ids == nil {
			s.unknown = true
			continue
		}
		for _, id := range ids {
			s.ids[id] = struct{}{}
		}
	}
}

// pushIsOwn says whether everything the server reported for pair key since its
// last push round was our own uploads, so the round can be skipped. It takes
// what was reported: the next push starts afresh.
func (e *Engine) pushIsOwn(ctx context.Context, key string) bool {
	o := &e.own
	o.mu.Lock()
	s := o.seen[key]
	delete(o.seen, key)
	if s == nil || s.unknown || len(s.ids) == 0 {
		o.mu.Unlock()
		return false
	}
	now := time.Now()
	var mine, rest []uint64
	parents := make(map[string]struct{})
	for id := range s.ids {
		if w, ok := o.writes[id]; ok && now.Sub(w.at) <= ownWriteTTL {
			mine = append(mine, id)
			parents[parentDir(w.path)] = struct{}{}
		} else {
			rest = append(rest, id)
		}
	}
	lookup := o.folderID
	o.mu.Unlock()
	if len(mine) == 0 || len(rest) > len(parents) {
		return false // each upload accounts for at most its own folder
	}
	if len(rest) > 0 {
		if lookup == nil {
			lookup = e.remoteFolderID
		}
		left := make(map[uint64]struct{}, len(rest))
		for _, id := range rest {
			left[id] = struct{}{}
		}
		for p := range parents {
			if id, ok := lookup(ctx, p); ok {
				delete(left, id)
			}
		}
		if len(left) > 0 {
			slog.Info("push round runs; the server reported changes besides our upload", "ours", mine, "other", rest)
			return false
		}
	}
	slog.Debug("push reported only our own upload", "ids", mine, "folders", len(rest))
	return true
}

// remoteFolderID asks the server for a folder's file id.
func (e *Engine) remoteFolderID(ctx context.Context, remotePath string) (uint64, bool) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	ent, ok, err := e.client.Stat(ctx, remotePath)
	if err != nil || !ok {
		return 0, false
	}
	return transport.NumericFileID(ent.FileID)
}

// parentDir is a files-root-relative path's folder, "" for the root.
func parentDir(p string) string {
	d := path.Dir(p)
	if d == "." || d == "/" {
		return ""
	}
	return d
}
