package platform

import (
	"runtime"
	"strings"
	"testing"

	"XCLing/internal/model"
)

// 说明：这些测试只调用只读路径（ReadSrpState / CanReadSRP / 解码函数），
// 绝不写 HKLM。ReadSrpState 内部仅以 registry.READ 打开键。

func TestDecodeDefaultLevel(t *testing.T) {
	cases := map[int]string{
		model.SrpLevelDisallowedRaw:   "disallowed",
		model.SrpLevelBasicUserRaw:    "basicUser",
		model.SrpLevelUnrestrictedRaw: "unrestricted",
		999999:                        "unknown",
	}
	for raw, want := range cases {
		if got := DecodeDefaultLevel(raw); got != want {
			t.Errorf("DecodeDefaultLevel(%d)=%s want %s", raw, got, want)
		}
	}
}

func TestDecodePolicyScope(t *testing.T) {
	if DecodePolicyScope(0) != "allUsers" {
		t.Error("0 should be allUsers")
	}
	if DecodePolicyScope(1) != "exceptAdmins" {
		t.Error("1 should be exceptAdmins")
	}
	if DecodePolicyScope(7) != "unknown" {
		t.Error("unexpected value should be unknown")
	}
}

func TestReadSrpState_NonDestructive(t *testing.T) {
	// 多次读取应稳定、不 panic，且始终标记只读。
	for i := 0; i < 3; i++ {
		st := ReadSrpState()
		if !st.ReadOnly {
			t.Fatal("SRP 状态必须标记 ReadOnly=true")
		}
		if !strings.Contains(st.RegistryPath, model.SRPRegistryPath) {
			t.Fatalf("RegistryPath 应包含目标路径，got %s", st.RegistryPath)
		}
		if st.ReadAt == "" {
			t.Fatal("ReadAt 不应为空")
		}
		if runtime.GOOS == "windows" {
			if !st.Supported {
				t.Fatal("Windows 平台应标记 Supported=true")
			}
		} else {
			if st.Supported {
				t.Fatal("非 Windows 平台应标记 Supported=false")
			}
		}
	}
}

// TestReadSrpState_ManagementSourceUnknown 守护保守语义：Phase 0 无法判定 SRP 的
// 管理来源（本地策略 vs 域/GPO），因此 Windows 上必须记为 unknown、非 Windows 记为
// unsupported，且永远不能出现声称“确定由 GPO 托管”的取值。
func TestReadSrpState_ManagementSourceUnknown(t *testing.T) {
	st := ReadSrpState()
	if !st.ReadOnly {
		t.Fatal("无论管理来源如何，Phase 0 都必须只读")
	}
	want := model.ManagementSourceUnknown
	if runtime.GOOS != "windows" {
		want = model.ManagementSourceUnsupported
	}
	if st.ManagementSource != want {
		t.Fatalf("ManagementSource=%q，want %q（不得断言确定的 GPO 托管）", st.ManagementSource, want)
	}
	// 绝不能出现声称“确定由 GPO 管理”的取值。
	for _, forbidden := range []string{"gpo", "managedByGpo", "domain", "enterprise"} {
		if st.ManagementSource == forbidden {
			t.Fatalf("ManagementSource 不得断言确定来源，got %q", st.ManagementSource)
		}
	}
}

func TestWarningsAlwaysPresent(t *testing.T) {
	if AdminBypassWarning().Code != "ADMIN_CAN_BYPASS" {
		t.Error("admin bypass warning code mismatch")
	}
	if ManagementSourceUnknownWarning().Code != "MANAGEMENT_SOURCE_UNKNOWN" {
		t.Error("management source warning code mismatch")
	}
}

// TestManagementSourceUnknownWarning 校验告警措辞不把 Policies 注册表路径当作
// “由 GPO 托管”的证明，而是表达“可能本地或域、无法判定、保守只读”。
func TestManagementSourceUnknownWarning(t *testing.T) {
	w := ManagementSourceUnknownWarning()
	if w.Code != "MANAGEMENT_SOURCE_UNKNOWN" {
		t.Fatalf("warning code=%q", w.Code)
	}
	for _, must := range []string{"可能", "无法判定", "只读"} {
		if !strings.Contains(w.Message, must) {
			t.Errorf("告警措辞应包含 %q，实际：%s", must, w.Message)
		}
	}
}

func TestCanReadSRP_DoesNotPanic(t *testing.T) {
	_ = CanReadSRP() // 仅确认只读探测不 panic
}
