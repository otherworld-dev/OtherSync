package agent

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/otherworld/nimbo/internal/transfer"
)

// TestConflictWaitingLogged checks a waiting conflict is logged once when it
// starts and once when it stops, not on every pass that re-detects it (Deck
// d1150: the log showed the repeating pass but never the file behind it).
func TestConflictWaitingLogged(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	e := &Engine{conflicts: make(map[string][]ConflictItem)}
	p := Pair{LocalDir: `C:\sync`}
	a := transfer.ConflictInfo{Path: "dir/a.md", Kind: "edited"}
	b := transfer.ConflictInfo{Path: "dir/b.md", Kind: "edited"}

	count := func(msg, path string) int {
		n := 0
		for _, l := range strings.Split(buf.String(), "\n") {
			if strings.Contains(l, `msg="`+msg+`"`) && strings.Contains(l, "path="+path) {
				n++
			}
		}
		return n
	}

	e.setConflicts(p, []transfer.ConflictInfo{a})
	e.setConflicts(p, []transfer.ConflictInfo{a}) // re-detected on the next pass
	e.setConflicts(p, []transfer.ConflictInfo{a, b})
	if got := count("conflict waiting for a decision", "dir/a.md"); got != 1 {
		t.Fatalf("a logged as waiting %d times, want 1\n%s", got, buf.String())
	}
	if got := count("conflict waiting for a decision", "dir/b.md"); got != 1 {
		t.Fatalf("b logged as waiting %d times, want 1\n%s", got, buf.String())
	}

	e.removeConflict(p.LocalDir, "dir/a.md")     // the user resolved it in Status
	e.setConflicts(p, []transfer.ConflictInfo{}) // b settled on its own
	if got := count("conflict no longer waiting", "dir/a.md"); got != 1 {
		t.Fatalf("a logged as cleared %d times, want 1\n%s", got, buf.String())
	}
	if got := count("conflict no longer waiting", "dir/b.md"); got != 1 {
		t.Fatalf("b logged as cleared %d times, want 1\n%s", got, buf.String())
	}
}
