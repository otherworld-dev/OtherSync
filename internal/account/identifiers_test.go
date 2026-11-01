package account

import "testing"

// App passwords are stored under this keychain service; a new one after the
// rename to OtherSync would leave every install signed out.
func TestIdentifiersStayNimbo(t *testing.T) {
	if keychainService != "nimbo" {
		t.Errorf("keychainService = %q, want nimbo", keychainService)
	}
}
