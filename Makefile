# XCLing 构建目标
#
# 以下目标只执行测试和构建，不运行应用，因此不会触发显式 ApplyService。

.PHONY: all test vet build clean verify wpf wpf-win10 wpf-win7 installer installer-win10

## verify: 后端单测 + 静态检查 + 构建（不写入任何 SRP）
verify: test vet build

## test: 运行全部 Go 单测（含安全回归；不触碰真实 HKLM 写入）
test:
	go test -count=1 ./...
	# Win7 兼容变体守护：验证 assumeAdmin=true 变体可编译、IsAdmin 恒真，
	# 防止 build-wpf.ps1 丢失 -tags win7 或 admin_assumption 再次被改坏而无人察觉。
	# 只跑这一个测试，避免触发 platform 包里与本次无关的既有只读守卫测试。
	go test -tags win7 -count=1 -run TestWin7CompatibilityDoesNotGateOnAdminToken ./internal/platform/

## vet: 静态检查
vet:
	go vet ./...

## build: 全量编译（cmd/core sidecar + cmd/checksum + internal/*）
build:
	go build ./...

## wpf: 构建 WPF 原生壳 + sidecar 部署包（Win10/11 + Win7 SP1）
##      需要 .NET SDK 8+ 和 Go 1.20.14（Win7 sidecar），详见 scripts/build-wpf.ps1
wpf:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build-wpf.ps1

## wpf-win10: 只构建 Win10/11 部署包（跳过 Win7 sidecar）
wpf-win10:
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build-wpf.ps1 -SkipWin7

## wpf-win7: 别名，等价于 wpf（Win7 sidecar 依赖 Go 1.20.14 工具链）
wpf-win7: wpf

## installer: 构建全版本安装包（自动兼容 Win7/Win8/Win10/Win11）
##           需要 Inno Setup 6 或 7，会自动调用 wpf 目标先编译程序
installer: wpf
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build-installer.ps1 -SkipBuild

## installer-win10: 构建仅支持 Win10/11 的安装包（跳过 Win7 sidecar，体积更小）
installer-win10: wpf-win10
	powershell -NoProfile -ExecutionPolicy Bypass -File scripts/build-installer.ps1 -SkipWin7 -SkipBuild

## clean: 清理所有构建产物
clean:
	rm -rf build/bin/wpf-win10 build/bin/wpf-win7 build/installer
	rm -f build/bin/xcling-core-win10.exe build/bin/xcling-core-win7.exe
