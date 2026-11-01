package main

import (
	"fmt"
	"image/png"
	"os"
	"testing"
)

// TestRenderTrayPreview writes the tray states to .icon-preview for visual
// inspection, at 16 and 32 px. Not a real test; kept handy for icon work.
func TestRenderTrayPreview(t *testing.T) {
	if os.Getenv("NIMBO_ICON_PREVIEW") == "" {
		t.Skip("set NIMBO_ICON_PREVIEW=1 to render")
	}
	_ = os.MkdirAll("../../.icon-preview", 0o755)
	for _, st := range []string{"idle", "sync", "paused", "error", "warning"} {
		for _, size := range []int{16, 32} {
			f, err := os.Create(fmt.Sprintf("../../.icon-preview/tray-%s-%d.png", st, size))
			if err != nil {
				t.Fatal(err)
			}
			_ = png.Encode(f, trayImage(st, 3, st == "idle", size))
			f.Close()
		}
	}
}
