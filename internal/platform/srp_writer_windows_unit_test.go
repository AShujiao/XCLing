//go:build windows

package platform

import (
	"testing"

	pgapply "XCLing/internal/apply"
)

// TestValidateRuleIDAcceptsGeneratedRuleIDs 回归守护：validateRuleID 必须接受
// apply 包实际生成的带花括号 GUID 规则 ID。此前校验正则漏掉了花括号，导致
// WriteSRPPlanFrom 写第一条规则前就失败，“启用保护”必然回滚并报
// WRITE_FAILED_ROLLED_BACK（写入失败，已恢复到原状态）。
func TestValidateRuleIDAcceptsGeneratedRuleIDs(t *testing.T) {
	plan, rule, err := pgapply.AddTrustedRule(pgapply.Plan{}, `C:\Games\Foo`, true, "回归测试")
	if err != nil {
		t.Fatal(err)
	}
	if err := validateRuleID(rule.ID); err != nil {
		t.Fatalf("生成的规则 ID %q 未通过写入校验: %v", rule.ID, err)
	}
	for _, planRule := range plan.Rules {
		if err := validateRuleID(planRule.ID); err != nil {
			t.Fatalf("计划内规则 ID %q 未通过写入校验: %v", planRule.ID, err)
		}
	}
	fixtures := []string{
		"{8d3e661d-58d3-4a2d-b3ef-a5960851f63b}",
		"{2F141F0C-A3C4-4456-871F-31C2456D21E0}",
	}
	for _, id := range fixtures {
		if err := validateRuleID(id); err != nil {
			t.Errorf("validateRuleID(%q) = %v, want nil", id, err)
		}
	}
}

func TestValidateRuleIDRejectsUnsafeIDs(t *testing.T) {
	invalid := []string{
		"",
		"8d3e661d-58d3-4a2d-b3ef-a5960851f63b",  // 缺花括号
		"{8d3e661d-58d3-4a2d-b3ef-a5960851f63b", // 括号不闭合
		`{..\..\otherkey}`,                      // 路径穿越
		`rules\escape`,                          // 注册表路径分隔符
		"plain-name",
	}
	for _, id := range invalid {
		if err := validateRuleID(id); err == nil {
			t.Errorf("validateRuleID(%q) = nil, want error", id)
		}
	}
}
