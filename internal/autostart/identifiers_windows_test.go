//go:build windows

package autostart

import "testing"

// Existing installs' autostart entries are found by these names; they stay
// after the rename to OtherSync.
func TestIdentifiersStayNimbo(t *testing.T) {
	for name, c := range map[string]struct{ got, want string }{
		"Run value":          {runValueName, "Nimbo"},
		"StartupTask TaskId": {startupTaskID, "NimboAutostart"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", name, c.got, c.want)
		}
	}
}
