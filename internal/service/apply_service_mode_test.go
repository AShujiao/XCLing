package service

import (
	"testing"

	pgapply "XCLing/internal/apply"
	"XCLing/internal/model"
	"XCLing/internal/platform"
)

func TestRecordPolicyModeDefaultsToWhitelist(t *testing.T) {
	if recordPolicyMode(model.RecoveryRecord{}) != model.PolicyModeWhitelist {
		t.Fatal("旧记录没有 policyMode 字段，必须按 whitelist 处理")
	}
	if recordPolicyMode(model.RecoveryRecord{PolicyMode: model.PolicyModeBlacklist}) != model.PolicyModeBlacklist {
		t.Fatal("blacklist 记录必须按 blacklist 处理")
	}
}

func TestPlanFromRecoveryRestoresBlockRules(t *testing.T) {
	record := model.RecoveryRecord{
		PolicyName:   "仅拦截策略",
		DefaultLevel: model.SrpLevelUnrestrictedRaw,
		BlockRules: []model.RecoveryRule{
			{ID: "{11111111-2222-3333-4444-555555555555}", Path: "360se.exe", Description: "测试", Level: model.SrpLevelDisallowedRaw},
		},
	}
	plan := planFromRecovery(record)
	if len(plan.BlockRules) != 1 || plan.BlockRules[0].Path != "360se.exe" || plan.BlockRules[0].Level != model.SrpLevelDisallowedRaw {
		t.Fatalf("恢复记录中的拦截规则未还原到计划: %+v", plan.BlockRules)
	}
}

// 仅拦截模式下即使指纹匹配，根形态不归本程序所有也必须判为 attention，
// 防止外部在根键上追加值/子键而不被察觉。
func TestDetectManagedStateBlacklistRequiresOwnedRoot(t *testing.T) {
	record := model.RecoveryRecord{PolicyMode: model.PolicyModeBlacklist}
	expected := pgapply.Plan{
		DefaultLevel:       model.SrpLevelUnrestrictedRaw,
		PolicyScope:        pgapply.PolicyScopeAllUsers,
		TransparentEnabled: pgapply.TransparentEnabledEXE,
		BlockRules:         []pgapply.Rule{pgapply.NewBlockRule("360se.exe", "测试")},
	}
	if got := detectManagedState(record, expected, expected, platform.SRPRootSnapshot{}); got != model.ProtectionStateAttention {
		t.Fatalf("state=%q, want attention when root snapshot is not owned", got)
	}
}
