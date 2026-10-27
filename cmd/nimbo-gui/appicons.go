package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nfnt/resize"

	"github.com/otherworld/nimbo/internal/config"
)

// Per-app icons for app windows and Start-menu shortcuts. Nextcloud's theming
// app (bundled + enabled by default) composes a 512×512 PNG per app at
// /index.php/apps/theming/icon/<id> — ideal raster source, since the raw app
// icons are SVGs Go can't rasterize without heavy deps. The PNG is downscaled
// to the standard icon sizes and cached in appIconsDir(); any failure (theming
// disabled, no Imagick) falls back to writing the embedded brand icon so the
// path always exists and everything stays functional.

var icoSizes = []int{16, 24, 32, 48, 64, 256}

// appIconsRev versions the generated icon style. Bumping it wipes the cache on
// next use and regenerates (background) every icon a shortcut references, so
// existing Start-menu pins pick up the new look at their unchanged paths.
// rev 2: Nimbo badge composited into the corner (PWA-style "via Nimbo" mark).
// rev 3: small frames encoded as classic DIBs — the shell's icon extractor
//
//	(taskbar pins, .lnk icons) renders PNG-compressed frames below 256px
//	as a blank page, even though LoadImageW copes.
//
// rev 4: the badge and the fallback are the OtherSync mark, taken from the
//
//	brand .ico frame drawn nearest each size (Deck #996).
const appIconsRev = "4"

// appIconsDir is the live icon folder: beside the Start-menu shortcuts, whose
// path names no package identity, so a change of signing identity (or a move
// to the Store build) can't strand the icon paths already written into
// shortcuts and taskbar pins. See appshortcuts.go for the full story.
func appIconsDir() string {
	cfg := ""
	if d, err := config.Resolve(); err == nil {
		cfg = d.Config
	}
	return appIconsDirFor(shortcutsDir(), cfg)
}

// legacyAppIconsDir is where icons lived before that move — under the config
// dir, which inside the package is redirected into the package data root.
// Still read once, to carry existing icons over (migrateLegacyAppIcons).
func legacyAppIconsDir() string {
	d, err := config.Resolve()
	if err != nil {
		return ""
	}
	return filepath.Join(d.Config, "appicons")
}

// appIconPath is where an app's cached .ico lives. The rev is part of the
// FILENAME: a style bump then regenerates at a fresh path with no directory
// wipe (a wipe can silently fail while Explorer holds an icon for a pinned
// shortcut — observed in the field), and Windows' icon cache, which happily
// serves stale content for an unchanged path, is defeated outright.
func (a *App) appIconPath(id string) string {
	d := appIconsDir()
	if d == "" {
		return ""
	}
	return filepath.Join(d, sanitizeFileName(id)+"."+appIconsRev+".ico")
}

// brandFrames holds the embedded brand .ico's frames, decoded once and keyed
// by pixel size (our generator emits PNG-payload ICOs, so frames are plain
// PNG). The generator draws each size natively — the small ones with a
// heavier, simplified mark — so a frame of the right size beats resampling
// the 256 one.
var brandFramesOnce sync.Once
var brandFrames map[int]image.Image

func loadBrandFrames() map[int]image.Image {
	brandFramesOnce.Do(func() { brandFrames = decodeICOFrames(navIconICO) })
	return brandFrames
}

// decodeICOFrames decodes every PNG frame of a PNG-payload .ico by size.
// Anything malformed is skipped.
func decodeICOFrames(b []byte) map[int]image.Image {
	out := map[int]image.Image{}
	if len(b) < 6 {
		return out
	}
	n := int(binary.LittleEndian.Uint16(b[4:6]))
	for i := 0; i < n; i++ {
		e := 6 + 16*i
		if e+16 > len(b) {
			break
		}
		w := int(b[e])
		if w == 0 {
			w = 256
		}
		ln := int(binary.LittleEndian.Uint32(b[e+8 : e+12]))
		off := int(binary.LittleEndian.Uint32(b[e+12 : e+16]))
		if off <= 0 || ln <= 0 || off+ln > len(b) {
			continue
		}
		if img, err := png.Decode(bytes.NewReader(b[off : off+ln])); err == nil {
			out[w] = img
		}
	}
	return out
}

// brandFrame returns the brand tile at size px: the frame drawn at that size
// when the .ico has one, otherwise the smallest larger frame resampled down
// (or the largest frame, if none is larger). Nil when the .ico has no frames.
func brandFrame(size int) image.Image {
	frames := loadBrandFrames()
	if img, ok := frames[size]; ok {
		return img
	}
	src := 0
	for s := range frames {
		switch {
		case src == 0,
			s > size && (src < size || s < src),
			src < size && s > src:
			src = s
		}
	}
	if src == 0 {
		return nil
	}
	return resize.Resize(uint(size), uint(size), frames[src], resize.Lanczos3)
}

// brandBadge returns the brand tile at its largest size.
func brandBadge() image.Image { return brandFrame(256) }

