package main

import "testing"

// The Linux build's GTK application id stays what the Nimbo build had.
func TestLinuxApplicationIDUnchanged(t *testing.T) {
	if got := linuxApplicationID(); got != "org.wails.nimbo" {
		t.Errorf("linuxApplicationID() = %q, want org.wails.nimbo", got)
	}
	for in, want := range map[string]string{"AcmeCloud Sync": "acmecloud_sync", "1Password": "_1password", "!!": "wailsapp"} {
		if got := wailsSanitizeAppName(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}
