// Package platform 封装与操作系统强相关的探测能力，以及隔离的实验性 SRP Writer。
//
// 安全边界：注册表写 API 只能出现在 srp_writer_windows.go，且只接受已验证的 SRP Plan。
// 其它文件保持只读；非 Windows 提供明确的不可用 stub。
package platform

import "XCLing/internal/model"

// DecodeDefaultLevel 将 SRP DefaultLevel DWORD 解码为可读级别。纯函数，跨平台可测。
func DecodeDefaultLevel(raw int) string {
	switch raw {
	case model.SrpLevelDisallowedRaw:
		return "disallowed"
	case model.SrpLevelBasicUserRaw:
		return "basicUser"
	case model.SrpLevelUnrestrictedRaw:
		return "unrestricted"
	default:
		return "unknown"
	}
}

// DecodePolicyScope 将 SRP PolicyScope DWORD 解码。纯函数，跨平台可测。
//
//	0 = 所有用户
//	1 = 除本地管理员外的所有用户（管理员可绕过）
func DecodePolicyScope(raw int) string {
	switch raw {
	case 0:
		return "allUsers"
	case 1:
		return "exceptAdmins"
	default:
		return "unknown"
	}
}

// AdminBypassWarning 返回“管理员可绕过本机策略”的固定提示。
// 这是 SRP 的固有属性，必须始终对用户明示。
func AdminBypassWarning() model.ConflictWarning {
	return model.ConflictWarning{
		Level:   model.WarnWarning,
		Code:    "ADMIN_CAN_BYPASS",
		Message: "本机 SRP 策略可被本地管理员绕过；SRP 不是防提权/防恶意管理员的安全边界。",
	}
}

// ManagementSourceUnknownWarning 返回“注册表路径本身不能证明管理来源”的固定提示。
//
// SRP 位于 HKLM\SOFTWARE\Policies 配置单元，但该路径本身并不能证明策略由域/GPO 下发——
// 本地组策略配置的 SRP 也写在同一位置。ApplyService 额外拒绝域成员和任何既有根键。
func ManagementSourceUnknownWarning() model.ConflictWarning {
	return model.ConflictWarning{
		Level: model.WarnWarning,
		Code:  "MANAGEMENT_SOURCE_UNKNOWN",
		Message: "SRP 策略位于 HKLM\\SOFTWARE\\Policies 配置单元，可能来自本地策略或域 GPO，" +
			"仅凭该路径无法判定来源；本状态探测保持只读。" + model.AppName + " 仅在未加入域且 " +
			"CodeIdentifiers 根键不存在或精确匹配可备份的空旧配置时允许实验性写入。",
	}
}
