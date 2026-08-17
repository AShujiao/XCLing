//go:build windows

package platform

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// RefreshSRPPolicy asks Windows to re-read computer policy immediately.
// Windows 7 can otherwise retain designated file types and path rules from
// the previous logon session after direct policy-registry updates.
func RefreshSRPPolicy() error {
	proc := windows.NewLazySystemDLL("userenv.dll").NewProc("RefreshPolicyEx")
	const (
		machine = 1
		rpForce = 0x00000001
	)
	result, _, callErr := proc.Call(machine, rpForce)
	if result != 0 {
		return nil
	}
	if callErr != nil {
		return fmt.Errorf("refresh Windows policy: %w", callErr)
	}
	return fmt.Errorf("refresh Windows policy failed")
}
