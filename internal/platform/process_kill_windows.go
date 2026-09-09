//go:build windows

package platform

import (
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// TerminateProcessesMatching 枚举系统进程，结束镜像路径命中 matches 的进程。
// 返回成功结束的数量与「已命中但无法结束」的数量——后者用于提示用户手动退出
// （带自我保护的安全软件会拒绝终止），否则界面会显得"规则没生效"。
//
// 安全护栏（与拦截规则自身的保护规则叠加，纵深防御）：
//   - 跳过系统关键 PID（0/4）与自身进程
//   - 跳过 C:\Windows 下的进程
//   - 跳过与自身同目录的进程（GUI 壳与 sidecar 同目录部署）
//
// best-effort：读不到镜像路径的进程静默跳过，不影响其余。
func TerminateProcessesMatching(matches func(imagePath string) bool) (killed, failed int) {
	selfDir := ""
	if exe, err := os.Executable(); err == nil {
		selfDir = strings.ToLower(strings.TrimRight(filepath.Dir(exe), `\`))
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0, 0
	}
	defer windows.CloseHandle(snapshot)

	selfPID := uint32(os.Getpid())
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for iterErr := windows.Process32First(snapshot, &entry); iterErr == nil; iterErr = windows.Process32Next(snapshot, &entry) {
		pid := entry.ProcessID
		if pid <= 4 || pid == selfPID {
			continue
		}
		matched, done := terminateIfMatches(pid, selfDir, matches)
		switch {
		case matched && done:
			killed++
		case matched:
			failed++
		}
	}
	return killed, failed
}

func terminateIfMatches(pid uint32, selfDir string, matches func(string) bool) (matched, killed bool) {
	query, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false, false
	}
	defer windows.CloseHandle(query)

	buf := make([]uint16, 32*1024)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(query, 0, &buf[0], &size); err != nil || size == 0 {
		return false, false
	}
	imagePath := windows.UTF16ToString(buf[:size])
	lower := strings.ToLower(imagePath)
	if strings.HasPrefix(lower, `c:\windows\`) {
		return false, false
	}
	if selfDir != "" {
		dir := strings.ToLower(strings.TrimRight(filepath.Dir(imagePath), `\`))
		if dir == selfDir {
			return false, false
		}
	}
	if !matches(imagePath) {
		return false, false
	}
	// 命中：单独申请终止权限，失败时如实报告而不是当成没命中。
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return true, false
	}
	defer windows.CloseHandle(handle)
	return true, windows.TerminateProcess(handle, 1) == nil
}
