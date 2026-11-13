// Package brand centralises every white-labellable identity value — product
// name, company, URLs, update feed, accent colour, AUMID app id — in one place,
// loaded from an embedded brand.json at build time. A reseller produces a
// branded build by editing brand.json (the app icon regenerates from the accent
// colour) and setting the MSIX manifest Name/Publisher — no code changes. See
// packaging/whitelabel/.
package brand

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
)

//go:embed brand.json
var brandJSON []byte

// Brand is the set of identity values that differ between the stock build and
// a white-label build.
type Brand struct {
	Name         string `json:"name"`         // product name shown to users ("OtherSync")
	FormerName   string `json:"formerName"`   // the name before a rename ("Nimbo"); "" = never renamed. See InstalledName
	Company      string `json:"company"`      // legal entity ("Otherworld Dev Ltd")
	Tagline      string `json:"tagline"`      // short descriptor
	Website      string `json:"website"`      // marketing site
	SupportEmail string `json:"supportEmail"` // shown in About / business page
	HelpURL      string `json:"helpUrl"`      // user help site; empty hides every help link (white-label)
	FeedURL      string `json:"feedUrl"`      // App Installer feed (in-app update target check)
	APIBase      string `json:"apiBase"`      // GitHub releases API root for the update check
	AccentHex    string `json:"accentHex"`    // brand accent (tray badge, UI accent fallback)
	AppID        string `json:"appId"`        // MSIX Application Id (AUMID) + self-update task name + the config/data dir name (slugged)
	// ServerURL presets the Nextcloud server the sign-in window offers, and
	// LockServer pins sign-in to it, the same lock the HKLM admin policy gives
	// but baked into a partner build, so it needs no licence key or registry
	// step on a client's PC (Deck d1010). Both are empty in the stock build.
	ServerURL  string `json:"serverUrl,omitempty"`
	LockServer bool   `json:"lockServer,omitempty"`
}

// Current is the active brand for this build.
var Current Brand

func init() {
	if err := json.Unmarshal(brandJSON, &Current); err != nil {
		// A malformed brand.json is a build mistake; fall back to safe stock
		// values so the app still runs while it's noticed.
		slog.Error("brand.json invalid, using stock defaults", "err", err)
		Current = stockBrand()
	}
	Current = sanitize(Current)
}

// stockFormerName is the stock build's name before the rename to OtherSync.
const stockFormerName = "Nimbo"

// Validate checks the rename fields: a former name differs from the name, and
// the stock build's former name only goes with the stock app id, so a
// white-label brand.json copied from the stock one can't pass as a renamed
// Nimbo.
func (b Brand) Validate() error {
	if b.FormerName == "" {
		return nil
	}
	if b.FormerName == b.Name {
		return fmt.Errorf("brand.json: formerName %q is the same as name", b.FormerName)
	}
	if b.FormerName == stockFormerName && b.AppID != stockFormerName {
		return fmt.Errorf("brand.json: formerName %q is the stock build's, but appId is %q; leave formerName out of a white-label brand", b.FormerName, b.AppID)
	}
	return nil
}

// ValidateServer checks the server preset: an https address with a host, and
// a lock only together with one. build-partner.ps1 refuses a brand that fails
// it, so a broken one should never ship.
func (b Brand) ValidateServer() error {
	if b.ServerURL == "" {
		if b.LockServer {
			return fmt.Errorf("brand.json: lockServer is set but serverUrl is empty")
		}
		return nil
	}
	u, err := url.Parse(b.ServerURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return fmt.Errorf("brand.json: serverUrl %q must be an https:// address", b.ServerURL)
	}
	return nil
}

// sanitize drops a former name that fails Validate, so such a build runs as a
// brand that was never renamed: its own Start-menu folder and labels, no
// rename notice. A server preset that fails ValidateServer is dropped with its
// lock: a lock to an unusable address would leave nobody able to sign in.
func sanitize(b Brand) Brand {
	if err := b.Validate(); err != nil {
		slog.Error("ignoring formerName", "err", err)
		b.FormerName = ""
	}
	if err := b.ValidateServer(); err != nil {
		slog.Error("ignoring serverUrl and lockServer", "err", err)
		b.ServerURL, b.LockServer = "", false
	}
	b.ServerURL = strings.TrimRight(b.ServerURL, "/")
	return b
}

// stockBrand is the stock brand.json, for when the embedded file can't be read.
func stockBrand() Brand {
	return Brand{
		Name: "OtherSync", FormerName: "Nimbo", Company: "Otherworld Dev Ltd",
		Tagline: "Native Nextcloud sync for desktop",
		Website: "https://www.othersync.com", SupportEmail: "contact@otherworld.dev",
		HelpURL:   "https://www.othersync.com/help/",
		FeedURL:   "https://github.com/otherworld-dev/OtherSync/releases/latest/download/OtherSync.appinstaller",
		APIBase:   "https://api.github.com/repos/otherworld-dev/OtherSync",
		AccentHex: "#22D3C4", AppID: "Nimbo",
	}
}

// InstalledName is the product name that names what existing installs have
// already made under it: the Start-menu "<name> Apps" folder (and the app
// icons inside it, which pinned shortcuts point at), the folder the sidebar
// icon is kept in, and the automatic Explorer label of accounts set up before
// a rename. It is FormerName when the product has been renamed, so those keep
// working after the rename, and Name otherwise. Text people read uses Name.
func (b Brand) InstalledName() string {
	if b.FormerName != "" {
		return b.FormerName
	}
	return b.Name
}

// Renamed reports whether this build carries a new product name that existing
// installs knew by another (FormerName).
func (b Brand) Renamed() bool {
	return b.FormerName != "" && b.FormerName != b.Name
}

// Slug turns a name into a filesystem-safe one: lowercased, ASCII letters and
// digits kept, every other run collapsed to a single hyphen, then trimmed. It
// returns "" when nothing usable remains.
func Slug(s string) string {
	var b strings.Builder
	hyphen := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			hyphen = false
		} else if b.Len() > 0 && !hyphen {
			b.WriteByte('-')
			hyphen = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// FileStem is the prefix for files people can see, such as the problem-report
// zip in Downloads and the update helper's files in the home folder: the
// product name as a slug ("othersync"). Saved state and identifiers use AppID
// or InstalledName instead and never change with it.
func (b Brand) FileStem() string {
	if s := Slug(b.Name); s != "" {
		return s
	}
	return Slug(b.AppID)
}

// HelpPage returns the address of one help page ("" = the help home), or ""
// when this brand has no help site, in which case callers hide the link.
func (b Brand) HelpPage(slug string) string {
	if b.HelpURL == "" {
		return ""
	}
	base := strings.TrimSuffix(b.HelpURL, "/") + "/"
	if slug == "" {
		return base
	}
	return base + slug + ".html"
}
