//go:build windows

package shellmenu

import "testing"

// The production keys the menu is written to (not a test's scratch roots):
// they stay "Nimbo" after the rename to OtherSync, so Register replaces an
// existing install's menu instead of adding a second one beside it.
func TestIdentifiersStayNimbo(t *testing.T) {
	for name, c := range map[string]struct{ got, want string }{
		"files key":   {fileRoot, `Software\Classes\*\shell\Nimbo`},
		"folders key": {dirRoot, `Software\Classes\Directory\shell\Nimbo`},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", name, c.got, c.want)
		}
	}
}
