package platform

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// 扫描目录树收集可执行文件名的边界：够覆盖常见流氓软件安装目录，又不会在
// 超大目录里失控。命中任一上限即停止。
const (
	dirScanMaxDepth = 5
	dirScanMaxFiles = 5000
	dirScanMaxNames = 100
)

// ListExecutableNames 递归扫描 dir，返回去重后的 .exe 文件基名（保留原始大小写）。
// 用于"拦截目录时一并按文件名拦截其中程序"——文件名规则不限位置，可挡住组件
// 从服务副本 / 看门狗等目录外路径复活。dir 不存在或无权限时返回空，绝不执行任何文件。
func ListExecutableNames(dir string) []string {
	root := strings.TrimSpace(dir)
	if root == "" {
		return nil
	}
	root = strings.TrimRight(root, `\`)
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil
	}
	rootDepth := strings.Count(filepath.Clean(root), string(os.PathSeparator))
	seen := make(map[string]string)
	filesScanned := 0

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // 跳过无权限的子树，继续扫描其余
		}
		if d.IsDir() {
			if strings.Count(filepath.Clean(path), string(os.PathSeparator))-rootDepth > dirScanMaxDepth {
				return filepath.SkipDir
			}
			return nil
		}
		filesScanned++
		if filesScanned > dirScanMaxFiles {
			return filepath.SkipAll
		}
		name := d.Name()
		if !strings.EqualFold(filepath.Ext(name), ".exe") {
			return nil
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; !ok {
			seen[key] = name
			if len(seen) >= dirScanMaxNames {
				return filepath.SkipAll
			}
		}
		return nil
	})

	names := make([]string, 0, len(seen))
	for _, original := range seen {
		names = append(names, original)
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	return names
}
