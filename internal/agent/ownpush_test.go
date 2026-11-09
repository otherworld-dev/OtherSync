package agent

import (
	"context"
	"testing"
	"time"
)

// Deck d1103: the push notice for our own upload ran a whole remote-delta
// round. A notice whose files are all ours (the upload, plus the folder it went
// into) is skipped; anything else still runs.
func TestPushOfOurOwnUploadIsRecognised(t *testing.T) {
	const key = "pair"
	newEngine := func() *Engine {
		e := &Engine{triggers: map[string]chan struct{}{key: make(chan struct{}, 1)}}
		e.own.folderID = func(_ context.Context, p string) (uint64, bool) {
			if p == "Notes" {
				return 50, true
			}
			return 0, false
		}
		return e
	}
	ctx := context.Background()

	t.Run("the upload alone", func(t *testing.T) {
		e := newEngine()
		e.noteOwnWrite("Notes/list.json", "00004213ocrt2fp5q2bq")
		e.notePushed([]uint64{4213})
		if !e.pushIsOwn(ctx, key) {
			t.Fatal("a notice of only our upload was not recognised")
		}
		if e.pushIsOwn(ctx, key) {
			t.Fatal("the next push judged the last one's ids again")
		}
	})
	t.Run("the upload and its folder", func(t *testing.T) {
		e := newEngine()
		e.noteOwnWrite("Notes/list.json", "4213")
		e.notePushed([]uint64{4213, 50})
		if !e.pushIsOwn(ctx, key) {
			t.Fatal("the folder the upload went into was not recognised")
		}
	})
	t.Run("someone else's file too", func(t *testing.T) {
		e := newEngine()
		e.noteOwnWrite("Notes/list.json", "4213")
		e.notePushed([]uint64{4213})
		e.notePushed([]uint64{77})
		if e.pushIsOwn(ctx, key) {
			t.Fatal("a change of somebody else's was skipped")
		}
	})
	t.Run("a different folder", func(t *testing.T) {
		e := newEngine()
		e.noteOwnWrite("Notes/list.json", "4213")
		e.notePushed([]uint64{4213, 99})
		if e.pushIsOwn(ctx, key) {
			t.Fatal("an id that is not the upload's folder was skipped")
		}
	})
	t.Run("a notice without ids", func(t *testing.T) {
		e := newEngine()
		e.noteOwnWrite("Notes/list.json", "4213")
		e.notePushed([]uint64{4213})
		e.notePushed(nil)
		if e.pushIsOwn(ctx, key) {
			t.Fatal("a notice that did not say what changed was skipped")
		}
	})
	t.Run("a sync asked for in the same window", func(t *testing.T) {
		e := newEngine()
		e.noteOwnWrite("Notes/list.json", "4213")
		e.notePushed([]uint64{4213})
		e.TriggerSync()
		if e.pushIsOwn(ctx, key) {
			t.Fatal("TriggerSync's round was skipped as our own upload")
		}
	})
	t.Run("an old upload", func(t *testing.T) {
		e := newEngine()
		e.noteOwnWrite("Notes/list.json", "4213")
		e.own.writes[4213] = ownWrite{path: "Notes/list.json", at: time.Now().Add(-ownWriteTTL - time.Second)}
		e.notePushed([]uint64{4213})
		if e.pushIsOwn(ctx, key) {
			t.Fatal("an upload past its time was still taken as ours")
		}
	})
	t.Run("nothing reported", func(t *testing.T) {
		if newEngine().pushIsOwn(ctx, key) {
			t.Fatal("a round with nothing reported was skipped")
		}
	})
}
