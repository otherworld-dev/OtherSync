//go:build windows

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/otherworld/nimbo/internal/brand"
)

func fileAttrs(t *testing.T, p string) uint32 {
	t.Helper()
	u, err := windows.UTF16PtrFromString(p)
	if err != nil {
		t.Fatal(err)
	}
	a, err := windows.GetFileAttributes(u)
	if err != nil {
		t.Fatalf("attributes of %s: %v", p, err)
	}
	return a
}

// withBrand swaps the current brand for the length of a test.
func withBrand(t *testing.T, b brand.Brand) {
	t.Helper()
	old := brand.Current
	brand.Current = b
	t.Cleanup(func() { brand.Current = old })
}

// After the rename the Start-menu folder keeps its path, "Nimbo Apps", and
// is shown as "OtherSync Apps": a hidden, system desktop.ini names it, and the
// folder is read-only, which is what makes the shell read that file. A wrong
// or stale desktop.ini is corrected; a folder that doesn't exist isn't made.
func TestLabelShortcutsFolder(t *testing.T) {
	withBrand(t, brand.Brand{Name: "OtherSync", FormerName: "Nimbo", AppID: "Nimbo"})
	t.Setenv("APPDATA", t.TempDir())
	dir := shortcutsDir()

	labelShortcutsFolder()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("the folder was created: %v", err)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	ini := filepath.Join(dir, shortcutsFolderININame)
	check := func(when string) {
		t.Helper()
		got, err := os.ReadFile(ini)
		if err != nil {
			t.Fatalf("%s: %v", when, err)
		}
		if want := shortcutsFolderINI("OtherSync Apps"); !bytes.Equal(got, want) {
			t.Errorf("%s: desktop.ini = % x, want % x", when, got, want)
		}
		if a := fileAttrs(t, ini); a&windows.FILE_ATTRIBUTE_HIDDEN == 0 || a&windows.FILE_ATTRIBUTE_SYSTEM == 0 {
			t.Errorf("%s: desktop.ini attributes %#x, want hidden and system", when, a)
		}
		if a := fileAttrs(t, dir); a&windows.FILE_ATTRIBUTE_READONLY == 0 {
			t.Errorf("%s: folder attributes %#x, want read-only", when, a)
		}
	}
	labelShortcutsFolder()
	check("first")

	// A stale one (another name, hidden and system already) is rewritten.
	withBrand(t, brand.Brand{Name: "Elsewhere", FormerName: "Nimbo", AppID: "Nimbo"})
	labelShortcutsFolder()
	if got, _ := os.ReadFile(ini); !bytes.Equal(got, shortcutsFolderINI("Elsewhere Apps")) {
		t.Errorf("stale desktop.ini not corrected: % x", got)
	}
	withBrand(t, brand.Brand{Name: "OtherSync", FormerName: "Nimbo", AppID: "Nimbo"})
	labelShortcutsFolder()
	check("corrected")
}

// A brand that was never renamed has a folder already named after it: no
// desktop.ini is written.
func TestLabelShortcutsFolderNotRenamed(t *testing.T) {
	withBrand(t, brand.Brand{Name: "AcmeCloud Sync", AppID: "AcmeCloud"})
	t.Setenv("APPDATA", t.TempDir())
	dir := shortcutsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	labelShortcutsFolder()
	if _, err := os.Stat(filepath.Join(dir, shortcutsFolderININame)); !os.IsNotExist(err) {
		t.Errorf("desktop.ini written for a brand that was never renamed: %v", err)
	}
	if a := fileAttrs(t, dir); a&windows.FILE_ATTRIBUTE_READONLY != 0 {
		t.Errorf("folder made read-only: %#x", a)
	}
}

// Removing the last shortcut still prunes the folder: desktop.ini and the icon
// folder don't count as contents, and the read-only folder is removable. With
// a shortcut left the folder, and its name, stay.
func TestRemoveShortcutFilePrunesLabelledFolder(t *testing.T) {
	withBrand(t, brand.Brand{Name: "OtherSync", FormerName: "Nimbo", AppID: "Nimbo"})
	t.Setenv("APPDATA", t.TempDir())
	dir := shortcutsDir()
	if err := ensureAppIconsDir(filepath.Join(dir, appIconsFolder)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, appIconsFolder, "deck-4.ico"), []byte("ico"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"Deck.lnk", "Notes.lnk"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("lnk"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	labelShortcutsFolder()

	if err := removeShortcutFile("Deck.lnk"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, shortcutsFolderININame)); err != nil {
		t.Errorf("desktop.ini gone while a shortcut is left: %v", err)
	}
	if a := fileAttrs(t, dir); a&windows.FILE_ATTRIBUTE_READONLY == 0 {
		t.Errorf("folder lost read-only while a shortcut is left: %#x", a)
	}

	if err := removeShortcutFile("Notes.lnk"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		entries, _ := os.ReadDir(dir)
		t.Errorf("folder not pruned (%v), left: %v", err, entries)
	}
}