// badgeAppIcon composites the brand tile into the bottom-right corner (~38%,
// like a browser's PWA badge) so windows, Start entries and taskbar pins all
// read as "this app, opened via OtherSync". The tile is the brand .ico frame
// nearest the badge size, so call it per icon size, not once on the source.
func badgeAppIcon(src image.Image) image.Image {
	b := src.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(out, out.Bounds(), src, b.Min, draw.Src)
	bw := b.Dx() * 38 / 100
	if bw < 8 {
		return src // too small to badge legibly
	}
	scaled := brandFrame(bw)
	if scaled == nil {
		return src
	}
	m := b.Dx() / 64 // whisker of margin
	pos := image.Rect(b.Dx()-bw-m, b.Dy()-bw-m, b.Dx()-m, b.Dy()-m)
	draw.Draw(out, pos, scaled, image.Point{}, draw.Over)
	return out
}

// migrateAppIcons runs once per rev bump: best-effort removal of previous-rev
// files (current-rev names embed the rev, so a locked leftover costs disk, not
// correctness) and background regeneration of every icon a recorded Start-menu
// shortcut references.
func (a *App) migrateAppIcons(dir string) {
	revFile := filepath.Join(dir, ".rev")
	if b, err := os.ReadFile(revFile); err == nil && strings.TrimSpace(string(b)) == appIconsRev {
		return
	}
	if err := ensureAppIconsDir(dir); err != nil {
		return
	}
	cur := "." + appIconsRev + ".ico"
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			n := e.Name()
			if strings.HasSuffix(n, ".ico") && !strings.HasSuffix(n, cur) {
				_ = os.Remove(filepath.Join(dir, n)) // stale rev — best-effort tidy
			}
		}
	}
	_ = os.WriteFile(revFile, []byte(appIconsRev), 0o600)
	if d, err := config.Resolve(); err == nil {
		if s, err := d.LoadSettings(); err == nil {
			for id := range s.AppShortcuts {
				go a.ensureAppIcon(id, false)
			}
		}
	}
}

// migrateOnce gates the icon-cache rev migration to once per process (rev only
// changes across releases; concurrent first calls must not race the wipe).
var migrateOnce sync.Once

// ensureAppIcon makes sure the app's .ico exists, fetching + converting the
// server's theming PNG. wait=false runs best-effort for the NEXT open (the
// current window falls back to the brand icon); wait=true blocks (shortcut
// creation points a .lnk at the path, so it must exist now).
func (a *App) ensureAppIcon(id string, wait bool) {
	path := a.appIconPath(id)
	if path == "" || a.eng == nil {
		return
	}
	migrateOnce.Do(func() { a.migrateAppIcons(filepath.Dir(path)) })
	if realIconCached(path) {
		return
	}
	fetch := func() {
		ensureIconFile(path, func(p string) error { return a.fetchAppIcon(id, p) })
	}
	if wait {
		fetch()
	} else {
		go fetch()
	}
}

// realIconCached reports whether the icon at path exists and is the real app
// icon. One marked as the brand fallback (a past fetch failed) doesn't count,
// so a transient server error can't brand-icon the app forever.
func realIconCached(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return false
	}
	_, err := os.Stat(path + ".fallback")
	return err != nil
}

// iconLocks holds one mutex per icon path (path -> *sync.Mutex).
var iconLocks sync.Map

// ensureIconFile fetches the icon at path, or on failure writes the brand icon
// there and marks it as the fallback: a shortcut points at the path, so
// something must exist. Callers for the same path take turns, and a caller
// that waited finds the icon already there. Opening an app while the icon
// migration regenerated it ran two fetches at once, and the second rename onto
// the finished icon ("Access is denied") went down the failure path and wrote
// the brand icon over the good one (#743).
func ensureIconFile(path string, fetch func(path string) error) {
	mu, _ := iconLocks.LoadOrStore(path, new(sync.Mutex))
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()
	if realIconCached(path) {
		return
	}
	marker := path + ".fallback"
	if err := ensureAppIconsDir(filepath.Dir(path)); err != nil {
		return
	}
	if err := fetch(path); err != nil {
		slog.Debug("app icon fetch failed; using brand icon", "path", path, "err", err)
		// Fallback re-encodes the brand image (shell-safe DIB frames) rather
		// than copying the raw embedded ico, whose all-PNG frames the shell
		// icon extractor can't render for .lnk/pin icons.
		if brandBadge() != nil {
			if err := writeIcoFrames(path, brandFrame); err == nil {
				_ = os.WriteFile(marker, nil, 0o600)
				return
			}
		}
		_ = os.WriteFile(path, navIconICO, 0o600) // last resort
		_ = os.WriteFile(marker, nil, 0o600)
		return
	}
	_ = os.Remove(marker) // real icon landed — clear any fallback flag
}

