// Package brandmark draws the OtherSync family mark.
//
// The mark: a warm slate tile carrying a valley-green portal ring, with two
// chalk sync arcs turning around a green centre point.
//
// It is drawn procedurally from the Other family brand sheet so every size is
// rendered crisp rather than downscaled, and so the tray can turn the glyph.
// Two designs, as the sheet exports them: above 64 px the master
// (Icons/othersync.svg); at 64 px and below the bolder taskbar/tray variant
// (Icons/small/othersync-small.svg: thicker ring, bigger, heavier glyph).
package brandmark

import (
	"image"
	"image/color"
	"math"
)

// The sheet's colours (Other family brief, 15 Nov 2026).
var (
	tileCentre = color.RGBA{0x33, 0x37, 0x3C, 0xff} // tile gradient centre
	tileEdge   = color.RGBA{0x1E, 0x21, 0x24, 0xff} // tile gradient edge
	Ring       = color.RGBA{0x5F, 0xA0, 0x5B, 0xff} // portal ring: valley green
	Chalk      = color.RGBA{0xF4, 0xF0, 0xE8, 0xff} // glyph
	Dot        = Ring                               // sync point
)

// TileCentre and TileEdge are the tile's radial gradient stops.
func TileCentre() color.RGBA { return tileCentre }
func TileEdge() color.RGBA   { return tileEdge }

// markSpec is the mark's geometry in the 512-unit design space.
type markSpec struct {
	ringR, ringW float64 // portal ring radius and stroke
	glowW, glowA float64 // soft glow ring stroke and opacity (0 = none)
	arcR, arcW   float64 // sync arcs: radius and stroke (round caps)
	gapDeg       float64 // half the angular gap between the arcs, at each side
	arrow        float64 // arrowhead scale relative to the master polygon
	arrowPad     float64 // half the arrowheads' round-joined outline stroke
	dotR         float64 // centre point radius (0 = none)
	cornerR      float64 // tile corner radius
}

// glyphSpec scales the sheet's glyph (drawn at 1x around the centre, then
// scaled by the svg's group transform) with the given stroke widths.
func glyphSpec(ringR, ringW, scale, arcW, arrowStroke float64) markSpec {
	return markSpec{
		ringR: ringR, ringW: ringW,
		arcR: 92 * scale, arcW: arcW * scale, gapDeg: 20,
		arrow: scale, arrowPad: arrowStroke / 2 * scale,
		dotR: 15 * scale, cornerR: 112,
	}
}

// masterSpec is othersync.svg exactly: ring r212 w24, glyph scaled 1.45.
var masterSpec = glyphSpec(212, 24, 1.45, 26, 6)

// smallSpec is othersync-small.svg exactly: ring r224 w48, glyph scaled 1.66
// with strokes 1.25x heavier.
var smallSpec = glyphSpec(224, 48, 1.66, 32.5, 7.5)

// SmallMax is the largest size drawn with the small variant (the sheet's
// export uses it at 64 px and below, so 200% taskbar scaling gets it too).
const SmallMax = 64

// specFor returns the geometry used at a pixel size.
func specFor(size int) markSpec {
	if size <= SmallMax {
		return smallSpec
	}
	return masterSpec
}

// Margin is the transparent border around the tile. The sheet's exports run
// the rounded tile to the edge of the canvas, so there is none.
func Margin(size int) int { return 0 }

// The master's right-hand arrowhead (the left one is its 180° rotation), and
// the end of the top arc it sits on.
var (
	masterArrow  = [3][2]float64{{351.4, 248.9}, {363.5, 206.2}, {314.7, 224.0}}
	masterArcEnd = [2]float64{342.5, 224.5}
)

// arrowLocal is the master arrowhead in a frame at the arc end: u along the
// outward radius, v along the direction of travel (clockwise on screen).
var arrowLocal = func() (l [3][2]float64) {
	ex, ey := masterArcEnd[0]-256, masterArcEnd[1]-256
	n := math.Hypot(ex, ey)
	ux, uy := ex/n, ey/n
	vx, vy := -uy, ux
	for i, p := range masterArrow {
		dx, dy := p[0]-masterArcEnd[0], p[1]-masterArcEnd[1]
		l[i] = [2]float64{dx*ux + dy*uy, dx*vx + dy*vy}
	}
	return l
}()

