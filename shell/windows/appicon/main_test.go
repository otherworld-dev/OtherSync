package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/otherworld/nimbo/internal/brand"
	"github.com/otherworld/nimbo/internal/brandmark"
)

var legacyPalette = palette{
	tileTop:    color.RGBA{0x6F, 0x6A, 0xF2, 0xff},
	tileBot:    color.RGBA{0x4A, 0x3E, 0xD2, 0xff},
	cloudLight: color.RGBA{0xFF, 0xD5, 0x5F, 0xff},
	cloudDeep:  color.RGBA{0xEF, 0xA5, 0x3A, 0xff},
}

// The legacy Nimbo accent — in any case, with or without '#' — must reproduce
// the exact hand-tuned gold-on-indigo nimbus.
func TestLegacyPaletteIsCanonical(t *testing.T) {
	for _, a := range []string{"#5856E0", "5856e0", " #5856e0 "} {
		if got := newPalette(a); got != legacyPalette {
			t.Errorf("newPalette(%q) = %+v, want canonical legacy palette", a, got)
		}
	}
}

// The stock build draws the family mark, so brand.json's accent must be the
// one the generator treats as stock.
func TestBrandAccentIsStock(t *testing.T) {
	if normalizeHex(brand.Current.AccentHex) != normalizeHex(stockAccent) {
		t.Fatalf("brand accent %s is not stockAccent %s: the stock icon would be a derived nimbus",
			brand.Current.AccentHex, stockAccent)
	}
}

func TestStockAccentSelectsTheMark(t *testing.T) {
	defer setAccent(stockAccent)
	for _, a := range []string{"#22D3C4", "22d3c4", " #22d3c4 "} {
		if setAccent(a); !stockMark {
			t.Errorf("setAccent(%q) did not select the family mark", a)
		}
	}
	for _, a := range []string{"#5856E0", "#CC2233", "garbage"} {
		if setAccent(a); stockMark {
			t.Errorf("setAccent(%q) selected the family mark; partners get the nimbus", a)
		}
	}
}

// The master render carries the sheet's colours where they belong.
func TestMarkColours(t *testing.T) {
	img := brandmark.Draw(512)
	m := float64(brandmark.Margin(512))
	k := (512 - 2*m) / 512
	at := func(u, v float64) color.NRGBA { return img.NRGBAAt(int(m+u*k), int(m+v*k)) }
	want := func(name string, got color.NRGBA, c color.RGBA) {
		t.Helper()
		if got.R != c.R || got.G != c.G || got.B != c.B || got.A != 0xff {
			t.Errorf("%s = %v, want %v", name, got, c)
		}
	}
	want("centre dot", at(256, 256), brandmark.Glow)
	want("ring", at(256+178, 256), brandmark.Teal)
	want("top arc", at(256, 256-92), brandmark.Paper)
	if c := img.NRGBAAt(0, 0); c.A != 0 {
		t.Errorf("corner should be transparent, got %v", c)
	}
}

// Partners keep exactly today's derived nimbus: these hashes were taken from
// the generator before the family mark existed.
func TestPartnerNimbusUnchanged(t *testing.T) {
	defer setAccent(stockAccent)
	for _, c := range []struct {
		accent string
		size   int
		sha    string
	}{
		{"#CC2233", 16, "36e00d077a207614122871ea796640f3e74406bc1e10e12ac117b6882c6b63a1"},
		{"#CC2233", 32, "0d5e5e7d357303c246ed9a2b84daa9b999379515c5876d5bb6e5ff5fb8466ff2"},
		{"#CC2233", 256, "e0f49f6a1018bc0e4d7c7aabf264c7d98d80100a10e6b4714d495797cf4ce90a"},
		{"#5856E0", 16, "fa96d00ab3703046381e9526ca6656fcea369e54453bfd22f68d03e5ca2d6788"},
		{"#5856E0", 32, "77cf644f7cd169c6bffdcf4ca6d5930ff221de08e21f0b2b07a4640c5487014b"},
		{"#5856E0", 256, "a941bc5b72f94437ce2d7e15ee2f4bdd52d77227003691f4ba4740c7ab327916"},
	} {
		setAccent(c.accent)
		img, ok := draw(c.size).(*image.RGBA)
		if !ok {
			t.Fatalf("%s: draw(%d) is not the nimbus", c.accent, c.size)
		}
		sum := sha256.Sum256(img.Pix)
		if got := hex.EncodeToString(sum[:]); got != c.sha {
			t.Errorf("%s at %d px changed: sha %s", c.accent, c.size, got)
		}
	}
}

