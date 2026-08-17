// Command checksum 为发布产物生成确定性的 SHA256SUMS.txt。
//
// 用法：checksum -out <SHA256SUMS.txt> <file1> [file2 ...]
//
// 安全边界：只读输入文件、只写指定的校验和文件；不触碰注册表、不改系统策略、不联网。
// 供 scripts/build-release.sh 调用，避免依赖平台是否安装 GNU sha256sum。
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"XCLing/internal/release"
)

func main() {
	out := flag.String("out", "SHA256SUMS.txt", "输出的校验和文件路径")
	flag.Parse()

	inputs := flag.Args()
	if len(inputs) == 0 {
		fmt.Fprintln(os.Stderr, "checksum: 未提供任何输入文件")
		fmt.Fprintln(os.Stderr, "用法: checksum -out <SHA256SUMS.txt> <file1> [file2 ...]")
		os.Exit(2)
	}

	files := make(map[string][]byte, len(inputs))
	for _, p := range inputs {
		data, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "checksum: 读取 %s 失败: %v\n", p, err)
			os.Exit(1)
		}
		// 以文件名（不含目录）作为记录名，便于在产物目录内直接校验。
		files[filepath.Base(p)] = data
	}

	content := release.BuildSHA256SUMS(files)
	if err := os.WriteFile(*out, []byte(content), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "checksum: 写入 %s 失败: %v\n", *out, err)
		os.Exit(1)
	}
	fmt.Print(content)
	fmt.Fprintf(os.Stderr, "checksum: 已写入 %s（%d 个文件）\n", *out, len(files))
}