// Options tunes Draw. The zero value is the static mark.
type Options struct {
	// GlyphAngle turns the sync glyph (both arcs and both arrowheads, not the
	// tile, ring or centre point) clockwise about the tile centre, in degrees.
	GlyphAngle float64
	// GlyphSamples is the glyph's anti-aliasing grid (n x n samples per pixel).
	// 0 or 1 uses the analytic one-pixel ramp.
	GlyphSamples int
	// Square fills the whole canvas with the tile: no margin and square
	// corners, for surfaces that apply their own mask (the Google Play icon).
	Square bool
}

// Geometry is the mark's shape at one pixel size, in the 512-unit design
// space centred on (256,256), for drawing it as vectors (the Android icons).
type Geometry struct {
	RingR, RingW float64 // portal ring radius and stroke
	GlowW, GlowA float64 // soft glow ring stroke and opacity (0 = none; the sheet has none)
	ArcR, ArcW   float64 // sync arcs: radius and stroke (round caps)
	// The top arc runs clockwise from 180+GapDeg to 360-GapDeg degrees and the
	// bottom one from GapDeg to 180-GapDeg (0 = +x, y down).
	GapDeg float64
	// Arrows are the two arrowhead triangles; each is also stroked ArrowPad*2
	// wide with round joins, as the master strokes its polygons.
	Arrows   [2][3][2]float64
	ArrowPad float64
	DotR     float64 // centre point radius (0 = none)
}

// GeometryFor returns the geometry Draw uses at a pixel size (above SmallMax
// the master, othersync.svg exactly; otherwise othersync-small.svg).
func GeometryFor(size int) Geometry {
	sp := specFor(size)
	return Geometry{
		RingR: sp.ringR, RingW: sp.ringW, GlowW: sp.glowW, GlowA: sp.glowA,
		ArcR: sp.arcR, ArcW: sp.arcW, GapDeg: sp.gapDeg,
		Arrows: arrowheads(sp), ArrowPad: sp.arrowPad, DotR: sp.dotR,
	}
}

// arrowheads places the master arrowhead at the end of each arc, scaled.
func arrowheads(sp markSpec) (arrows [2][3][2]float64) {
	phi := -sp.gapDeg * math.Pi / 180 // end of the top arc (right-hand side)
	ux, uy := math.Cos(phi), math.Sin(phi)
	vx, vy := -uy, ux
	ex, ey := 256+sp.arcR*ux, 256+sp.arcR*uy
	for i, l := range arrowLocal {
		x := ex + sp.arrow*(l[0]*ux+l[1]*vx)
		y := ey + sp.arrow*(l[0]*uy+l[1]*vy)
		arrows[0][i] = [2]float64{x, y}
		arrows[1][i] = [2]float64{512 - x, 512 - y}
	}
	return arrows
}

// Draw renders the family mark at size x size pixels.
func Draw(size int) *image.NRGBA { return DrawWith(size, Options{}) }

// DrawWith renders the family mark at size x size pixels with options.
func DrawWith(size int, o Options) *image.NRGBA {
	sp := specFor(size)
	m := float64(Margin(size))
	if o.Square {
		m, sp.cornerR = 0, 0
	}
	f := float64(size)
	tile := f - 2*m
	k := tile / 512 // pixels per design unit
	// The two arrowheads, in design units.
	arrows := arrowheads(sp)
	// The sheet strokes its arrowheads with round joins.
	arrowPad := sp.arrowPad

	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			cx, cy := float64(x)+0.5, float64(y)+0.5
			// Tile coverage, measured in pixels.
			a := cover(sdRoundRect(cx, cy, m, m, f-m, f-m, sp.cornerR*k))
			if a == 0 {
				continue
			}
			// Back to design units for everything inside the tile.
			du, dv := (cx-m)/k, (cy-m)/k
			// Radial tile gradient: cx 50% cy 42% r 65% of the tile.
			t := math.Min(1, math.Hypot(du-256, dv-0.42*512)/(0.65*512))
			r, g, b := rgbF(lerpColor(tileCentre, tileEdge, t))
			paint := func(col color.RGBA, alpha float64) {
				if alpha <= 0 {
					return
				}
				cr, cg, cb := rgbF(col)
				r += (cr - r) * alpha
				g += (cg - g) * alpha
				b += (cb - b) * alpha
			}
			d := math.Hypot(du-256, dv-256)
			if sp.glowA > 0 {
				paint(Ring, sp.glowA*cover((math.Abs(d-sp.ringR)-sp.glowW/2)*k))
			}
			paint(Ring, cover((math.Abs(d-sp.ringR)-sp.ringW/2)*k))
			glyph := func(u, v float64) float64 {
				if o.GlyphAngle != 0 {
					// Sample the unrotated glyph at the point turned back.
					u, v = rot(u, v, -o.GlyphAngle)
				}
				arcs := math.Min(
					sdArc(u, v, sp.arcR, 180+sp.gapDeg, 360-sp.gapDeg),
					sdArc(u, v, sp.arcR, sp.gapDeg, 180-sp.gapDeg),
				) - sp.arcW/2
				heads := math.Min(sdTriangle(u, v, arrows[0]), sdTriangle(u, v, arrows[1])) - arrowPad
				return math.Min(arcs, heads)
			}
			if n := o.GlyphSamples; n > 1 {
				var sum float64
				for j := 0; j < n; j++ {
					for i := 0; i < n; i++ {
						sx := cx + (float64(i)+0.5)/float64(n) - 0.5
						sy := cy + (float64(j)+0.5)/float64(n) - 0.5
						// Each sub-sample is a point: inside or out.
						if glyph((sx-m)/k, (sy-m)/k) <= 0 {
							sum++
						}
					}
				}
				paint(Chalk, sum/float64(n*n))
			} else {
				paint(Chalk, cover(glyph(du, dv)*k))
			}
			if sp.dotR > 0 {
				paint(Dot, cover((d-sp.dotR)*k))
			}
			img.SetNRGBA(x, y, color.NRGBA{toByte(r), toByte(g), toByte(b), toByte(a)})
		}
	}
	return img
}

