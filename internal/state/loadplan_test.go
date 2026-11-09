package state

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/otherworld/nimbo/internal/engine"
)

func TestUseTableScan(t *testing.T) {
	for _, c := range []struct {
		pairRows int
		maxRowid int64
		want     bool
	}{
		{352312, 352338, true}, // the desktop: one pair is the whole table
		{1000, 4000, true},     // exactly a quarter
		{1000, 4001, false},
		{999, 999, false}, // too small to matter
		{0, 0, false},
		{50000, 1000000, false}, // a small pair in a big table stays on the index
	} {
		if got := useTableScan(c.pairRows, c.maxRowid); got != c.want {
			t.Errorf("useTableScan(%d, %d) = %v, want %v", c.pairRows, c.maxRowid, got, c.want)
		}
	}
}

// TestFullLoadBothPlans checks a full load returns exactly the pair's own rows
// whichever way it reads them: the table pass must still filter by account and
// pair, since other accounts' and pairs' rows share the table (Deck d1102).
func TestFullLoadBothPlans(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "state.db")
	st, err := Open(dbPath, "acct", false)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	other, err := Open(dbPath, "other", false) // a second account in the same file
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()

	insert := func(s *Store, pairKey string, n int, tag string) {
		t.Helper()
		tx, err := s.db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < n; i++ {
			if _, err := tx.Exec(
				`INSERT INTO baseline (account_id, pair_key, path, is_dir, remote_etag, remote_fileid, local_size, local_mtime_nanos, content_sha1, mount_root, content_key)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				s.accountID, pairKey, fmt.Sprintf("d%d/f%d.txt", i%7, i), i%5 == 0, tag+fmt.Sprint(i), "id", i, int64(i)*10, "", i%11 == 0, "",
			); err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	insert(st, "BIG", 3000, "big")
	insert(st, "SMALL", 40, "small")
	insert(other, "BIG", 200, "foreign") // same pair key, another account

	for _, c := range []struct {
		pair     string
		n        int
		tag      string
		wantScan bool
	}{
		{"BIG", 3000, "big", true},
		{"SMALL", 40, "small", false},
	} {
		rows, scan, err := st.fullLoadPlan(c.pair)
		if err != nil {
			t.Fatal(err)
		}
		if rows != c.n || scan != c.wantScan {
			t.Fatalf("%s plan: rows=%d scan=%v, want %d/%v", c.pair, rows, scan, c.n, c.wantScan)
		}
		got, err := st.LoadBaseline(c.pair)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != c.n {
			t.Fatalf("%s: loaded %d rows, want %d", c.pair, len(got), c.n)
		}
		for i := 0; i < c.n; i++ {
			p := fmt.Sprintf("d%d/f%d.txt", i%7, i)
			b := got[p]
			want := engine.BaselineState{Path: p, IsDir: i%5 == 0, RemoteETag: c.tag + fmt.Sprint(i), RemoteFileID: "id",
				LocalSize: int64(i), LocalMTimeNanos: int64(i) * 10, MountRoot: i%11 == 0}
			if b != want {
				t.Fatalf("%s %s: got %+v, want %+v", c.pair, p, b, want)
			}
		}
	}

	// The other account's copy of BIG is read through the index (200 of 3240
	// rows) and must not pick up this account's rows either.
	got, err := other.LoadBaseline("BIG")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 200 || got["d0/f0.txt"].RemoteETag != "foreign0" {
		t.Fatalf("other account: %d rows, %+v", len(got), got["d0/f0.txt"])
	}
}
