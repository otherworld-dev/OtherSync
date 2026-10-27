package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"runtime"
	"strings"
	"sync"

	"github.com/otherworld/nimbo/internal/brand"
	"github.com/otherworld/nimbo/internal/brandmark"
)

// Tray badge colours.
var (
	traySpin  = color.RGBA{0x0D, 0x94, 0x88, 0xff} // spinner: the brand's deep teal (legible on white)
	trayRed   = color.RGBA{0xd6, 0x39, 0x39, 0xff}
	trayAmber = color.RGBA{0xE8, 0x8A, 0x00, 0xff} // "needs attention", the flyout's amber bar
	trayGray  = color.RGBA{0x88, 0x90, 0x96, 0xff}
	trayWhite = color.RGBA{0xff, 0xff, 0xff, 0xff}
)

// traySizes are the small-icon sizes Windows asks for at 100, 125, 150, 200,
// 250 and 300% scaling. The tray .ico carries one frame drawn at each, so the
// shell never shrinks a 32 px image into the 16 px slot.
var traySizes = []int{16, 20, 24, 32, 40, 48}

// trayIcon renders the system-tray icon for a sync state ("idle", "sync",
// "paused", "error", "warning"): the brand tile (the app icon's own frame for
// that size, greyed while paused) plus a small state badge bottom-right —
// spinner while syncing (frame rotates it), pause bars, a red ! (error) or an
// amber ! (items that need the user, Deck #984). badge adds a red dot
// top-right (pending notifications).
//
// On Windows it is a multi-size .ico (Wails picks the frame for the tray's
// size); elsewhere a 32 px PNG.
func trayIcon(state string, frame int, badge bool) []byte {
	if state == "sync" && stockTray() {
		return cachedSpinIcon(frame, badge)
	}
	return renderTrayIcon(state, frame, badge)
}

// renderTrayIcon encodes trayImage at every size the platform wants.
func renderTrayIcon(state string, frame int, badge bool) []byte {
	if runtime.GOOS != "windows" {
		var buf bytes.Buffer
		_ = png.Encode(&buf, trayImage(state, frame, badge, 32))
		return buf.Bytes()
	}
	frames := make([][]byte, len(traySizes))
	for i, s := range traySizes {
		var buf bytes.Buffer
		_ = png.Encode(&buf, trayImage(state, frame, badge, s))
		frames[i] = buf.Bytes()
	}
	return encodeICO(traySizes, frames)
}

// trayImage draws the tray icon at size x size pixels.
func trayImage(state string, frame int, badge bool, size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	spin := state == "sync" && stockTray()
	if spin {
		// The stock mark's glyph turns in place of a corner spinner.
		img = brandmark.DrawWith(size, brandmark.Options{
			GlyphAngle:   trayGlyphAngle(frame),
			GlyphSamples: 4,
		})
	} else if tile := brandFrame(size); tile != nil {
		b := tile.Bounds()
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				c := color.NRGBAModel.Convert(tile.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
				if state == "paused" {
					// Greyed out: the mark's own luminance, so it still reads.
					l := uint8(math.Round(0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)))
					c.R, c.G, c.B = l, l, l
				}
				img.SetNRGBA(x, y, c)
			}
		}
	}

	f := float64(size)
	ring := f * 0.0375 // white halo around the badges (1.2 px at 32)
	// State badge bottom-right.
	bx, by, br := f*0.74, f*0.74, f*0.235
	// Below 24 px a stroke is about one pixel wide, so centre the badge on a
	// pixel and draw the bars on whole pixels: crisp instead of a grey smear.
	small := size < 24
	if small {
		bx, by = math.Floor(bx)+0.5, math.Floor(by)+0.5
	}
	bar := func(x0, y0, x1, y1 float64, col color.RGBA) {
		if small {
			x0, y0, x1, y1 = math.Round(x0), math.Round(y0), math.Round(x1), math.Round(y1)
			if x1-x0 < 1 { // never thinner than one pixel
				x0 = math.Floor((x0 + x1) / 2)
				x1 = x0 + 1
			}
		}
		fillRect(img, x0, y0, x1, y1, col)
	}
	switch state {
	case "sync":
		if spin {
			break
		}
		fillDisc(img, bx, by, br+ring, trayWhite)
		// Rotating open arc with a leading arrowhead.
		rr := br * 0.62
		start := float64((frame*12)%360) * math.Pi / 180
		const sweep = 250 * math.Pi / 180
		w := math.Max(br*0.32, 1.1)
		paint(img, traySpin, func(x, y float64) float64 {
			return sdArcSeg(x-bx, y-by, rr, start, sweep) - w/2
		})
		end := start + sweep
		fillDisc(img, bx+rr*math.Cos(end), by+rr*math.Sin(end), math.Max(br*0.30, 1.1), traySpin)
	case "paused":
		fillDisc(img, bx, by, br+ring, trayWhite)
		bar(bx-br*0.42, by-br*0.5, bx-br*0.10, by+br*0.5, trayGray)
		bar(bx+br*0.10, by-br*0.5, bx+br*0.42, by+br*0.5, trayGray)
	case "error", "warning":
		fill := trayRed
		if state == "warning" {
			fill = trayAmber
		}
		fillDisc(img, bx, by, br+ring, trayWhite)
		fillDisc(img, bx, by, br, fill)
		if small {
			bar(bx-0.5, by-br*0.62, bx+0.5, by+br*0.12, trayWhite)
			bar(bx-0.5, by+br*0.40, bx+0.5, by+br*0.40+1, trayWhite)
		} else {
			thick(img, bx, by-br*0.5, bx, by+br*0.12, br*0.26, trayWhite)
			fillDisc(img, bx, by+br*0.48, br*0.14, trayWhite)
		}
	}

	// Pending-notifications badge: a red dot with a white ring, top-right.
	if badge {
		nx, ny, nr := f*0.78, f*0.22, f*0.17
		fillDisc(img, nx, ny, nr+f*0.044, trayWhite)
		fillDisc(img, nx, ny, nr, trayRed)
	}
	return img
}

