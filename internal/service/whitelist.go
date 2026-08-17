package service

import (
	"encoding/json"
	"os"
	"strings"

	"XCLing/internal/model"
	"XCLing/internal/platform"
)

// WhitelistService 暴露给前端的「白名单向导」能力：只读发现已安装软件、纯内存生成草案、
// 纯内存预检模拟。
//
// Phase 1a 安全边界（从代码层面约束）：
//   - 本服务**只读/只算**，绝不写入/删除/修改 HKLM SRP 或任何注册表；
//   - 不存在 Apply / Enable / Disable / Restore / Write / Delete / Set / Save / Remove
//     等任何写语义方法（由 TestWhitelistService_NoWriteMethods 反射守护）；
//   - 发现到的 UninstallString / ExecutablePath 只存不执行。
type WhitelistService struct{}

// NewWhitelistService 构造函数。
func NewWhitelistService() *WhitelistService { return &WhitelistService{} }

// ListDiscoveredApps 只读枚举卸载项注册表，返回可安全导出白名单候选的应用列表。
// 非 Windows 平台返回空列表（演示数据由前端 mock 提供）。
func (s *WhitelistService) ListDiscoveredApps() ([]model.DiscoveredApp, error) {
	return platform.DiscoverInstalledApps(), nil
}

// BuildWhitelistDraft 从用户选择生成白名单草案。跨 IPC 传参为 WhitelistSelection 的 JSON 字符串。
// 纯内存计算，不写注册表、不落地为真实 SRP 规则。
func (s *WhitelistService) BuildWhitelistDraft(selectionJSON string) (model.WhitelistDraft, error) {
	var sel model.WhitelistSelection
	if err := json.Unmarshal([]byte(strings.TrimSpace(selectionJSON)), &sel); err != nil {
		return model.WhitelistDraft{}, err
	}
	return BuildDraftFromSelection(sel), nil
}

// PreflightWhitelistDraft 对草案做预检模拟。跨 IPC 传参为 WhitelistDraft 的 JSON 字符串。
// 仅做纯内存推理 + 只读 os.Executable（当前 GUI 主程序路径，用于“核心程序缺口”核对）。
func (s *WhitelistService) PreflightWhitelistDraft(draftJSON string) (model.PreflightReport, error) {
	var draft model.WhitelistDraft
	if err := json.Unmarshal([]byte(strings.TrimSpace(draftJSON)), &draft); err != nil {
		return model.PreflightReport{}, err
	}
	mainExe, _ := os.Executable() // 只读：仅用于核对 GUI 主程序是否被覆盖，绝不修改任何东西
	return PreflightDraft(draft, mainExe), nil
}
