//go:build windows

package platform

import (
	"os"
	"testing"

	pgapply "XCLing/internal/apply"
	"XCLing/internal/model"

	"golang.org/x/sys/windows/registry"
)

func TestSRPWriterIntegration_ExplicitOptInOnly(t *testing.T) {
	if os.Getenv("POLICYGUARD_SRP_INTEGRATION") != "1" {
		t.Skip("set POLICYGUARD_SRP_INTEGRATION=1 in an isolated VM")
	}
	if !IsAdmin() {
		t.Fatal("integration test requires an elevated process")
	}
	if joined, err := IsDomainJoined(); err != nil || joined {
		t.Fatalf("domain check: joined=%v err=%v", joined, err)
	}
	if exists, err := SRPKeyExists(); err != nil || exists {
		t.Fatalf("requires absent SRP root: exists=%v err=%v", exists, err)
	}
	plan := pgapply.Plan{DefaultLevel: 0, PolicyScope: 1, TransparentEnabled: 1, Rules: []pgapply.Rule{{ID: "{8d3e661d-58d3-4a2d-b3ef-a5960851f63b}", Path: os.Args[0], Description: "XCLing integration test", Level: model.SrpLevelUnrestrictedRaw}}}
	if err := WriteSRPPlan(plan); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RemoveSRPPlan(plan) })
	actual, err := ReadSRPPlan()
	if err != nil {
		t.Fatal(err)
	}
	if pgapply.Fingerprint(actual) != pgapply.Fingerprint(plan) {
		t.Fatal("write/read fingerprint mismatch")
	}
	ruleKey, err := registry.OpenKey(registry.LOCAL_MACHINE, unrestrictedPaths+`\`+plan.Rules[0].ID, registry.READ)
	if err != nil {
		t.Fatal(err)
	}
	_, itemType, itemErr := ruleKey.GetStringValue("ItemData")
	_ = ruleKey.Close()
	if itemErr != nil || itemType != registry.EXPAND_SZ {
		t.Fatalf("ItemData type=%d err=%v, want REG_EXPAND_SZ", itemType, itemErr)
	}
	if err := RemoveSRPPlan(plan); err != nil {
		t.Fatal(err)
	}
	if exists, err := SRPKeyExists(); err != nil || exists {
		t.Fatalf("cleanup failed: exists=%v err=%v", exists, err)
	}
}

func TestSRPWriterIntegration_RestoresInertUnrestrictedState(t *testing.T) {
	if os.Getenv("POLICYGUARD_SRP_INERT_INTEGRATION") != "1" {
		t.Skip("set POLICYGUARD_SRP_INERT_INTEGRATION=1 only in an isolated VM")
	}
	if !IsAdmin() {
		t.Fatal("integration test requires an elevated process")
	}
	before, err := InspectSRPRoot()
	if err != nil {
		t.Fatal(err)
	}
	if ClassifySRPRoot(before) != SRPDispositionInertUnrestricted {
		t.Fatal("requires exact inert_unrestricted SRP fixture")
	}
	plan := pgapply.Plan{DefaultLevel: 0, PolicyScope: 1, TransparentEnabled: 1, Rules: []pgapply.Rule{{ID: "{2f141f0c-a3c4-4456-871f-31c2456d21e0}", Path: os.Args[0], Description: "XCLing inert integration test", Level: model.SrpLevelUnrestrictedRaw}}}
	if err := WriteSRPPlanFrom(plan, before); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = RestoreSRPBeforeState(plan, SRPDispositionInertUnrestricted) })
	if err := RestoreSRPBeforeState(plan, SRPDispositionInertUnrestricted); err != nil {
		t.Fatal(err)
	}
	after, err := InspectSRPRoot()
	if err != nil {
		t.Fatal(err)
	}
	if !SameSRPRootSnapshot(before, after) {
		t.Fatalf("restored snapshot differs: before=%+v after=%+v", before, after)
	}
}
