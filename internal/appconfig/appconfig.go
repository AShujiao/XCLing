// Package appconfig 提供应用品牌配置（app.config.json）的统一加载逻辑，
// 供 cmd/core sidecar 入口使用（WPF GUI 壳通过 sidecar 间接读取）。
//
// 解析优先级：EXE 同目录 app.config.json > 工作目录 app.config.json > 内嵌默认值。
// sidecar 无法 embed 上级目录文件，传 nil 即回退到 DefaultName；
// WPF GUI 壳自行读取同目录 app.config.json 用于窗口标题等纯展示。
package appconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// DefaultName 是未提供任何配置时的应用名称。
const DefaultName = "星陈守护"

// Config 包含用户可见的应用品牌配置。
type Config struct {
	Name string `json:"name"`
}

// Load 按优先级解析应用配置。embedded 为编译期内嵌的 app.config.json 内容，可为 nil。
func Load(embedded []byte) Config {
	config := Config{Name: DefaultName}
	if len(embedded) > 0 {
		decode(embedded, &config)
	}

	paths := make([]string, 0, 2)
	if executable, err := os.Executable(); err == nil {
		paths = append(paths, filepath.Join(filepath.Dir(executable), "app.config.json"))
	}
	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(cwd, "app.config.json"))
	}
	for _, path := range paths {
		if data, err := os.ReadFile(path); err == nil {
			decode(data, &config)
			break
		}
	}
	config.Name = strings.TrimSpace(config.Name)
	if config.Name == "" {
		config.Name = DefaultName
	}
	return config
}

func decode(data []byte, target *Config) {
	var value Config
	if json.Unmarshal(data, &value) == nil && strings.TrimSpace(value.Name) != "" {
		target.Name = strings.TrimSpace(value.Name)
	}
}
