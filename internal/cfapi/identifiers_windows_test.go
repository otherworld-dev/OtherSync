//go:build windows

package cfapi

import (
	"os/user"
	"testing"

	"github.com/otherworld/nimbo/internal/brand"
)

// Sync roots are registered and found by these values; the rename to OtherSync
// must not change any of them, or Windows sees existing roots as someone
// else's.
//
// The ProviderName is deliberately not among them. It is a display string
// (Windows shows it beside the root, e.g. in Settings → Storage), not part of
// what a root is matched by, and every registration is made with
// CF_REGISTER_FLAG_UPDATE on the same path and identity, so an existing root
// takes the new name in place. See TestProviderNameIsTheBrandName.
func TestIdentifiersStayNimbo(t *testing.T) {
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	id, err := shellRootID(`C:\Users\x\Nimbo - adam`)
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct{ got, want string }{
		"provider GUID": {providerID.String(), "{7C9F2B41-5E3A-4D88-9C16-2A0F8B7D3E20}"},
		"sync-root id":  {id, "Nimbo!" + u.Uid + "!cdbb2df7"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", name, c.got, c.want)
		}
	}
}

// The provider is shown under the product's name, so a partner build shows
// its own and the stock build says OtherSync.
func TestProviderNameIsTheBrandName(t *testing.T) {
	if got := providerName(); got != brand.Current.Name || got == "" {
		t.Errorf("providerName() = %q, want %q", got, brand.Current.Name)
	}
}
