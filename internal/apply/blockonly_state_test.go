package apply

import (
	"testing"

	"XCLing/internal/model"
)

func blockOnlyPlan() Plan {
	return Plan{
		PolicyName:         "仅拦截策略",
		DefaultLevel:       model.SrpLevelUnrestrictedRaw,
		PolicyScope:        PolicyScopeAllUsers,
		TransparentEnabled: TransparentEnabledEXE,
		BlockRules:         []Rule{NewBlockRule("360se.exe", "测试规则")},
	}
}

// 仅拦截模式：注册表与期望计划完全一致即为锁定（拦截生效中）。
func TestDetectBlockOnlyProtectionStateLocked(t *testing.T) {
	expected := blockOnlyPlan()
	if got := DetectBlockOnlyProtectionState(expected, expected); got != model.ProtectionStateLocked {
		t.Fatalf("state=%q, want locked", got)
	}
}

// 临时解锁 = 注册表等于期望计划去掉全部拦截规则（规则保留在恢复记录里）。
func TestDetectBlockOnlyProtectionStateUnlocked(t *testing.T) {
	expected := blockOnlyPlan()
	actual := expected
	actual.BlockRules = nil
	if got := DetectBlockOnlyProtectionState(actual, expected); got != model.ProtectionStateUnlocked {
		t.Fatalf("state=%q, want unlocked", got)
	}
}

func TestDetectBlockOnlyProtectionStateAttention(t *testing.T) {
	expected := blockOnlyPlan()
	// DefaultLevel 被外部改成 Disallowed（等同整机锁死）必须判为外部修改。
	tampered := expected
	tampered.DefaultLevel = DefaultLevelDisallowed
	if got := DetectBlockOnlyProtectionState(tampered, expected); got != model.ProtectionStateAttention {
		t.Fatalf("state=%q, want attention for tampered DefaultLevel", got)
	}
	// 出现名单之外的拦截规则同样是外部修改。
	foreign := expected
	foreign.BlockRules = append(append([]Rule(nil), expected.BlockRules...), NewBlockRule("foreign.exe", "外部添加"))
	if got := DetectBlockOnlyProtectionState(foreign, expected); got != model.ProtectionStateAttention {
		t.Fatalf("state=%q, want attention for foreign block rule", got)
	}
}

// 刚启用、还没有任何拦截规则时，锁定与解锁指纹相同，按锁定处理。
func TestDetectBlockOnlyProtectionStateEmptyPlanIsLocked(t *testing.T) {
	empty := blockOnlyPlan()
	empty.BlockRules = nil
	if got := DetectBlockOnlyProtectionState(empty, empty); got != model.ProtectionStateLocked {
		t.Fatalf("state=%q, want locked for empty block-only plan", got)
	}
}