// paint composites col over img wherever the signed distance sd (pixels,
// negative inside) says, anti-aliased over one pixel.
func paint(img *image.NRGBA, col color.RGBA, sd func(x, y float64) float64) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			a := 0.5 - sd(float64(x)+0.5, float64(y)+0.5)
			if a <= 0 {
				continue
			}
			if a > 1 {
				a = 1
			}
			over(img, x, y, col, a)
		}
	}
}

// over blends an opaque colour at coverage a onto one straight-alpha pixel.
func over(img *image.NRGBA, x, y int, col color.RGBA, a float64) {
	d := img.NRGBAAt(x, y)
	da := float64(d.A) / 255
	oa := a + da*(1-a)
	if oa == 0 {
		return
	}
	mix := func(s, t uint8) uint8 {
		return uint8(math.Round((float64(s)*a + float64(t)*da*(1-a)) / oa))
	}
	img.SetNRGBA(x, y, color.NRGBA{mix(col.R, d.R), mix(col.G, d.G), mix(col.B, d.B), uint8(math.Round(oa * 255))})
}

func fillDisc(img *image.NRGBA, cx, cy, r float64, col color.RGBA) {
	paint(img, col, func(x, y float64) float64 { return math.Hypot(x-cx, y-cy) - r })
}

func fillRect(img *image.NRGBA, x0, y0, x1, y1 float64, col color.RGBA) {
	paint(img, col, func(x, y float64) float64 {
		return math.Max(math.Max(x0-x, x-x1), math.Max(y0-y, y-y1))
	})
}

func thick(img *image.NRGBA, x0, y0, x1, y1, w float64, col color.RGBA) {
	paint(img, col, func(x, y float64) float64 { return distSeg(x, y, x0, y0, x1, y1) - w/2 })
}

// sdArcSeg is the distance from (x, y), relative to the arc's centre, to an
// arc of radius r running clockwise (on screen) from angle start for sweep
// radians.
func sdArcSeg(x, y, r, start, sweep float64) float64 {
	a := math.Mod(math.Atan2(y, x)-start, 2*math.Pi)
	if a < 0 {
		a += 2 * math.Pi
	}
	if a <= sweep {
		return math.Abs(math.Hypot(x, y) - r)
	}
	e0 := math.Hypot(x-r*math.Cos(start), y-r*math.Sin(start))
	e1 := math.Hypot(x-r*math.Cos(start+sweep), y-r*math.Sin(start+sweep))
	return math.Min(e0, e1)
}

func distSeg(px, py, x0, y0, x1, y1 float64) float64 {
	dx, dy := x1-x0, y1-y0
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return math.Hypot(px-x0, py-y0)
	}
	t := math.Max(0, math.Min(1, ((px-x0)*dx+(py-y0)*dy)/l2))
	return math.Hypot(px-(x0+t*dx), py-(y0+t*dy))
}

// trayLoopFrames is how many frames one loop of the stock sync animation has.
// The glyph (two arcs and two arrowheads) repeats every half turn, so a loop
// is 180 degrees: 15 degrees a frame, a half turn every 1.44 s at the 120 ms
// tick.
const trayLoopFrames = 12

// trayGlyphAngle is the glyph's clockwise turn, in degrees, at a frame.
func trayGlyphAngle(frame int) float64 {
	f := frame % trayLoopFrames
	if f < 0 {
		f += trayLoopFrames
	}
	return float64(f) * 180 / trayLoopFrames
}

// stockTray reports whether the tray carries the stock OtherSync mark, whose
// sync state turns the glyph. Partner builds keep the nimbus with a corner
// spinner.
func stockTray() bool {
	return strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(brand.Current.AccentHex), "#"), "22D3C4")
}

// spinCache holds the encoded stock sync icons, drawn on first use, so a tick
// only looks one up.
var spinCache struct {
	sync.Mutex
	icons map[[2]int][]byte // frame in the loop, notification dot (0/1)
}

func cachedSpinIcon(frame int, badge bool) []byte {
	f := frame % trayLoopFrames
	if f < 0 {
		f += trayLoopFrames
	}
	b := 0
	if badge {
		b = 1
	}
	spinCache.Lock()
	defer spinCache.Unlock()
	if ic, ok := spinCache.icons[[2]int{f, b}]; ok {
		return ic
	}
	if spinCache.icons == nil {
		spinCache.icons = map[[2]int][]byte{}
	}
	ic := renderTrayIcon("sync", f, badge)
	spinCache.icons[[2]int{f, b}] = ic
	return ic
}
