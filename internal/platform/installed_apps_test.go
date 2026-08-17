package platform

import (
	"testing"

	"XCLing/internal/model"
)

// 说明：这些测试全部作用于纯函数（字符串解析 / 过滤 / 去重）与注入的假 pathExists / getenv，
// 绝不读写真实注册表、绝不执行任何文件。DiscoverInstalledApps 的运行时只读调用单独做非崩溃断言。

func TestStripDisplayIconIndex(t *testing.T) {
	cases := map[string]string{
		`"C:\Program Files\App\app.exe",0`: `C:\Program Files\App\app.exe`,
		`C:\App\app.exe,1`:                 `C:\App\app.exe`,
		`C:\App\app.exe`:                   `C:\App\app.exe`,
		`"C:\App\app.exe"`:                 `C:\App\app.exe`,
		`%ProgramFiles%\App\app.exe,0`:     `%ProgramFiles%\App\app.exe`,
		`C:\weird,name\app.exe`:            `C:\weird,name\app.exe`, // 逗号后非纯数字：不当作图标索引
		``:                                 ``,
		`   `:                              ``,
	}
	for in, want := range cases {
		if got := StripDisplayIconIndex(in); got != want {
			t.Errorf("StripDisplayIconIndex(%q)=%q want %q", in, got, want)
		}
	}
}

func TestExpandWinEnvVars(t *testing.T) {
	getenv := func(k string) string {
		if k == "ProgramFiles" {
			return `C:\Program Files`
		}
		return ""
	}
	cases := map[string]string{
		`%ProgramFiles%\App\app.exe`: `C:\Program Files\App\app.exe`,
		`%Unknown%\x`:                `%Unknown%\x`, // 未知变量原样保留
		`no vars here`:               `no vars here`,
	}
	for in, want := range cases {
		if got := ExpandWinEnvVars(in, getenv); got != want {
			t.Errorf("ExpandWinEnvVars(%q)=%q want %q", in, got, want)
		}
	}
}

