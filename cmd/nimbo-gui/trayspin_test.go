package main

import (
	"bytes"
	"image"
	"image/color"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/otherworld/nimbo/internal/brand"
)

func withAccent(t *testing.T, hex string) {
	t.Helper()
	old := brand.Current.AccentHex
	brand.Current.AccentHex = hex
	t.Cleanup(func() { brand.Current.AccentHex = old })
}

// One loop is trayLoopFrames frames, and the next loop repeats it exactly.
func TestTraySpinLoops(t *testing.T) {
	withAccent(t, "#22D3C4")
	if trayLoopFrames != 12 {
		t.Fatalf("trayLoopFrames = %d", trayLoopFrames)
	}
	for _, size := range []int{16, 32} {
		for n := 0; n < trayLoopFrames; n++ {
			a := trayImage("sync", n, false, size)
			b := trayImage("sync", n+trayLoopFrames, false, size)
			if !bytes.Equal(a.Pix, b.Pix) {
				t.Fatalf("size %d: frame %d differs from frame %d", size, n, n+trayLoopFrames)
			}
			if n > 0 && bytes.Equal(a.Pix, trayImage("sync", n-1, false, size).Pix) {
				t.Fatalf("size %d: frames %d and %d are identical", size, n-1, n)
			}
		}
	}
	if !bytes.Equal(trayIcon("sync", 5, true), trayIcon("sync", 5+trayLoopFrames, true)) {
		t.Fatal("cached icon differs between loops")
	}
}

// The stock sync state turns the glyph and carries no corner spinner; the
// spinner's teal is gone and the corner is the tile's own.
func TestTraySpinStockHasNoCornerBadge(t *testing.T) {
	withAccent(t, "#22D3C4")
	sync := trayImage("sync", 0, false, 32)
	idle := trayImage("idle", 0, false, 32)
	// The bottom-right corner area (where the badge sits) outside the glyph
	// radius: compare a patch around (0.84, 0.84).
	for y := 24; y < 29; y++ {
		for x := 24; x < 29; x++ {
			if sync.NRGBAAt(x, y) != idle.NRGBAAt(x, y) {
				t.Fatalf("pixel %d,%d differs from idle: %v vs %v", x, y, sync.NRGBAAt(x, y), idle.NRGBAAt(x, y))
			}
		}
	}
	// Frame 0 is the idle mark, bar anti-aliasing noise from the glyph sampling.
	diff := 0
	for i := range sync.Pix {
		d := int(sync.Pix[i]) - int(idle.Pix[i])
		if d < -40 || d > 40 {
			diff++
		}
	}
	if diff > 40 {
		t.Fatalf("sync frame 0 strays from the static mark in %d samples", diff)
	}
}

// The notification dot overlays the turning mark.
func TestTraySpinKeepsNotificationDot(t *testing.T) {
	withAccent(t, "#22D3C4")
	c := trayImage("sync", 3, true, 32).NRGBAAt(25, 7)
	if c.R < 0xc0 || c.G > 0x60 {
		t.Fatalf("notification dot missing: %v", c)
	}
}

// Partners keep the nimbus with the corner spinner.
func TestTraySyncPartnerKeepsSpinner(t *testing.T) {
	withAccent(t, "#CC2233")
	img := trayImage("sync", 0, false, 32)
	var teal int
	for y := 20; y < 32; y++ {
		for x := 20; x < 32; x++ {
			if c := img.NRGBAAt(x, y); c.G > c.R+60 && c.B > c.R+40 && c.A > 0 && color.NRGBAModel.Convert(c).(color.NRGBA).G > 100 {
				teal++
			}
		}
	}
	if teal == 0 {
		t.Fatal("partner sync lost its corner spinner")
	}
	// And it is not frame-cached to the stock rotation: it advances per frame.
	if bytes.Equal(trayImage("sync", 0, false, 32).Pix, trayImage("sync", 1, false, 32).Pix) {
		t.Fatal("partner spinner does not turn")
	}
	// Frames past the stock loop are still distinct for partners (360/12 = 30 frames).
	if bytes.Equal(trayImage("sync", 1, false, 32).Pix, trayImage("sync", 1+trayLoopFrames, false, 32).Pix) {
		t.Fatal("partner spinner loops at the stock loop length")
	}
}

// Writes the frame strips and GIFs to the (gitignored) preview folder.
func TestRenderTraySpinPreview(t *testing.T) {
	dir := os.Getenv("NIMBO_TRAY_PREVIEW")
	if dir == "" {
		t.Skip("set NIMBO_TRAY_PREVIEW=<dir> to render")
	}
	withAccent(t, "#22D3C4")
	_ = os.MkdirAll(dir, 0o755)
	const scale = 4
	bgs := []color.NRGBA{{0xf3, 0xf3, 0xf3, 0xff}, {0x20, 0x20, 0x20, 0xff}}
	cell := map[int]int{16: 16 * scale, 32: 32 * scale}
	// Strip: for each bg and size, frames in a row, drawn 4x nearest.
	totalH := 0
	for _, s := range []int{16, 32} {
		totalH += cell[s] * len(bgs)
	}
	W := trayLoopFrames * 32 * scale
	strip := newRGBA(W, totalH)
	y0 := 0
	for _, s := range []int{16, 32} {
		for _, bg := range bgs {
			for n := 0; n < trayLoopFrames; n++ {
				im := trayImage("sync", n, false, s)
				fill(strip, n*cell[s], y0, cell[s], cell[s], bg)
				for y := 0; y < s*scale; y++ {
					for x := 0; x < s*scale; x++ {
						c := im.NRGBAAt(x/scale, y/scale)
						blend(strip, n*cell[s]+x, y0+y, c, bg)
					}
				}
			}
			y0 += cell[s]
		}
	}
	savePNG(t, filepath.Join(dir, "sync-strip.png"), strip)
	for _, s := range []int{32, 64} {
		g := &gif.GIF{}
		for n := 0; n < trayLoopFrames; n++ {
			im := trayImage("sync", n, false, s)
			bg := newRGBA(s, s)
			fill(bg, 0, 0, s, s, bgs[0])
			draw.Draw(bg, bg.Bounds(), im, im.Bounds().Min, draw.Over)
			p := quantize(bg)
			g.Image = append(g.Image, p)
			g.Delay = append(g.Delay, 12)
		}
		var buf bytes.Buffer
		if err := gif.EncodeAll(&buf, g); err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(filepath.Join(dir, "sync-"+strconv.Itoa(s)+".gif"), buf.Bytes(), 0o644)
	}
}

func newRGBA(w, h int) *image.RGBA { return image.NewRGBA(image.Rect(0, 0, w, h)) }

func fill(dst *image.RGBA, x, y, w, h int, c color.NRGBA) {
	draw.Draw(dst, image.Rect(x, y, x+w, y+h), image.NewUniform(c), image.Point{}, draw.Src)
}

func blend(dst *image.RGBA, x, y int, c color.NRGBA, bg color.NRGBA) {
	a := uint32(c.A)
	m := func(s, d uint8) uint8 { return uint8((uint32(s)*a + uint32(d)*(255-a) + 127) / 255) }
	dst.SetRGBA(x, y, color.RGBA{m(c.R, bg.R), m(c.G, bg.G), m(c.B, bg.B), 255})
}

func savePNG(t *testing.T, path string, im image.Image) {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, im); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func quantize(src *image.RGBA) *image.Paletted {
	p := image.NewPaletted(src.Bounds(), palette.Plan9)
	draw.FloydSteinberg.Draw(p, src.Bounds(), src, image.Point{})
	return p
}
