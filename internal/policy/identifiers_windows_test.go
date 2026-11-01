//go:build windows

package policy

import "testing"

// Admins' GPOs and Intune profiles write here; it stays after the rename to
// OtherSync.
func TestIdentifiersStayNimbo(t *testing.T) {
	if policyKey != `SOFTWARE\Policies\Nimbo` {
		t.Errorf("policyKey = %q", policyKey)
	}
}
