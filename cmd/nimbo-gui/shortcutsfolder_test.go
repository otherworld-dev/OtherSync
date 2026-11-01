package main

import (
	"bytes"
	"testing"
	"unicode/utf16"
)

// desktop.ini is read as UTF-16LE when it starts with that BOM, which is the
// one encoding that is safe for any product name.
func TestShortcutsFolderINI(t *testing.T) {
	got := shortcutsFolderINI("OtherSync Apps")
	if !bytes.HasPrefix(got, []byte{0xFF, 0xFE}) {
		t.Fatalf("no UTF-16LE BOM: % x", got[:min(len(got), 4)])
	}
	body := got[2:]
	if len(body)%2 != 0 {
		t.Fatalf("odd number of bytes after the BOM: %d", len(body))
	}
	u := make([]uint16, len(body)/2)
	for i := range u {
		u[i] = uint16(body[2*i]) | uint16(body[2*i+1])<<8
	}
	want := "[.ShellClassInfo]\r\nLocalizedResourceName=OtherSync Apps\r\n"
	if s := string(utf16.Decode(u)); s != want {
		t.Errorf("content = %q, want %q", s, want)
	}
	// A name outside ASCII survives (the reason for UTF-16).
	if !bytes.Contains(shortcutsFolderINI("Ørsted Apps"), []byte{0xD8, 0x00}) {
		t.Error("Ø was not written as UTF-16LE")
	}
}

// The folder is spent once nothing but our own furniture is left in it: the
// hidden icon folder and the desktop.ini that names it.
func TestShortcutsFolderSpent(t *testing.T) {
	for _, c := range []struct {
		names []string
		want  bool
	}{
		{nil, true},
		{[]string{appIconsFolder}, true},
		{[]string{shortcutsFolderININame}, true},
		{[]string{appIconsFolder, "Desktop.ini"}, true},
		{[]string{appIconsFolder, shortcutsFolderININame, "Deck.lnk"}, false},
		{[]string{"notes.txt"}, false},
	} {
		if got := shortcutsFolderSpent(c.names); got != c.want {
			t.Errorf("shortcutsFolderSpent(%q) = %v, want %v", c.names, got, c.want)
		}
	}
}
