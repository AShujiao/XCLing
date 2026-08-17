//go:build !windows

package platform

import (
	"runtime"
	"time"

	"XCLing/internal/model"
)

// 非 Windows 平台 stub：不接触任何注册表，返回“不支持”的只读状态，
// 使 go build / go test 在 Linux / macOS 上也能编译运行。

// IsAdmin 在非 Windows 平台恒为 false（本工具仅在 Windows 管理 SRP）。
func IsAdmin() bool { return false }

// OSVersion 返回 GOOS 作为占位版本信息。
func OSVersion() string { return runtime.GOOS }

// CanReadSRP 非 Windows 恒为 false：无 SRP 概念。
func CanReadSRP() bool { return false }

// ReadSrpState 返回“当前平台不支持”的只读状态。
func ReadSrpState() model.SrpState {
	return model.SrpState{
		Supported:        false,
		Available:        false,
		Configured:       false,
		RegistryPath:     `HKLM\` + model.SRPRegistryPath,
		DefaultLevel:     "unsupported",
		PolicyScope:      "unknown",
		ManagementSource: model.ManagementSourceUnsupported,
		ReadOnly:         true,
		ReadAt:           time.Now().Format(time.RFC3339),
		Message:          "当前操作系统（" + runtime.GOOS + "）不是 Windows，无法读取软件限制策略（SRP）。",
		Warnings: []model.ConflictWarning{{
			Level:   model.WarnInfo,
			Code:    "OS_UNSUPPORTED",
			Message: "SRP 仅存在于 Windows；本平台下所有 SRP 读取均为不可用状态。",
		}},
	}
}
