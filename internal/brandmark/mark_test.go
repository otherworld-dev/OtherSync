package brandmark

import "testing"

// The sheet exports the bolder taskbar/tray variant at 64 px and below and the
// master above; both carry the sheet's ring, glyph and dot colours.
func TestSmallVariantAtAndBelow64(t *testing.T) {
	for _, s := range []int{16, 24, 32, 48, 64} {
		if g := GeometryFor(s); g.RingR != 224 || g.RingW != 48 {
			t.Errorf("%d px: ring r%v w%v, want the small variant (r224 w48)", s, g.RingR, g.RingW)
		}
	}
	for _, s := range []int{65, 96, 256, 512} {
		if g := GeometryFor(s); g.RingR != 212 || g.RingW != 24 || g.GlowA != 0 {
			t.Errorf("%d px: ring r%v w%v glow %v, want the master (r212 w24, no glow)", s, g.RingR, g.RingW, g.GlowA)
		}
	}
}

func TestMasterColours(t *testing.T) {
	img := Draw(512)
	want := func(name string, x, y int, r, g, b uint8) {
		t.Helper()
		c := img.NRGBAAt(x, y)
		if c.R != r || c.G != g || c.B != b || c.A != 0xff {
			t.Errorf("%s at %d,%d = %v, want #%02X%02X%02X", name, x, y, c, r, g, b)
		}
	}
	want("ring", 256+212, 256, 0x5F, 0xA0, 0x5B)
	want("centre dot", 256, 256, 0x5F, 0xA0, 0x5B)
	want("top arc", 256, 256-133, 0xF4, 0xF0, 0xE8)
	if c := img.NRGBAAt(0, 0); c.A != 0 {
		t.Errorf("tile corner should be transparent, got %v", c)
	}
}