// cover turns a signed distance in pixels (negative inside) into coverage,
// a one-pixel-wide linear ramp centred on the edge.
func cover(dpx float64) float64 { return clamp01(0.5 - dpx) }

func rgbF(c color.RGBA) (float64, float64, float64) {
	return float64(c.R) / 255, float64(c.G) / 255, float64(c.B) / 255
}

func toByte(v float64) uint8 { return uint8(math.Round(clamp01(v) * 255)) }

// sdRoundRect is the signed distance to a rounded rectangle.
func sdRoundRect(px, py, x0, y0, x1, y1, r float64) float64 {
	hx, hy := (x1-x0)/2, (y1-y0)/2
	qx := math.Abs(px-(x0+hx)) - (hx - r)
	qy := math.Abs(py-(y0+hy)) - (hy - r)
	out := math.Hypot(math.Max(qx, 0), math.Max(qy, 0))
	return out + math.Min(math.Max(qx, qy), 0) - r
}

// sdArc is the unsigned distance to a circular arc around (256,256) running
// clockwise on screen from angle a0 to a1 (degrees, 0 = +x, y down).
func sdArc(px, py, r, a0, a1 float64) float64 {
	dx, dy := px-256, py-256
	ang := math.Atan2(dy, dx) * 180 / math.Pi
	for ang < a0 {
		ang += 360
	}
	if ang <= a1 {
		return math.Abs(math.Hypot(dx, dy) - r)
	}
	end := func(a float64) float64 {
		t := a * math.Pi / 180
		return math.Hypot(px-(256+r*math.Cos(t)), py-(256+r*math.Sin(t)))
	}
	return math.Min(end(a0), end(a1))
}

// sdTriangle is the signed distance to a triangle (negative inside).
func sdTriangle(px, py float64, v [3][2]float64) float64 {
	d := math.Inf(1)
	inside := true
	var sign float64
	for i := 0; i < 3; i++ {
		a, b := v[i], v[(i+1)%3]
		ex, ey := b[0]-a[0], b[1]-a[1]
		wx, wy := px-a[0], py-a[1]
		t := clamp01((wx*ex + wy*ey) / (ex*ex + ey*ey))
		d = math.Min(d, math.Hypot(wx-ex*t, wy-ey*t))
		c := ex*wy - ey*wx
		if i == 0 {
			sign = c
		} else if (c > 0) != (sign > 0) {
			inside = false
		}
	}
	if inside {
		return -d
	}
	return d
}

// rot turns (u, v) by deg degrees clockwise on screen about the design centre.
func rot(u, v, deg float64) (float64, float64) {
	s, c := math.Sincos(deg * math.Pi / 180)
	du, dv := u-256, v-256
	return 256 + du*c - dv*s, 256 + du*s + dv*c
}

func clamp01(f float64) float64 { return math.Max(0, math.Min(1, f)) }

// lerpColor linearly interpolates between two colours.
func lerpColor(a, b color.RGBA, t float64) color.RGBA {
	l := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return color.RGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), 0xff}
}
