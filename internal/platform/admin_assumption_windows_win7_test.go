//go:build windows && win7

package platform

import "testing"

func TestWin7CompatibilityDoesNotGateOnAdminToken(t *testing.T) {
	if !IsAdmin() {
		t.Fatal("Win7 compatibility build must not gate workflows on UAC token detection")
	}
}
