package model

// SRP 注册表根路径。位于 HKLM\SOFTWARE\Policies 下。
//
// 注意：该路径本身并不能证明策略由域/企业 GPO 下发。本地组策略编辑器
// （gpedit.msc / secpol.msc）在单机上配置的 SRP 也写入同一 Policies 配置单元。
// 因此“路径在 Policies 下”只能说明它是策略型配置，无法据此判定管理来源。
const SRPRegistryPath = `SOFTWARE\Policies\Microsoft\Windows\Safer\CodeIdentifiers`

// 策略管理来源。Phase 0 只做只读探测，无法区分本地策略与域/GPO 下发，
// 因此统一记为 unknown；ApplyService 另行拒绝域成员和任何既有 SRP 根键。
const (
	ManagementSourceUnknown     = "unknown"     // 无法判定：可能本地策略，也可能域/GPO 下发
	ManagementSourceUnsupported = "unsupported" // 非 Windows 平台，无 SRP 概念
)

// SRP DefaultLevel DWORD 取值（Windows 官方定义）。
const (
	SrpLevelDisallowedRaw   = 0      // 0x00000000 Disallowed
	SrpLevelBasicUserRaw    = 131072 // 0x00020000 Basic User
	SrpLevelUnrestrictedRaw = 262144 // 0x00040000 Unrestricted
)

// SrpState 当前机器 SRP 注册表的只读快照。
// 仅通过 golang.org/x/sys/windows/registry 读取，绝不写入。
type SrpState struct {
	Supported          bool              `json:"supported"`          // 当前平台是否支持读取（仅 Windows）
	Available          bool              `json:"available"`          // SRP 注册表键是否存在
	Configured         bool              `json:"configured"`         // 是否已配置（存在有效值/规则）
	Enforced           bool              `json:"enforced"`           // TransparentEnabled 是否启用强制
	RegistryPath       string            `json:"registryPath"`       // 读取的注册表路径
	DefaultLevel       string            `json:"defaultLevel"`       // 解码后的默认级别
	DefaultLevelRaw    int               `json:"defaultLevelRaw"`    // 原始 DWORD
	TransparentEnabled int               `json:"transparentEnabled"` // 0/1/2
	PolicyScope        string            `json:"policyScope"`        // allUsers | exceptAdmins | unknown
	PolicyScopeRaw     int               `json:"policyScopeRaw"`     // 原始 DWORD
	ExecutableTypes    []string          `json:"executableTypes"`    // 受管可执行扩展名列表
	PathRuleCount      int               `json:"pathRuleCount"`      // 路径规则数量
	HashRuleCount      int               `json:"hashRuleCount"`      // 哈希规则数量
	ManagementSource   string            `json:"managementSource"`   // 管理来源：unknown（无法判定本地/域）| unsupported
	ReadOnly           bool              `json:"readOnly"`           // 是否只读（Phase 0 恒为 true）
	Warnings           []ConflictWarning `json:"warnings"`           // 只读/冲突提示
	Message            string            `json:"message"`            // 概要说明（中文）
	ReadAt             string            `json:"readAt"`             // 读取时间，RFC3339
}
