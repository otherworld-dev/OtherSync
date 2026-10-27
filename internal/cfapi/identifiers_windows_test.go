//go:build windows

package cfapi

import (
	"os/user"
	"testing"
)

// Sync roots are registered and found by these values; the rename to OtherSync
// must not change any of them, or Windows sees existing roots as someone
// else's.
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
		"ProviderName":  {providerName, "Nimbo"},
		"provider GUID": {providerID.String(), "{7C9F2B41-5E3A-4D88-9C16-2A0F8B7D3E20}"},
		"sync-root id":  {id, "Nimbo!" + u.Uid + "!cdbb2df7"},
	} {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", name, c.got, c.want)
		}
	}
}
