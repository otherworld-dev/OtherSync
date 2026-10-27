//go:build windows

package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// The rename to OtherSync must not change anything Windows or an existing
// install already knows the app by: these are the values a 0.2.0 install
// created, and each must come out the same.
func TestRenameKeepsWindowsIdentifiers(t *testing.T) {
	for name, c := range map[string]struct{ got, want string }{
		"Start-menu folder":   {shortcutsFolderName(), "Nimbo Apps"},
		"launcher alias":      {launcherAlias(), "nimbo-app.exe"},
		"self-update task":    {selfUpdateTaskName(), "NimboSelfUpdate"},
		"app-window AUMID":    {aumidFor("files"), "Nimbo.App.files"},
		"sidebar icon folder": {sidebarIconFolder(), "Nimbo"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", name, c.got, c.want)
		}
	}
	t.Setenv("APPDATA", `C:\Users\x\AppData\Roaming`)
	if got, want := shortcutsDir(), filepath.Join(`C:\Users\x\AppData\Roaming`, "Microsoft", "Windows", "Start Menu", "Programs", "Nimbo Apps"); got != want {
		t.Errorf("shortcutsDir = %q, want %q", got, want)
	}
	if got := appIconsDirFor(shortcutsDir(), ""); !strings.HasPrefix(got, shortcutsDir()) {
		t.Errorf("app icons left the Start-menu folder: %q", got)
	}
}
