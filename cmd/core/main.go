// xcling-core 是原生 GUI 壳（WPF）的后端 sidecar：
// 通过 stdio NDJSON JSON-RPC 暴露服务层方法白名单。
//
// 该进程不包含托盘、窗口和文件对话框——这些由 GUI 壳原生实现；
// SRP 写入等安全不变量全部由 internal 包内的既有实现保证，本入口只做接线。
package main

import (
	"fmt"
	"os"

	"XCLing/internal/appconfig"
	"XCLing/internal/model"
	"XCLing/internal/rpc"
	"XCLing/internal/service"
)

func main() {
	if !hasStdioFlag(os.Args[1:]) {
		fmt.Fprintln(os.Stderr, "xcling-core：本程序是 GUI 的后端服务，由 GUI 通过 stdio 启动，请勿直接运行。")
		fmt.Fprintln(os.Stderr, "用法: xcling-core serve --stdio")
		os.Exit(2)
	}

	// 先解析品牌名，策略文案和自保护检查都依赖 model.AppName。
	config := appconfig.Load(nil)
	model.AppName = config.Name

	shutdownService := service.NewShutdownService()
	_ = shutdownService.Start()

	registry := rpc.NewRegistry()
	register(registry, "WhitelistService", service.NewWhitelistService(),
		"ListDiscoveredApps", "BuildWhitelistDraft", "PreflightWhitelistDraft")
	register(registry, "AuditService", service.NewAuditService(),
		"GetAuditCapability", "ListBlockedEvents", "EnableAuditPolicy")
	register(registry, "ApplyService", service.NewApplyService(),
		"GetApplyStatus", "EnableProtection", "EnableBlockOnlyProtection",
		"UnlockProtection", "LockProtection", "AddTrustedPath", "RemoveTrustedRule",
		"RestoreOriginalPolicy", "ListProtectionEvents")
	register(registry, "ShutdownService", shutdownService,
		"GetConfig", "SetConfig", "CancelShutdown")
	register(registry, "BlocklistService", service.NewBlocklistService(),
		"GetBlocklistStatus", "GetVendorPresets",
		"ApplyVendorPreset", "RemoveVendorPreset",
		"AddBlockRule", "RemoveBlockRule",
		"ScanVendorTargets", "ApplyScanResult")
	register(registry, "UpdateService", service.NewUpdateService(), "CheckUpdate")

	server := rpc.NewServer(registry, &rpc.Hello{
		App:      config.Name,
		Version:  model.AppVersion,
		Protocol: 1,
	})
	if err := server.Serve(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "xcling-core serve:", err)
		os.Exit(1)
	}
	shutdownService.Stop()
}

func register(registry *rpc.Registry, name string, instance interface{}, methods ...string) {
	if err := registry.Register(name, instance, methods...); err != nil {
		// 接线错误属于程序缺陷，必须在启动期立刻失败。
		fmt.Fprintln(os.Stderr, "xcling-core:", err)
		os.Exit(1)
	}
}

func hasStdioFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--stdio" {
			return true
		}
	}
	return false
}
