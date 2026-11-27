package main

import "testing"

func TestNormalizeAccentSource(t *testing.T) {
	cases := []struct{ src, custom, want string }{
		{"", "", "brand"},
		{"brand", "#123456", "brand"},
		{"server", "", "server"},
		{"custom", "#3A7BD5", "custom"},
		{"custom", "#3a7bd5", "custom"},
		// "custom" with no usable colour falls back rather than showing nothing.
		{"custom", "", "brand"},
		{"custom", "#12345", "brand"},
		{"custom", "3A7BD5", "brand"},
		{"custom", "#GGGGGG", "brand"},
		// A value from a later version.
		{"rainbow", "#3A7BD5", "brand"},
	}
	for _, c := range cases {
		if got := normalizeAccentSource(c.src, c.custom); got != c.want {
			t.Errorf("normalizeAccentSource(%q, %q) = %q, want %q", c.src, c.custom, got, c.want)
		}
	}
}
