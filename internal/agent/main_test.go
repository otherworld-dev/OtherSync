package agent

import (
	"os"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	// No random pause before whole-tree crawls in tests (stagger.go).
	crawlStagger = func() time.Duration { return 0 }
	os.Exit(m.Run())
}
