package apply

import (
	"path/filepath"
	"strings"
)

// 本文件提供拦截模式与进程镜像路径的匹配，用于"加入黑名单后主动结束存量进程"：
// SRP 只拦新进程创建，对已在运行的托盘/后台进程无效，不清理会出现
// "主界面被拦但托盘图标还在"的观感。匹配语义与 SRP 路径规则对齐。

// MatchesAnyBlockPattern 判断进程镜像路径是否命中任一拦截模式。
// 裸文件名模式匹配任意目录下的同名文件；`X\*` 目录模式匹配该目录及子目录；
// 其余为精确文件路径匹配。
func MatchesAnyBlockPattern(imagePath string, patterns []string) bool {
	path := canonicalPath(imagePath)
	if path == "" {
		return false
	}
	base := strings.ToLower(filepath.Base(path))
	for _, pattern := range patterns {
		p := strings.TrimSpace(pattern)
		if p == "" {
			continue
		}
		if !strings.ContainsAny(p, `\/`) {
			if base == strings.ToLower(p) {
				return true
			}
			continue
		}
		if pathMatches(p, path) {
			return true
		}
	}
	return false
}