// The committed icons are what the generator draws for the stock brand. This
// pins the family mark (a change to it fails here until the assets are
// regenerated) and catches a generator change that was never re-run. One step
// per channel is allowed so float differences between platforms don't fail it.
func TestCommittedIconsMatchGenerator(t *testing.T) {
	setAccent(stockAccent)
	check := func(name string, got image.Image, data []byte) {
		t.Helper()
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if img.Bounds() != got.Bounds() {
			t.Fatalf("%s: size %v, generator draws %v", name, img.Bounds(), got.Bounds())
		}
		b := img.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				w := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA)
				g := color.NRGBAModel.Convert(got.At(x, y)).(color.NRGBA)
				if d(w.R, g.R) > 1 || d(w.G, g.G) > 1 || d(w.B, g.B) > 1 || d(w.A, g.A) > 1 {
					t.Fatalf("%s differs at (%d,%d): committed %v, generator %v; re-run the generator", name, x, y, w, g)
				}
			}
		}
	}
	assets := filepath.Join("..", "..", "..", "packaging", "msix", "Assets")
	for name, size := range logoSizes() {
		data, err := os.ReadFile(filepath.Join(assets, name))
		if err != nil {
			t.Fatal(err)
		}
		check(name, draw(size), data)
	}
	ico, err := os.ReadFile(filepath.Join("..", "..", "..", "cmd", "nimbo-gui", "assets", "nimbo.ico"))
	if err != nil {
		t.Fatal(err)
	}
	n := int(binary.LittleEndian.Uint16(ico[4:6]))
	if n != len(icoSizes) {
		t.Fatalf("nimbo.ico has %d frames, the generator writes %d", n, len(icoSizes))
	}
	for i := 0; i < n; i++ {
		e := ico[6+16*i:]
		size := int(e[0])
		if size == 0 {
			size = 256
		}
		ln := binary.LittleEndian.Uint32(e[8:12])
		off := binary.LittleEndian.Uint32(e[12:16])
		check(fmt.Sprintf("nimbo.ico %d px", size), draw(size), ico[off:off+ln])
	}
}

func TestDerivedPaletteDiffersAndTracksAccent(t *testing.T) {
	p := newPalette("#CC2233") // a red accent
	if p == legacyPalette {
		t.Fatal("a non-stock accent should derive a different palette")
	}
	if h, _, _ := rgbToHSL(p.tileTop); !(h < 25 || h > 335) {
		t.Errorf("derived tile hue %.0f should track the red accent", h)
	}
}

func TestParseHex(t *testing.T) {
	if got := parseHex("#a1b2c3"); got != (color.RGBA{0xA1, 0xB2, 0xC3, 0xff}) {
		t.Errorf("parseHex(#a1b2c3) = %+v", got)
	}
	// Malformed input must not panic; it falls back to stock indigo.
	if got := parseHex("not-a-colour"); got != (color.RGBA{0x58, 0x56, 0xE0, 0xff}) {
		t.Errorf("parseHex(garbage) = %+v, want stock indigo fallback", got)
	}
}

func TestHSLRoundTrip(t *testing.T) {
	for _, c := range []color.RGBA{
		{0x58, 0x56, 0xE0, 0xff}, {0xCC, 0x22, 0x33, 0xff},
		{0x20, 0xA0, 0x60, 0xff}, {0x80, 0x80, 0x80, 0xff}, {0xFF, 0xD5, 0x5F, 0xff},
	} {
		h, s, l := rgbToHSL(c)
		got := hslToRGB(h, s, l)
		if d(got.R, c.R) > 1 || d(got.G, c.G) > 1 || d(got.B, c.B) > 1 {
			t.Errorf("HSL round-trip %+v -> %+v", c, got)
		}
	}
}

