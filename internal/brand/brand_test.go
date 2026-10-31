package brand

import "testing"

// TestStockBrandLoaded checks the embedded brand.json parses into sane stock
// values (a white-label build swaps the file; this guards the default).
func TestStockBrandLoaded(t *testing.T) {
	if Current.Name == "" {
		t.Fatal("brand name is empty — brand.json failed to load")
	}
	for field, v := range map[string]string{
		"name": Current.Name, "company": Current.Company, "website": Current.Website,
		"feedUrl": Current.FeedURL, "apiBase": Current.APIBase, "appId": Current.AppID,
		"helpUrl":   Current.HelpURL,
		"accentHex": Current.AccentHex,
	} {
		if v == "" {
			t.Errorf("brand field %q is empty", field)
		}
	}
	if Current.AccentHex[0] != '#' {
		t.Errorf("accentHex %q is not a hex colour", Current.AccentHex)
	}
}

// TestHelpPage pins how help links are built, including the white-label case
// where a brand has no help site and every help link must disappear.
func TestHelpPage(t *testing.T) {
	cases := []struct {
		base, slug, want string
	}{
		{"https://example.com/help/", "", "https://example.com/help/"},
		{"https://example.com/help/", "file-modes", "https://example.com/help/file-modes.html"},
		{"https://example.com/help", "faq", "https://example.com/help/faq.html"},
		{"https://example.com/help", "", "https://example.com/help/"},
		{"", "faq", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		if got := (Brand{HelpURL: c.base}).HelpPage(c.slug); got != c.want {
			t.Errorf("HelpPage(%q) with base %q = %q, want %q", c.slug, c.base, got, c.want)
		}
	}
}

// The rename to OtherSync is display-only. Everything an existing install, or
// Windows, recognises the app by keeps its old value: the app id (AUMID,
// config/data dir, self-update task), the update feed, and the name things
// already on disk were created under (InstalledName).
func TestStockRenameKeepsIdentity(t *testing.T) {
	if Current.Name != "OtherSync" {
		t.Errorf("Name = %q, want OtherSync", Current.Name)
	}
	if Current.AppID != "Nimbo" {
		t.Errorf("AppID = %q, want Nimbo (the AUMID, config dir and task names hang off it)", Current.AppID)
	}
	if got := Current.InstalledName(); got != "Nimbo" {
		t.Errorf("InstalledName() = %q, want Nimbo (Start-menu folder, icon folder, existing labels)", got)
	}
	if want := "https://github.com/otherworld-dev/OtherSync/releases/latest/download/Nimbo.appinstaller"; Current.FeedURL != want {
		t.Errorf("FeedURL = %q, want %q", Current.FeedURL, want)
	}
	if want := "https://api.github.com/repos/otherworld-dev/OtherSync"; Current.APIBase != want {
		t.Errorf("APIBase = %q, want %q", Current.APIBase, want)
	}
}

// The fallback used when brand.json is broken must be the same brand, or a
// broken build would quietly move every identifier back to other values.
func TestStockFallbackMatchesBrandJSON(t *testing.T) {
	if got := stockBrand(); got != Current {
		t.Errorf("stockBrand() = %+v\nbrand.json  = %+v", got, Current)
	}
}

// InstalledName is FormerName when the product has been renamed, else Name, so
// a brand that was never renamed (a white-label build) keeps today's values.
func TestInstalledName(t *testing.T) {
	if got := (Brand{Name: "AcmeCloud Sync"}).InstalledName(); got != "AcmeCloud Sync" {
		t.Errorf("never renamed: %q", got)
	}
	if got := (Brand{Name: "OtherSync", FormerName: "Nimbo"}).InstalledName(); got != "Nimbo" {
		t.Errorf("renamed: %q", got)
	}
}

// formerName "Nimbo" belongs to the stock build. A white-label brand copied
// from the stock file must not inherit it, or it would share the stock Start
// menu folder and icon folder and label its accounts "Nimbo - <user>".
func TestValidateFormerName(t *testing.T) {
	for _, c := range []struct {
		b  Brand
		ok bool
	}{
		{stockBrand(), true},
		{Brand{Name: "AcmeCloud Sync", AppID: "AcmeCloud"}, true},
		{Brand{Name: "AcmeCloud Sync", FormerName: "Nimbo", AppID: "AcmeCloud"}, false},
		{Brand{Name: "Nimbo", FormerName: "Nimbo", AppID: "Nimbo"}, false},
		{Brand{Name: "Acme Two", FormerName: "Acme One", AppID: "AcmeOne"}, true},
	} {
		if err := c.b.Validate(); (err == nil) != c.ok {
			t.Errorf("%+v: Validate() = %v, want ok=%v", c.b, err, c.ok)
		}
	}
}

// A brand that fails the check runs as never renamed rather than wearing the
// stock build's former name.
func TestSanitizeDropsBadFormerName(t *testing.T) {
	b := sanitize(Brand{Name: "AcmeCloud Sync", FormerName: "Nimbo", AppID: "AcmeCloud"})
	if b.FormerName != "" || b.Renamed() || b.InstalledName() != "AcmeCloud Sync" {
		t.Errorf("got %+v", b)
	}
	if got := sanitize(stockBrand()); got != stockBrand() {
		t.Errorf("stock brand changed: %+v", got)
	}
}
