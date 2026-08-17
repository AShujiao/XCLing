//go:build windows

package platform

import (
	"errors"
	"os"
	"strconv"
	"time"

	"XCLing/internal/model"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// srpLevelSubkeys 是 CodeIdentifiers 下按安全级别命名的子键。
// 每个级别下各有 Paths / Hashes 两个子键，规则以 GUID 子键形式存在。
var srpLevelSubkeys = []string{
	strconv.Itoa(model.SrpLevelDisallowedRaw),
	strconv.Itoa(model.SrpLevelBasicUserRaw),
	strconv.Itoa(model.SrpLevelUnrestrictedRaw),
}

// IsAdmin reports whether the current process can act as an administrator.
// TokenElevation is authoritative on modern Windows. Windows 7 can report a
// false negative for some UAC token configurations, so fall back to checking
// the enabled built-in Administrators group in the same process token.
func IsAdmin() bool {
	if assumeAdmin {
		return true
	}
	token := windows.GetCurrentProcessToken()
	if token.IsElevated() {
		return true
	}
	administrators, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return false
	}
	member, err := token.IsMember(administrators)
	return err == nil && member
}

// OSVersion 尽力而为地返回 Windows 版本串，如 "10.0.26200"。
func OSVersion() string {
	v := windows.RtlGetVersion()
	if v == nil {
		return ""
	}
	return strconv.FormatUint(uint64(v.MajorVersion), 10) + "." +
		strconv.FormatUint(uint64(v.MinorVersion), 10) + "." +
		strconv.FormatUint(uint64(v.BuildNumber), 10)
}

// CanReadSRP 仅尝试以只读方式打开 SRP 键，判断可访问性。
// 键不存在（未配置 SRP）视为“可访问但为空”，返回 true。
func CanReadSRP() bool {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, model.SRPRegistryPath, registry.READ)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
			return true // 键缺失说明本机未配置 SRP，读取路径本身是可达的
		}
		return false
	}
	_ = key.Close()
	return true
}

// ReadSrpState 只读读取 HKLM\SOFTWARE\Policies\Microsoft\Windows\Safer\CodeIdentifiers
// 的当前状态。全程只用 registry.READ 权限打开，绝不写入。
func ReadSrpState() model.SrpState {
	state := model.SrpState{
		Supported:    true,
		RegistryPath: `HKLM\` + model.SRPRegistryPath,
		DefaultLevel: "unknown",
		ReadAt:       time.Now().Format(time.RFC3339),
		// 该键位于 HKLM\SOFTWARE\Policies 配置单元，但路径本身无法证明来源：
		// 本地组策略与域/GPO 下发都写在这里。Phase 0 无法判定，保守记为 unknown 并只读。
		ManagementSource: model.ManagementSourceUnknown,
		ReadOnly:         true,
	}

	root, err := registry.OpenKey(registry.LOCAL_MACHINE, model.SRPRegistryPath,
		registry.READ|registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
			state.Available = false
			state.Message = "本机未配置软件限制策略（SRP）。CodeIdentifiers 注册表键不存在。"
			state.Warnings = append(state.Warnings, AdminBypassWarning())
			return state
		}
		state.Available = false
		state.Message = "无法读取 SRP 注册表键：" + err.Error()
		state.Warnings = append(state.Warnings, model.ConflictWarning{
			Level:   model.WarnDanger,
			Code:    "SRP_READ_FAILED",
			Message: "读取 SRP 注册表失败（可能权限不足）：" + err.Error(),
		})
		return state
	}
	defer root.Close()

	state.Available = true

	if raw, _, e := root.GetIntegerValue("DefaultLevel"); e == nil {
		state.DefaultLevelRaw = int(raw)
		state.DefaultLevel = DecodeDefaultLevel(int(raw))
		state.Configured = true
	}
	if raw, _, e := root.GetIntegerValue("TransparentEnabled"); e == nil {
		state.TransparentEnabled = int(raw)
		state.Enforced = raw != 0
		state.Configured = true
	}
	if raw, _, e := root.GetIntegerValue("PolicyScope"); e == nil {
		state.PolicyScopeRaw = int(raw)
		state.PolicyScope = DecodePolicyScope(int(raw))
	} else {
		state.PolicyScope = "unknown"
	}
	if types, _, e := root.GetStringsValue("ExecutableTypes"); e == nil {
		state.ExecutableTypes = types
	}

	state.PathRuleCount, state.HashRuleCount = countRules()

	if state.PathRuleCount > 0 || state.HashRuleCount > 0 {
		state.Configured = true
	}

	if state.Configured {
		state.Message = "检测到已配置的 SRP 策略（只读）。"
	} else {
		state.Message = "SRP 注册表键存在但未配置有效规则/默认级别。"
	}

	state.Warnings = append(state.Warnings, AdminBypassWarning(), ManagementSourceUnknownWarning())
	return state
}

// countRules 只读遍历各级别下的 Paths / Hashes 子键，统计规则数量。
func countRules() (paths int, hashes int) {
	for _, level := range srpLevelSubkeys {
		paths += countSubKeys(model.SRPRegistryPath + `\` + level + `\Paths`)
		hashes += countSubKeys(model.SRPRegistryPath + `\` + level + `\Hashes`)
	}
	return paths, hashes
}

// countSubKeys 只读打开一个键并返回其子键数量，键不存在返回 0。
func countSubKeys(path string) int {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.READ|registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return 0
	}
	defer key.Close()
	names, err := key.ReadSubKeyNames(-1)
	if err != nil {
		return 0
	}
	return len(names)
}
