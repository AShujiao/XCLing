package platform

import (
	"os"
	"strings"
	"testing"
)

// TestPlatformSource_RegistryWritesAreIsolated 静态守护：注册表写 API 只能出现在
// srp_writer_windows.go；其它平台生产源码必须保持只读。
//
// 该测试读取本包自身的 .go 源文件（只读），扫描一批禁用符号。
func TestPlatformSource_NoRegistryWriteAPIs(t *testing.T) {
	const allowedWriter = "srp_writer_windows.go"
	forbidden := []string{
		"registry.WRITE",
		"registry.SET_VALUE",
		"registry.ALL_ACCESS",
		"registry.CREATE_SUB_KEY",
		"registry.CreateKey",
		"registry.DeleteKey",
		".CreateKey(",
		".DeleteKey(",
		".DeleteValue(",
		".SetStringValue(",
		".SetStringsValue(",
		".SetExpandStringValue(",
		".SetDWordValue(",
		".SetQWordValue(",
		".SetBinaryValue(",
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		if name == allowedWriter {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		src := string(data)
		for _, bad := range forbidden {
			if strings.Contains(src, bad) {
				t.Errorf("%s 含禁用的注册表写符号 %q —— 仅 %s 可写 SRP", name, bad, allowedWriter)
			}
		}
	}
}
