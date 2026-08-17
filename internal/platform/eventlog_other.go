//go:build !windows

package platform

// 非 Windows 平台 stub：无 Windows 事件日志。只读能力报告为不可用，查询返回空。
// 使 go build / go test 在 Linux / macOS 上也能编译运行。

// EventLogAvailable 非 Windows 恒不可用。
func EventLogAvailable() EventLogCapability {
	return EventLogCapability{
		Available: false,
		Tool:      EventLogTool,
		Reason:    "当前平台非 Windows：无 Windows 事件日志，审计能力不可用。",
	}
}

// QueryEventLog 非 Windows 返回空输出（不报错，便于服务层统一处理）。绝不执行任何命令。
func QueryEventLog(_ EventQuery) (string, error) {
	return "", nil
}
