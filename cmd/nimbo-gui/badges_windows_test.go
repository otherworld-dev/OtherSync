package main

import (
	"strings"
	"testing"
)

func TestPathSegment(t *testing.T) {
	cases := []struct{ name, want string }{
		{"OtherSync", "OtherSync"},
		{"Acme Sync", "Acme Sync"},
		{`Acme: Sync/Pro?`, "Acme SyncPro"},
		{"Acme.  ", "Acme"},
		{`<>:"/\|?*`, "fallback"},
		{"", "fallback"},
	}
	for _, c := range cases {
		if got := pathSegment(c.name, "fallback"); got != c.want {
			t.Errorf("pathSegment(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

// The stock build's badges must move out of the pre-rename folder, never back
// into it, or the clean-up would delete the copy Explorer is using.
func TestBadgeFolderIsNotLegacy(t *testing.T) {
	if strings.EqualFold(badgeFolder(), legacyBadgeFolder) {
		t.Fatalf("badgeFolder() = %q, the legacy folder", badgeFolder())
	}
}
