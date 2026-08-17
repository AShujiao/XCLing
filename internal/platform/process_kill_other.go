//go:build !windows

package platform

// TerminateProcessesMatching 非 Windows 平台无 SRP，恒为空操作。
func TerminateProcessesMatching(matches func(imagePath string) bool) int {
	return 0
}