func d(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

func TestWriteAndroidNamedVariant(t *testing.T) {
	res := t.TempDir()
	writeAndroid(res, "ic_launcher_forest")

	for _, f := range []string{
		"drawable/ic_launcher_forest_background.xml",
		"drawable/ic_launcher_forest_foreground.xml",
		"drawable/ic_launcher_forest_monochrome.xml",
		"mipmap-anydpi-v26/ic_launcher_forest.xml",
	} {
		if _, err := os.Stat(filepath.Join(res, f)); err != nil {
			t.Fatalf("missing %s: %v", f, err)
		}
	}
	icon, _ := os.ReadFile(filepath.Join(res, "mipmap-anydpi-v26", "ic_launcher_forest.xml"))
	for _, ref := range []string{"@drawable/ic_launcher_forest_background", "@drawable/ic_launcher_forest_foreground", "@drawable/ic_launcher_forest_monochrome"} {
		if !strings.Contains(string(icon), ref) {
			t.Errorf("adaptive icon does not reference %s", ref)
		}
	}
}

func TestWriteAndroidDefaultLeavesTheHandWrittenIcon(t *testing.T) {
	res := t.TempDir()
	writeAndroid(res, "ic_launcher")
	if _, err := os.Stat(filepath.Join(res, "drawable", "ic_launcher_foreground.xml")); err != nil {
		t.Fatal(err)
	}
	// ic_launcher.xml carries a hand-written comment; the generator must not replace it.
	if _, err := os.Stat(filepath.Join(res, "mipmap-anydpi-v26", "ic_launcher.xml")); !os.IsNotExist(err) {
		t.Fatalf("default name wrote mipmap-anydpi-v26/ic_launcher.xml")
	}
}

// The committed Android icons are the generator's output: the stock adaptive
// layers and notification icon, and each supporter variant at its accent.
// Catches a generator change that was not re-run (or a hand edit).
func TestCommittedAndroidIconsMatchGenerator(t *testing.T) {
	committed := filepath.Join("..", "..", "..", "android", "app", "src", "main", "res")
	defer setAccent(brand.Current.AccentHex)
	cases := []struct{ accent, name string }{
		{stockAccent, "ic_launcher"},
		{"#2E7D5B", "ic_launcher_forest"},
		{"#B4441C", "ic_launcher_ember"},
		{"#3D4B5C", "ic_launcher_slate"},
	}
	for _, c := range cases {
		setAccent(c.accent)
		res := t.TempDir()
		writeAndroid(res, c.name)
		files := []string{c.name + "_background.xml", c.name + "_foreground.xml", c.name + "_monochrome.xml"}
		if c.name == "ic_launcher" {
			files = append(files, "ic_notification.xml")
		}
		for _, f := range files {
			want, err := os.ReadFile(filepath.Join(res, "drawable", f))
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(filepath.Join(committed, "drawable", f))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n")), want) {
				t.Errorf("%s differs from the generator: run go run . android (accent %s, -name %s)", f, c.accent, c.name)
			}
		}
	}
}

// The stock foreground's ring (outer edge) spans ringSafeFill of the 66dp
// safe zone, and the glow stays inside it.
func TestAndroidMarkFitsTheSafeZone(t *testing.T) {
	g := brandmark.GeometryFor(512)
	s := markScale(g)
	ring := 2 * (g.RingR + g.RingW/2) * s
	if math.Abs(ring-ringSafeFill*adaptiveSafe) > 0.01 {
		t.Errorf("ring spans %.2fdp, want %.2fdp", ring, ringSafeFill*adaptiveSafe)
	}
	if glow := 2 * (g.RingR + g.GlowW/2) * s; glow > adaptiveSafe {
		t.Errorf("glow spans %.2fdp, past the %vdp safe zone", glow, adaptiveSafe)
	}
}

// The stock Play icon is the mark's tile filling the square: opaque night in
// the corners (Play masks them), the ring where the master has it.
func TestPlayIconIsTheSquareMark(t *testing.T) {
	defer setAccent(brand.Current.AccentHex)
	setAccent(stockAccent)
	path := filepath.Join(t.TempDir(), "play.png")
	writePlayIcon(path)
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 512 {
		t.Fatalf("size %v", img.Bounds())
	}
	_, _, _, a := img.At(511, 511).RGBA()
	if a != 0xffff {
		t.Errorf("corner is not opaque (alpha %d)", a)
	}
	r, g, b, _ := img.At(256+178, 256).RGBA()
	if want := brandmark.Teal; d(uint8(r>>8), want.R) > 2 || d(uint8(g>>8), want.G) > 2 || d(uint8(b>>8), want.B) > 2 {
		t.Errorf("ring pixel %02x%02x%02x, want teal", r>>8, g>>8, b>>8)
	}
	committed, err := os.ReadFile(filepath.Join("..", "..", "..", "android", "store", "icon-512.png"))
	if err != nil {
		t.Fatal(err)
	}
	gen, _ := os.ReadFile(path)
	if !bytes.Equal(committed, gen) {
		t.Errorf("android/store/icon-512.png differs from the generator: run go run . play ../../../android/store/icon-512.png")
	}
}
