package platform

import (
	"os"
	"strings"
	"testing"
)

// TestBuildWevtutilArgs_ReadOnlyAndNoUserInput 验证事件日志查询参数：
//   - 首个参数恒为只读子命令 "qe"；
//   - 使用渲染格式与整数上限；
//   - 通道/Provider 为传入的白名单常量；
//   - 绝不包含任何关键词字段（EventQuery 无 keyword，此处双重确认）。
func TestBuildWevtutilArgs_ReadOnlyAndNoUserInput(t *testing.T) {
	q := EventQuery{
		Channel:      "Application",
		ProviderName: "Microsoft-Windows-SoftwareRestrictionPolicies",
		MaxRecords:   50,
		WithinMillis: 86_400_000,
	}
	args := buildWevtutilArgs(q)
	if len(args) == 0 || args[0] != EventLogVerbQuery {
		t.Fatalf("first arg must be the read-only verb %q, got %v", EventLogVerbQuery, args)
	}
	if EventLogVerbQuery != "qe" {
		t.Fatalf("event log verb must be 'qe' (query-events), got %q", EventLogVerbQuery)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "/f:RenderedXml") {
		t.Errorf("must request rendered XML: %v", args)
	}
	if !strings.Contains(joined, "/c:50") {
		t.Errorf("must carry validated count: %v", args)
	}
	if !strings.Contains(joined, "Application") {
		t.Errorf("channel must be present: %v", args)
	}
	if strings.Contains(joined, "keyword") {
		t.Errorf("no user keyword may appear in args: %v", args)
	}
}

func TestBuildWevtutilArgs_ClampsCount(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"0", "/c:1"},
		{"99999", "/c:100"},
	} {
		var n int
		switch c.in {
		case "0":
			n = 0
		default:
			n = 99999
		}
		args := buildWevtutilArgs(EventQuery{Channel: "Application", MaxRecords: n})
		if !strings.Contains(strings.Join(args, " "), c.want) {
			t.Errorf("count %d should clamp to %s: %v", n, c.want, args)
		}
	}
}

func TestBuildEventXPath_RejectsUnsafeProvider(t *testing.T) {
	// 带单引号的 Provider 会被判为不安全 → 降级为不按 Provider 过滤（纵深防御）。
	xp := buildEventXPath("evil' or '1'='1", 0)
	if strings.Contains(xp, "evil") {
		t.Errorf("unsafe provider must be dropped from xpath, got %q", xp)
	}
	if xp != "*" {
		t.Errorf("with no time window and unsafe provider, xpath should be '*', got %q", xp)
	}
	// 合法 Provider 保留。
	xp2 := buildEventXPath("Microsoft-Windows-SoftwareRestrictionPolicies", 1000)
	if !strings.Contains(xp2, "Microsoft-Windows-SoftwareRestrictionPolicies") {
		t.Errorf("safe provider should be kept: %q", xp2)
	}
	if !strings.Contains(xp2, "timediff") {
		t.Errorf("time window should produce timediff condition: %q", xp2)
	}
}

// TestEventLogSource_OnlyReadOnlyVerb 静态守护：platform 源码中事件日志相关代码
// 绝不能出现修改/清空/卸载日志的 wevtutil 子命令字面量。
func TestEventLogSource_OnlyReadOnlyVerb(t *testing.T) {
	// 这些是 wevtutil 具有写/清除语义的子命令，绝不允许作为字符串字面量出现。
	forbiddenVerbs := []string{`"sl"`, `"cl"`, `"um"`, `"im"`, `"ep"`, `"epl"`, `"al"`, `"ala"`, `"rla"`}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		src := string(data)
		for _, v := range forbiddenVerbs {
			if strings.Contains(src, v) {
				t.Errorf("%s 含 wevtutil 写/清除子命令字面量 %s —— 事件日志只允许只读 qe", name, v)
			}
		}
	}
}
