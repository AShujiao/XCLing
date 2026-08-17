//go:build windows

package service

import (
	"testing"

	"XCLing/internal/platform"
)

func TestGetApplyStatusAllowsExactInertRootOnEligibleMachine(t *testing.T) {
	snapshot, err := platform.InspectSRPRoot()
	if err != nil {
		t.Fatal(err)
	}
	if platform.ClassifySRPRoot(snapshot) != platform.SRPDispositionInertUnrestricted {
		t.Skip("machine does not have the exact inert SRP fixture")
	}
	if !platform.IsAdmin() {
		t.Skip("status requires an elevated process")
	}
	joined, err := platform.IsDomainJoined()
	if err != nil {
		t.Fatal(err)
	}
	if joined {
		t.Skip("domain members are intentionally blocked")
	}
	status, err := NewApplyService().GetApplyStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !status.CanApply || !status.WillReplaceLegacy || status.SrpDisposition != platform.SRPDispositionInertUnrestricted {
		t.Fatalf("unexpected status: %+v", status)
	}
}
