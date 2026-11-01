package main

import (
	"runtime"
	"testing"
)

// The tray follows the flyout's composed status line (Deck #988), so it never
// says idle while the flyout says "Checking folders…". Items that need the
// user (can't sync, conflicts, no longer shared) put it in a warning state, so
// they can't go unnoticed with the flyout closed (Deck #984).
func TestTrayStateFor(t *testing.T) {
	for state, want := range map[string]string{
		"idle":      "idle",
		"busy":      "sync",
		"attention": "warning",
		"offline":   "error",
		"paused":    "paused",
		"":          "idle",
	} {
		if got := trayStateFor(state); got != want {
			t.Errorf("trayStateFor(%q) = %q, want %q", state, got, want)
		}
	}
}

// The warning badge is amber, not the error red.
func TestTrayIconWarningIsAmber(t *testing.T) {
	img := trayImage("warning", 0, false, 32)
	// Left of the "!" stroke, inside the badge disc.
	r, g, b, _ := img.At(21, 24).RGBA()
	if c := (trayAmber); uint8(r>>8) != c.R || uint8(g>>8) != c.G || uint8(b>>8) != c.B {
		t.Errorf("badge pixel = %02x%02x%02x, want amber %02x%02x%02x", r>>8, g>>8, b>>8, c.R, c.G, c.B)
	}
}

// On Windows the tray gets a multi-size .ico with a frame drawn at every
// small-icon size, so the shell never shrinks the 32 px one into the 16 slot.
func TestTrayIconCarriesEverySmallIconSize(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the tray .ico is Windows-only")
	}
	frames := decodeICOFrames(trayIcon("idle", 0, true))
	for _, s := range traySizes {
		img, ok := frames[s]
		if !ok {
			t.Fatalf("tray icon has no %d px frame", s)
		}
		if img.Bounds().Dx() != s {
			t.Errorf("%d px frame is %d wide", s, img.Bounds().Dx())
		}
	}
}

// The brand frames come from the embedded .ico, drawn natively at each size;
// a size it lacks is resampled from the nearest larger frame.
func TestBrandFrame(t *testing.T) {
	for _, s := range []int{16, 20, 24, 32, 40, 48, 64, 256} {
		if _, ok := loadBrandFrames()[s]; !ok {
			t.Errorf("embedded brand .ico has no %d px frame", s)
		}
	}
	for _, s := range []int{12, 18, 100, 300} {
		if img := brandFrame(s); img == nil || img.Bounds().Dx() != s {
			t.Errorf("brandFrame(%d) = %v", s, img)
		}
	}
}