func TestIsBroadDir(t *testing.T) {
	broad := []string{
		`C:\`, `C:\Windows`, `C:\Program Files`, `C:\Program Files (x86)`,
		`C:\Users`, `C:\Users\bob`, `C:\Users\bob\AppData`, `C:\Users\bob\AppData\Local`,
		`C:\Users\bob\Downloads`, `C:\Users\bob\AppData\Local\Temp`, `D:\Temp`,
	}
	for _, d := range broad {
		if !IsBroadDir(d) {
			t.Errorf("IsBroadDir(%q) should be true (broad)", d)
		}
	}
	specific := []string{
		`C:\Program Files\Acme`, `D:\Tools\Acme`,
		`C:\Users\bob\AppData\Local\Programs\VSCode`,
	}
	for _, d := range specific {
		if IsBroadDir(d) {
			t.Errorf("IsBroadDir(%q) should be false (specific)", d)
		}
	}
}

func trueFor(paths ...string) func(string) bool {
	set := map[string]bool{}
	for _, p := range paths {
		set[p] = true
	}
	return func(p string) bool { return set[p] }
}

func noEnv(k string) string { return "%" + k + "%" }

func TestBuildDiscoveredApp_NormalInstallLocation(t *testing.T) {
	raw := RawUninstallEntry{
		KeyName:         "{ACME}",
		DisplayName:     "Acme Reader",
		Publisher:       "Acme Corp",
		DisplayVersion:  "3.2.1",
		InstallLocation: `C:\Program Files\Acme`,
		DisplayIcon:     `"C:\Program Files\Acme\acme.exe",0`,
		UninstallString: `"C:\Program Files\Acme\uninstall.exe"`,
		Source:          model.SourceHKLM64,
	}
	app, ok := BuildDiscoveredApp(raw, trueFor(`C:\Program Files\Acme`), noEnv)
	if !ok {
		t.Fatal("expected includable app")
	}
	if app.CandidatePath != `C:\Program Files\Acme` || !app.CandidateIsDir {
		t.Fatalf("expected dir candidate at install location, got %q dir=%v", app.CandidatePath, app.CandidateIsDir)
	}
	if app.Confidence != model.ConfidenceHigh || !app.Selectable {
		t.Fatalf("expected high confidence & selectable, got %s selectable=%v", app.Confidence, app.Selectable)
	}
	if app.ExecutablePath != `C:\Program Files\Acme\acme.exe` {
		t.Fatalf("exe path parse mismatch: %q", app.ExecutablePath)
	}
	if app.ID == "" {
		t.Fatal("expected stable id")
	}
	if app.Warnings == nil {
		t.Fatal("warnings must serialize as [] instead of null")
	}
}

func TestBuildDiscoveredApp_ExcludeNoNameAndSystemComponent(t *testing.T) {
	if _, ok := BuildDiscoveredApp(RawUninstallEntry{DisplayName: "   "}, trueFor(), noEnv); ok {
		t.Error("empty name should be excluded")
	}
	raw := RawUninstallEntry{DisplayName: "Sys", InstallLocation: `C:\Program Files\Sys`, SystemComponent: 1}
	if _, ok := BuildDiscoveredApp(raw, trueFor(`C:\Program Files\Sys`), noEnv); ok {
		t.Error("system component should be excluded")
	}
}

func TestBuildDiscoveredApp_ExcludeNoSafeCandidate(t *testing.T) {
	// 无安装目录、DisplayIcon 指向 dll（非 exe）→ 无候选 → 不展示。
	raw := RawUninstallEntry{DisplayName: "NoExe", DisplayIcon: `C:\App\icon.dll,0`}
	if _, ok := BuildDiscoveredApp(raw, trueFor(), noEnv); ok {
		t.Error("entry without exportable candidate should be excluded")
	}
}

func TestBuildDiscoveredApp_PortableSingleFileInUserDir(t *testing.T) {
	raw := RawUninstallEntry{
		DisplayName: "Portable Tool",
		DisplayIcon: `C:\Users\bob\Downloads\tool.exe`,
		Source:      model.SourceHKCU,
	}
	app, ok := BuildDiscoveredApp(raw, trueFor(`C:\Users\bob\Downloads\tool.exe`), noEnv)
	if !ok {
		t.Fatal("expected includable portable app")
	}
	if app.CandidateIsDir {
		t.Fatal("user-writable dir must yield single-file candidate, not a directory rule")
	}
	if app.CandidatePath != `C:\Users\bob\Downloads\tool.exe` {
		t.Fatalf("candidate should be the single exe, got %q", app.CandidatePath)
	}
	if app.Confidence != model.ConfidenceMedium {
		t.Fatalf("expected medium confidence, got %s", app.Confidence)
	}
}

func TestBuildDiscoveredApp_UnverifiablePathLowConfidence(t *testing.T) {
	raw := RawUninstallEntry{DisplayName: "Ghost", DisplayIcon: `C:\Users\bob\Downloads\ghost.exe`}
	app, ok := BuildDiscoveredApp(raw, trueFor() /* nothing exists */, noEnv)
	if !ok {
		t.Fatal("still includable but low confidence")
	}
	if app.Confidence != model.ConfidenceLow {
		t.Fatalf("expected low confidence, got %s", app.Confidence)
	}
	if app.Selectable {
		t.Fatal("low-confidence (unverifiable) app must not be auto-selectable")
	}
	if len(app.Warnings) == 0 {
		t.Fatal("expected a warning about unverifiable path")
	}
}

func TestBuildDiscoveredApp_DoesNotExecuteUninstallString(t *testing.T) {
	// UninstallString 必须原样存档、绝不执行——这里只断言它被保留且未被改写。
	raw := RawUninstallEntry{
		DisplayName:     "Acme",
		InstallLocation: `C:\Program Files\Acme`,
		UninstallString: `cmd /c del /f /q C:\important`,
		Source:          model.SourceHKLM64,
	}
	app, ok := BuildDiscoveredApp(raw, trueFor(`C:\Program Files\Acme`), noEnv)
	if !ok {
		t.Fatal("expected includable")
	}
	if app.UninstallString != `cmd /c del /f /q C:\important` {
		t.Fatalf("uninstall string must be archived verbatim, got %q", app.UninstallString)
	}
}

func TestBuildDiscoveredApp_TrimsQuotedInstallLocation(t *testing.T) {
	// 部分安装器把 InstallLocation 存成带引号；候选路径不得残留引号。
	raw := RawUninstallEntry{
		DisplayName:     "Antigravity Tools",
		InstallLocation: `"C:\Users\dell\AppData\Local\Antigravity Tools"`,
		Source:          model.SourceHKCU,
	}
	app, ok := BuildDiscoveredApp(raw, trueFor(`C:\Users\dell\AppData\Local\Antigravity Tools`), noEnv)
	if !ok {
		t.Fatal("expected includable app")
	}
	if app.CandidatePath != `C:\Users\dell\AppData\Local\Antigravity Tools` {
		t.Fatalf("candidate must not contain quotes, got %q", app.CandidatePath)
	}
	if !app.CandidateIsDir {
		t.Fatal("a specific AppData\\Local\\<App> subdir should be a directory candidate")
	}
}

func TestDedupeApps(t *testing.T) {
	apps := []model.DiscoveredApp{
		{DisplayName: "Acme", CandidatePath: `C:\Program Files\Acme`, Confidence: model.ConfidenceMedium, Source: model.SourceHKLM32},
		{DisplayName: "Acme", CandidatePath: `C:\Program Files\Acme`, Confidence: model.ConfidenceHigh, Source: model.SourceHKLM64},
		{DisplayName: "Other", CandidatePath: `D:\Tools\Other`, Confidence: model.ConfidenceHigh, Source: model.SourceHKCU},
	}
	out := DedupeApps(apps)
	if len(out) != 2 {
		t.Fatalf("expected 2 after dedupe, got %d", len(out))
	}
	for _, a := range out {
		if a.DisplayName == "Acme" && a.Confidence != model.ConfidenceHigh {
			t.Fatalf("dedupe should keep higher confidence, got %s", a.Confidence)
		}
	}
}

func TestDiscoverInstalledApps_DoesNotPanic(t *testing.T) {
	apps := DiscoverInstalledApps() // 运行时只读调用（非 Windows 返回空）
	for _, a := range apps {
		if a.DisplayName == "" || a.CandidatePath == "" {
			t.Fatalf("discovered app must have name and candidate: %+v", a)
		}
	}
}
