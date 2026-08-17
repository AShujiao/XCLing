//go:build windows

package platform

import (
	"os"
	"sort"

	"XCLing/internal/model"

	"golang.org/x/sys/windows/registry"
)

// uninstallSubPath 是 Windows 记录“已安装软件”的卸载项根路径。
const uninstallSubPath = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`

// uninstallScope 描述一个待枚举的卸载项视图。全部只读打开。
type uninstallScope struct {
	root   registry.Key
	access uint32 // 仅 READ / ENUMERATE_SUB_KEYS / WOW64_* —— 绝无写权限
	source string
}

// DiscoverInstalledApps 只读枚举 HKLM/HKCU 的卸载项注册表（含 Wow6432Node 32 位视图），
// 解析出可安全导出白名单候选的应用并去重。
//
// 安全保证：
//   - 只用 registry.READ | registry.ENUMERATE_SUB_KEYS（+ WOW64 视图选择位）打开键，
//     绝不请求 SET_VALUE / WRITE / CREATE_SUB_KEY 等写权限；
//   - 只读取值，从不执行 UninstallString / DisplayIcon 指向的任何程序；
//   - 仅通过 os.Stat 做只读存在性检查（用于置信度评估），不打开、不运行文件。
func DiscoverInstalledApps() []model.DiscoveredApp {
	scopes := []uninstallScope{
		{registry.LOCAL_MACHINE, registry.READ | registry.ENUMERATE_SUB_KEYS | registry.WOW64_64KEY, model.SourceHKLM64},
		{registry.LOCAL_MACHINE, registry.READ | registry.ENUMERATE_SUB_KEYS | registry.WOW64_32KEY, model.SourceHKLM32},
		{registry.CURRENT_USER, registry.READ | registry.ENUMERATE_SUB_KEYS, model.SourceHKCU},
	}

	pathExists := func(p string) bool {
		if p == "" {
			return false
		}
		_, err := os.Stat(p)
		return err == nil
	}

	var all []model.DiscoveredApp
	for _, sc := range scopes {
		for _, raw := range readUninstallScope(sc) {
			if app, ok := BuildDiscoveredApp(raw, pathExists, os.Getenv); ok {
				all = append(all, app)
			}
		}
	}

	deduped := DedupeApps(all)
	sort.SliceStable(deduped, func(i, j int) bool {
		return deduped[i].DisplayName < deduped[j].DisplayName
	})
	return deduped
}

// readUninstallScope 只读枚举单个视图下的全部卸载项子键，返回原始值。
func readUninstallScope(sc uninstallScope) []RawUninstallEntry {
	root, err := registry.OpenKey(sc.root, uninstallSubPath, sc.access)
	if err != nil {
		return nil // 视图不存在（如 32 位系统上的 WOW64_32KEY 冗余）→ 跳过
	}
	defer root.Close()

	names, err := root.ReadSubKeyNames(-1)
	if err != nil {
		return nil
	}

	out := make([]RawUninstallEntry, 0, len(names))
	for _, name := range names {
		sub, err := registry.OpenKey(sc.root, uninstallSubPath+`\`+name, sc.access)
		if err != nil {
			continue
		}
		entry := RawUninstallEntry{KeyName: name, Source: sc.source}
		entry.DisplayName, _, _ = sub.GetStringValue("DisplayName")
		entry.DisplayVersion, _, _ = sub.GetStringValue("DisplayVersion")
		entry.Publisher, _, _ = sub.GetStringValue("Publisher")
		entry.InstallLocation, _, _ = sub.GetStringValue("InstallLocation")
		entry.DisplayIcon, _, _ = sub.GetStringValue("DisplayIcon")
		entry.UninstallString, _, _ = sub.GetStringValue("UninstallString")
		if v, _, e := sub.GetIntegerValue("SystemComponent"); e == nil {
			entry.SystemComponent = int(v)
		}
		sub.Close()
		out = append(out, entry)
	}
	return out
}
