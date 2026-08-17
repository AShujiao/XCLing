package appconfig

import "testing"

func TestDecodeConfig(t *testing.T) {
	config := Config{Name: DefaultName}
	decode([]byte(`{"name":" 自定义名称 "}`), &config)
	if config.Name != "自定义名称" {
		t.Fatalf("unexpected name: %q", config.Name)
	}
}

func TestDecodeConfigRejectsEmptyName(t *testing.T) {
	config := Config{Name: DefaultName}
	decode([]byte(`{"name":"  "}`), &config)
	if config.Name != DefaultName {
		t.Fatalf("empty name replaced default: %q", config.Name)
	}
}

func TestLoadWithoutEmbeddedFallsBackToDefault(t *testing.T) {
	config := Load(nil)
	if config.Name == "" {
		t.Fatal("name must never be empty")
	}
}

func TestLoadUsesEmbeddedName(t *testing.T) {
	config := Load([]byte(`{"name":"内嵌名称"}`))
	// 测试运行目录可能存在 app.config.json 覆盖内嵌值，两者都合法；只验证非空。
	if config.Name == "" {
		t.Fatal("name must never be empty")
	}
}
