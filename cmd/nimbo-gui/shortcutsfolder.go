package main

import (
	"strings"
	"unicode/utf16"
)

// The Start-menu folder's shown name. The folder itself keeps the name the
// install first gave it (shortcutsFolderName, "Nimbo Apps"): the shortcuts
// live in it, the icons pinned shortcuts point at live in it, and the
// manifest's virtualization:ExcludedDirectory names it. After a rename it is
// given the new product's name to show instead ("OtherSync Apps"), the way
// Windows names its own Start-menu folders: a desktop.ini with
// LocalizedResourceName, which the shell reads only from a folder marked
// read-only (or system). See labelShortcutsFolder.

// shortcutsFolderININame is the file that gives the folder its shown name.
const shortcutsFolderININame = "desktop.ini"

// shortcutsFolderINI is the desktop.ini that shows the folder as name. It is
// UTF-16LE with a BOM, the encoding the shell reads any name from (an ANSI
// file would mangle anything outside the system code page).
func shortcutsFolderINI(name string) []byte {
	text := "[.ShellClassInfo]\r\nLocalizedResourceName=" + name + "\r\n"
	u := utf16.Encode([]rune(text))
	b := make([]byte, 2, 2+2*len(u))
	b[0], b[1] = 0xFF, 0xFE
	for _, c := range u {
		b = append(b, byte(c), byte(c>>8))
	}
	return b
}

// shortcutsFolderSpent reports whether a folder holding these entries has no
// shortcuts left, only what we keep in it ourselves: the hidden icon folder
// and the desktop.ini that names it.
func shortcutsFolderSpent(names []string) bool {
	for _, n := range names {
		if n != appIconsFolder && !strings.EqualFold(n, shortcutsFolderININame) {
			return false
		}
	}
	return true
}
