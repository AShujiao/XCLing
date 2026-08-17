//go:build !windows

package platform

import "XCLing/internal/model"

// 非 Windows 平台 stub：不接触任何注册表。真实的“已安装软件发现”只存在于 Windows；
// 此处返回空列表，保证 go build / go test 在 Linux / macOS 上也能编译运行。
// （浏览器降级下的演示数据由前端 mock 提供，并明确标注为演示。）
func DiscoverInstalledApps() []model.DiscoveredApp {
	return []model.DiscoveredApp{}
}
