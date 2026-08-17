package service

import (
	"testing"

	pgapply "XCLing/internal/apply"
	"XCLing/internal/model"
)

func TestRecoveryRecordWithPlanPreservesOriginalSnapshot(t *testing.T) {
	before := model.RegistryTreeSnapshot{Exists: true, Root: model.RegistryKeySnapshot{Values: []model.RegistryValueSnapshot{{Name: "DefaultLevel", Type: 4, Data: []byte{0, 0, 4, 0}}}}}
	record := model.RecoveryRecord{SchemaVersion: "2", BeforeState: model.BeforeStateManaged, BeforeSnapshot: before, CreatedAt: "original"}
	plan := pgapply.Plan{PolicyName: "updated", DefaultLevel: 0, PolicyScope: 0, TransparentEnabled: 1, Rules: []pgapply.Rule{{ID: "{rule}", Path: `D:\Tools\*`, Description: "Tools", Level: model.SrpLevelUnrestrictedRaw}}}

	updated := recoveryRecordWithPlan(record, plan, model.ProtectionStateLocked, "changed")
	if !updated.BeforeSnapshot.Equal(before) || updated.CreatedAt != "original" {
		t.Fatal("rule updates must preserve the first pre-takeover snapshot")
	}
	if updated.RuleCount != 1 || updated.Rules[0].Path != `D:\Tools\*` || updated.ProtectionState != model.ProtectionStateLocked {
		t.Fatalf("updated record does not describe the new plan: %+v", updated)
	}
}