// fetchAppIcon downloads the theming PNG for an app and writes a multi-size ICO.
func (a *App) fetchAppIcon(id, path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	url := a.absURL("/index.php/apps/theming/icon/" + id)
	req, err := a.eng.Client().NewRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := a.eng.Client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errStatus(resp.StatusCode)
	}
	src, err := png.Decode(resp.Body)
	if err != nil {
		return err
	}
	// The "via OtherSync" corner badge goes on per size, so each frame carries
	// the mark drawn for its size rather than the 256 one shrunk to a smudge.
	return writeIcoFrames(path, func(s int) image.Image {
		return badgeAppIcon(resize.Resize(uint(s), uint(s), src, resize.Lanczos3))
	})
}

// writeIcoFromImage renders an image into a multi-size .ico at path
// (atomically, via a unique temp file — concurrent fetches must not share one).
func writeIcoFromImage(path string, src image.Image) error {
	return writeIcoFrames(path, func(s int) image.Image {
		return resize.Resize(uint(s), uint(s), src, resize.Lanczos3)
	})
}

// writeIcoFrames writes a multi-size ICO whose frame at each size is
// frame(size).
func writeIcoFrames(path string, frame func(int) image.Image) error {
	var frames [][]byte
	for _, s := range icoSizes {
		img := frame(s)
		var frame []byte
		if s >= 256 {
			var b bytes.Buffer
			if err := png.Encode(&b, img); err != nil {
				return err
			}
			frame = b.Bytes()
		} else {
			frame = dibFrame(img, s)
		}
		frames = append(frames, frame)
	}
	ico := encodeICO(icoSizes, frames)
	tmp, err := os.CreateTemp(filepath.Dir(path), ".ico-*.tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(ico); err != nil {
		tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return nil
}

// dibFrame encodes one icon frame as a classic 32bpp DIB (BITMAPINFOHEADER +
// bottom-up straight-alpha BGRA + an all-clear AND mask). The shell's icon
// extractor requires this for sizes below 256 — it renders PNG-compressed
// small frames as a blank page (while LoadImageW copes), which broke taskbar
// pin icons.
func dibFrame(img image.Image, s int) []byte {
	var buf bytes.Buffer
	maskStride := ((s + 31) / 32) * 4
	binary.Write(&buf, binary.LittleEndian, uint32(40))                 // biSize
	binary.Write(&buf, binary.LittleEndian, int32(s))                   // biWidth
	binary.Write(&buf, binary.LittleEndian, int32(2*s))                 // biHeight (XOR + AND)
	binary.Write(&buf, binary.LittleEndian, uint16(1))                  // biPlanes
	binary.Write(&buf, binary.LittleEndian, uint16(32))                 // biBitCount
	binary.Write(&buf, binary.LittleEndian, uint32(0))                  // biCompression = BI_RGB
	binary.Write(&buf, binary.LittleEndian, uint32(s*s*4+maskStride*s)) // biSizeImage
	buf.Write(make([]byte, 16))                                         // XPels/YPels/ClrUsed/ClrImportant
	b := img.Bounds()
	for y := s - 1; y >= 0; y-- { // bottom-up
		for x := 0; x < s; x++ {
			// NRGBA = straight (non-premultiplied) alpha, which icon DIBs expect.
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			buf.Write([]byte{c.B, c.G, c.R, c.A})
		}
	}
	buf.Write(make([]byte, maskStride*s)) // AND mask: all visible; alpha governs
	return buf.Bytes()
}

type errStatus int

func (e errStatus) Error() string { return "server returned status " + http.StatusText(int(e)) }

// encodeICO packs PNG-encoded frames into a Vista+ PNG-payload .ico
// (same format the brand icon generator emits — shell/windows/appicon).
func encodeICO(sizes []int, pngs [][]byte) []byte {
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, uint16(0)) // reserved
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // type: icon
	binary.Write(&buf, binary.LittleEndian, uint16(len(sizes)))
	offset := 6 + 16*len(sizes)
	for i, s := range sizes {
		w := s
		if w >= 256 {
			w = 0 // 0 encodes 256
		}
		buf.WriteByte(byte(w))
		buf.WriteByte(byte(w))
		buf.WriteByte(0)                                    // palette
		buf.WriteByte(0)                                    // reserved
		binary.Write(&buf, binary.LittleEndian, uint16(1))  // planes
		binary.Write(&buf, binary.LittleEndian, uint16(32)) // bpp
		binary.Write(&buf, binary.LittleEndian, uint32(len(pngs[i])))
		binary.Write(&buf, binary.LittleEndian, uint32(offset))
		offset += len(pngs[i])
	}
	for _, p := range pngs {
		buf.Write(p)
	}
	return buf.Bytes()
}

// sanitizeFileName strips characters Windows forbids in file names.
func sanitizeFileName(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '<', '>', ':', '"', '/', '\\', '|', '?', '*':
			return '-'
		}
		if r < 0x20 {
			return '-'
		}
		return r
	}, s)
}
