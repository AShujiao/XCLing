//go:build windows

package platform

import (
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TerminateProcessesMatching 枚举系统进程，结束镜像路径命中 matches 的进程，
// 返回成功结束的数量。用于黑名单新增规则后清理存量进程——SRP 只拦新进程
// 创建，已在运行的托盘/后台进程必须主动结束。
//
// 安全护栏（与拦截规则自身的保护规则叠加，纵深防御）：
//   - 跳过系统关键 PID（0/4）与自身进程
//   - 跳过 C:\Windows 下的进程
//   - 跳过与自身同目录的进程（GUI 壳与 sidecar 同目录部署）
//
// best-effort：无权限或已退出的进程静默跳过，不影响其余。
func TerminateProcessesMatching(matches func(imagePath string) bool) int {
	selfDir := ""
	if exe, err := os.Executable(); err == nil {
		selfDir = strings.ToLower(strings.TrimRight(filepath.Dir(exe), `\`))
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(snapshot)

	selfPID := uint32(os.Getpid())
	killed := 0
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for iterErr := windows.Process32First(snapshot, &entry); iterErr == nil; iterErr = windows.Process32Next(snapshot, &entry) {
		pid := entry.ProcessID
		if pid <= 4 || pid == selfPID {
			continue
		}
		if terminateIfMatches(pid, selfDir, matches) {
			killed++
		}
	}
	return killed
}

func terminateIfMatches(pid uint32, selfDir string, matches func(string) bool) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)

	buf := make([]uint16, 32*1024)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(handle, 0, &buf[0], &size); err != nil || size == 0 {
		return false
	}
	imagePath := windows.UTF16ToString(buf[:size])
	lower := strings.ToLower(imagePath)
	if strings.HasPrefix(lower, `c:\windows\`) {
		return false
	}
	if selfDir != "" {
		dir := strings.ToLower(strings.TrimRight(filepath.Dir(imagePath), `\`))
		if dir == selfDir {
			return false
		}
	}
	if !matches(imagePath) {
		return false
	}
	return windows.TerminateProcess(handle, 1) == nil
}
