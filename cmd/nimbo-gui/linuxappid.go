package main

import (
	"regexp"
	"strings"

	"github.com/otherworld/nimbo/internal/brand"
)

// linuxApplicationID is the GTK application id the Linux build runs under.
// Wails derives it from Options.Name ("org.wails." + a sanitised name) unless
// it is set, so the rename to OtherSync would have changed it from
// "org.wails.nimbo" to "org.wails.othersync". It is pinned to the installed
// name instead, reproducing Wails' own derivation (wails v3 beta.26,
// application_linux_appid.go sanitizeAppName).
func linuxApplicationID() string { return "org.wails." + wailsSanitizeAppName(brand.Current.InstalledName()) }

var (
	wailsInvalidAppNameChars = regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	wailsLeadingDigits       = regexp.MustCompile(`^[0-9]+`)
)

func wailsSanitizeAppName(name string) string {
	name = wailsInvalidAppNameChars.ReplaceAllString(name, "_")
	for strings.Contains(name, "__") {
		name = strings.ReplaceAll(name, "__", "_")
	}
	name = strings.Trim(name, "_")
	if name == "" {
		name = "wailsapp"
	}
	name = wailsLeadingDigits.ReplaceAllString(name, "_$0")
	return strings.ToLower(name)
}
