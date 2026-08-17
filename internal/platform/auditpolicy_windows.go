//go:build windows

package platform

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const auditPolTool = "auditpol"
const auditPolicySubcategory = "Process Creation"

// AuditPolicyStatus describes the current state of SRP event logging.
type AuditPolicyStatus struct {
	Available   bool   // configuration capability available
	Enabled     bool   // SRP event logging enabled
	Detail      string // Human readable detail
	ErrorDetail string // Error if any
}

// GetAuditPolicyStatus checks whether SRP event logging is enabled.
// SRP writes events to the Application log only if the LogEvent registry value is set to 1.
func GetAuditPolicyStatus() AuditPolicyStatus {
	status := AuditPolicyStatus{Available: true}

	// Check SRP LogEvent registry value
	// HKLM\SOFTWARE\Policies\Microsoft\Windows\Safer\CodeIdentifiers
	// LogEvent = 1 enables logging (0 disables, default is not present = logging disabled)
	k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Policies\Microsoft\Windows\Safer\CodeIdentifiers`,
		registry.QUERY_VALUE)
	if err != nil {
		status.Enabled = false
		status.Detail = "SRP事件日志未启用，拦截事件不会被记录"
		return status
	}
	defer k.Close()

	val, _, err := k.GetIntegerValue("LogEvent")
	if err != nil {
		status.Enabled = false
		status.Detail = "SRP事件日志未启用，拦截事件不会被记录"
		return status
	}

	status.Enabled = val >= 1
	if status.Enabled {
		status.Detail = "SRP事件日志已启用，拦截事件会记录到Application日志"
	} else {
		status.Detail = "SRP事件日志已禁用，拦截事件不会被记录"
	}
	return status
}

// EnableAuditPolicy enables SRP interception event logging by setting the
// LogEvent registry value to 1. Requires Administrator privileges.
func EnableAuditPolicy() error {
	// Open or create the registry key
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE,
		`SOFTWARE\Policies\Microsoft\Windows\Safer\CodeIdentifiers`,
		registry.SET_VALUE)
	if err != nil {
		return errors.New("打开注册表项失败（需要管理员权限）：" + err.Error())
	}
	defer k.Close()

	// Set LogEvent = 1 to enable SRP event logging (Event ID 865/866 in Application log)
	if err := k.SetDWordValue("LogEvent", 1); err != nil {
		return errors.New("设置注册表值失败：" + err.Error())
	}

	// Also ensure Application event log is enabled (best effort)
	if wevtutilPath, err := exec.LookPath(EventLogTool); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, wevtutilPath, "sl", "Application", "/e:true")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
		_ = cmd.Run()
	}

	// Also try enabling Process Creation auditing via auditpol (best effort, for additional logging)
	if auditPolPath, err := exec.LookPath(auditPolTool); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, auditPolPath,
			"/set",
			"/subcategory:"+auditPolicySubcategory,
			"/success:enable",
		)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
		var out bytes.Buffer
		cmd.Stdout = &out
		var errBuf bytes.Buffer
		cmd.Stderr = &errBuf
		_ = cmd.Run()
	}

	return nil
}
