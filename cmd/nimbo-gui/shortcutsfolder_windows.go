//go:build windows

package main

import (
	"bytes"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/otherworld/nimbo/internal/brand"
)

const shcneUpdateDir = 0x00001000

// labelShortcutsFolder shows the Start-menu folder under the product's name
// after a rename ("Nimbo Apps" is shown as "OtherSync Apps"), without moving
// it: it writes, or corrects, the folder's desktop.ini and marks the folder
// read-only so the shell reads it. It never creates the folder, and does
// nothing for a brand that was never renamed, whose folder already has its
// name. Called whenever the folder may just have been made (a shortcut or the
// icon folder written) and once at startup. Best-effort: an unnamed folder
// still works, it just shows its real name.
func labelShortcutsFolder() {
	if !brand.Current.Renamed() {
		return
	}
	dir := shortcutsDir()
	if dir == "" {
		return
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return
	}
	ini := filepath.Join(dir, shortcutsFolderININame)
	changed, err := writeHiddenSystemFile(ini, shortcutsFolderINI(brand.Current.Name+" Apps"))
	if err != nil {
		slog.Warn("could not name the Start-menu folder", "dir", dir, "err", err)
		return
	}
	if setAttr(dir, windows.FILE_ATTRIBUTE_READONLY, true) {
		changed = true
	}
	if changed {
		if p, err := windows.UTF16PtrFromString(dir); err == nil {
			procSHChangeNotify.Call(shcneUpdateDir, shcnfPathW|shcnfFlush, uintptr(unsafe.Pointer(p)), 0)
		}
		slog.Info("named the Start-menu folder", "dir", dir, "name", brand.Current.Name+" Apps")
	}
}

// writeHiddenSystemFile makes path hold exactly data, hidden and system, and
// reports whether it had to change anything. The attributes come off before a
// rewrite: Windows refuses to overwrite a hidden or system file opened
// without them.
func writeHiddenSystemFile(path string, data []byte) (bool, error) {
	changed := false
	if cur, err := os.ReadFile(path); err != nil || !bytes.Equal(cur, data) {
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		if err == nil {
			if p, perr := windows.UTF16PtrFromString(path); perr == nil {
				_ = windows.SetFileAttributes(p, windows.FILE_ATTRIBUTE_NORMAL)
			}
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return false, err
		}
		changed = true
	}
	if setAttr(path, windows.FILE_ATTRIBUTE_HIDDEN|windows.FILE_ATTRIBUTE_SYSTEM, true) {
		changed = true
	}
	return changed, nil
}

// setAttr sets (on) or clears (!on) the attribute bits attr on path, and
// reports whether it changed them. Best-effort.
func setAttr(path string, attr uint32, on bool) bool {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	cur, err := windows.GetFileAttributes(p)
	if err != nil {
		return false
	}
	next := cur &^ attr
	if on {
		next = cur | attr
	}
	if next == cur {
		return false
	}
	if next == 0 {
		next = windows.FILE_ATTRIBUTE_NORMAL // only valid alone, and 0 isn't
	}
	return windows.SetFileAttributes(p, next) == nil
}

// unlabelShortcutsFolder undoes labelShortcutsFolder ahead of removing the
// folder: the desktop.ini goes and the folder loses read-only, which
// RemoveDirectory would otherwise refuse.
func unlabelShortcutsFolder(dir string) {
	ini := filepath.Join(dir, shortcutsFolderININame)
	setAttr(ini, windows.FILE_ATTRIBUTE_HIDDEN|windows.FILE_ATTRIBUTE_SYSTEM|windows.FILE_ATTRIBUTE_READONLY, false)
	_ = os.Remove(ini)
	setAttr(dir, windows.FILE_ATTRIBUTE_READONLY, false)
}
