// Package release 提供发布产物的**纯函数**校验和工具。
//
// 设计要点：
//   - 纯计算，无网络、无注册表、无系统策略写；只对内存字节做 SHA256；
//   - BuildSHA256SUMS 产出确定性（按文件名排序）的 SHA256SUMS.txt 内容，
//     与 GNU coreutils `sha256sum` 输出格式一致（"<hex>  <name>"），便于跨平台校验；
//   - 供 cmd/checksum 在发布脚本里调用，避免依赖平台是否安装 sha256sum。
package release

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// SHA256Hex 返回字节内容的十六进制 SHA256。
func SHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// ChecksumLine 返回一行 sha256sum 风格记录："<hex>  <name>"（两个空格分隔）。
func ChecksumLine(name, hexsum string) string {
	return hexsum + "  " + name
}

// BuildSHA256SUMS 由 name->content 映射产出确定性的 SHA256SUMS.txt 文本
//（按文件名升序，末尾带换行）。
func BuildSHA256SUMS(files map[string][]byte) string {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, n := range names {
		b.WriteString(ChecksumLine(n, SHA256Hex(files[n])))
		b.WriteByte('\n')
	}
	return b.String()
}
