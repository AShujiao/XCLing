package platform

import (
	"hash/fnv"
	"strings"

	"XCLing/internal/model"
)

// 本文件是「已安装软件发现」的**纯函数内核**：不含任何 build tag、不导入任何
// OS/registry 包，因此在所有平台都能编译与单测。Windows 侧的 installed_apps_windows.go
// 只负责从注册表**只读**读出原始值，然后调用这里的纯函数做解析/过滤/去重。
//
// 安全边界：这里只做字符串解析与（通过注入的 pathExists）只读存在性判断，
// 绝不执行 DisplayIcon / UninstallString / ExecutablePath 指向的任何程序。

// RawUninstallEntry 是从单个卸载项注册表子键读出的原始值（未加工）。
type RawUninstallEntry struct {
	KeyName         string // 子键名（GUID 或程序名），用于生成稳定 id
	DisplayName     string
	DisplayVersion  string
	Publisher       string
	InstallLocation string
	DisplayIcon     string
	UninstallString string
	SystemComponent int    // 1 表示系统组件，应跳过
	Source          string // 发现来源标签（HKLM64 / HKLM32 / HKCU）
}

// StripDisplayIconIndex 从 DisplayIcon 值中安全提取路径部分。纯函数，绝不执行。
//
// DisplayIcon 常见形态：
//
//	"C:\Program Files\App\app.exe",0   → C:\Program Files\App\app.exe
//	C:\Program Files\App\app.exe,1     → C:\Program Files\App\app.exe
//	C:\App\app.exe                     → C:\App\app.exe
//	%ProgramFiles%\App\app.exe,0       → %ProgramFiles%\App\app.exe（env 稍后展开）
func StripDisplayIconIndex(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	// 带引号：取第一对引号内的内容，忽略其后的 ,index。
	if s[0] == '"' {
		if end := strings.IndexByte(s[1:], '"'); end >= 0 {
			return strings.TrimSpace(s[1 : 1+end])
		}
		return strings.TrimSpace(strings.Trim(s, `"`))
	}
	// 未带引号：仅剥离结尾的 “,<可选符号><数字>” 图标索引，避免误伤路径中的逗号。
	if idx := strings.LastIndexByte(s, ','); idx >= 0 {
		if isIconIndexSuffix(s[idx+1:]) {
			return strings.TrimSpace(s[:idx])
		}
	}
	return s
}

// isIconIndexSuffix 判断逗号后的内容是否是纯图标索引（可含正负号）。
func isIconIndexSuffix(suffix string) bool {
	suffix = strings.TrimSpace(suffix)
	if suffix == "" {
		return false
	}
	start := 0
	if suffix[0] == '-' || suffix[0] == '+' {
		start = 1
	}
	if start == len(suffix) {
		return false
	}
	for i := start; i < len(suffix); i++ {
		if suffix[i] < '0' || suffix[i] > '9' {
			return false
		}
	}
	return true
}

// ExpandWinEnvVars 展开 Windows 风格的 %VAR% 引用。getenv 注入以便纯函数单测；
// 只做字符串替换，绝不执行任何内容。未知变量原样保留。
func ExpandWinEnvVars(s string, getenv func(string) string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	for {
		start := strings.IndexByte(s, '%')
		if start < 0 {
			b.WriteString(s)
			break
		}
		end := strings.IndexByte(s[start+1:], '%')
		if end < 0 {
			b.WriteString(s)
			break
		}
		name := s[start+1 : start+1+end]
		b.WriteString(s[:start])
		if v := getenv(name); v != "" {
			b.WriteString(v)
		} else {
			b.WriteString("%" + name + "%") // 未知变量原样保留
		}
		s = s[start+1+end+1:]
	}
	return b.String()
}

// normalizeWinPath 归一化 Windows 路径：去首尾空白与尾部反斜杠、正斜杠转反斜杠、小写。
// 纯字符串处理，不访问文件系统（故可跨平台）。
func normalizeWinPath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.ReplaceAll(p, "/", `\`)
	p = strings.TrimRight(p, `\`)
	return strings.ToLower(p)
}

// trimPathValue 清理注册表里可能带引号/空白的路径值（部分安装器会把 InstallLocation 存成带引号）。
func trimPathValue(p string) string {
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(p), `"`))
}

// winParentDir 返回 Windows 路径的父目录（按反斜杠切分，不用 filepath 以免受运行平台影响）。
func winParentDir(p string) string {
	p = strings.TrimRight(strings.TrimSpace(strings.ReplaceAll(p, "/", `\`)), `\`)
	idx := strings.LastIndexByte(p, '\\')
	if idx <= 0 {
		return ""
	}
	return p[:idx]
}

// broadContainerDirs 是绝不允许“整目录放行”的容器目录（归一化）。
var broadContainerDirs = map[string]bool{
	`c:\windows`:             true,
	`c:\program files`:       true,
	`c:\program files (x86)`: true,
	`c:\programdata`:         true,
	`c:\users`:               true,
	`c:\users\public`:        true,
}

// userContainerLeaves 是用户配置目录中的“容器”末段：位于 c:\users 下且以这些名字结尾的目录
// 不能被整目录放行（会等价于放行整个用户可写区），但其更深层的具体应用子目录可以。
var userContainerLeaves = map[string]bool{
	"appdata":   true,
	"local":     true,
	"locallow":  true,
	"roaming":   true,
	"programs":  true,
	"downloads": true,
	"desktop":   true,
	"documents": true,
	"temp":      true,
	"tmp":       true,
	"cache":     true,
}

// IsBroadDir 判断某目录是否“过宽”，不适合作为“目录\*”白名单规则。纯函数。
func IsBroadDir(dir string) bool {
	norm := normalizeWinPath(dir)
	if norm == "" {
		return true
	}
	// 盘符根：c: / c:\
	if len(norm) == 2 && norm[1] == ':' {
		return true
	}
	if broadContainerDirs[norm] {
		return true
	}
	segs := strings.Split(norm, `\`)
	// 用户配置根：c:\users\<name>（恰好三段）
	if len(segs) == 3 && segs[1] == "users" {
		return true
	}
	// 用户区容器叶子（appdata/local/roaming/programs/downloads/... ）
	if len(segs) >= 3 && segs[1] == "users" {
		if userContainerLeaves[segs[len(segs)-1]] {
			return true
		}
	}
	// 任意盘的 temp/tmp 叶子也视为过宽
	leaf := segs[len(segs)-1]
	if leaf == "temp" || leaf == "tmp" {
		return true
	}
	return false
}

// deriveCandidate 根据安装目录与可执行文件路径推导“安全候选”。
// 返回：候选路径、是否目录、置信度、告警。绝不执行任何内容，仅用 pathExists 做只读探测。
func deriveCandidate(instLoc, exePath string, pathExists func(string) bool) (candidate string, isDir bool, confidence string, warnings []string) {
	warnings = make([]string, 0)
	// 1. 优先用安装目录做“目录\*”规则（前提：足够具体）。
	if instLoc != "" {
		if IsBroadDir(instLoc) {
			warnings = append(warnings, "安装目录过宽（"+instLoc+"），不能整目录放行。")
		} else if pathExists(instLoc) {
			return instLoc, true, model.ConfidenceHigh, warnings
		} else {
			return instLoc, true, model.ConfidenceMedium,
				append(warnings, "安装目录未在磁盘上找到，请确认路径后再放行。")
		}
	}
	// 2. 回退到可执行文件路径。
	if exePath != "" {
		exeDir := winParentDir(exePath)
		if exeDir != "" && !IsBroadDir(exeDir) {
			if pathExists(exeDir) {
				return exeDir, true, model.ConfidenceHigh, warnings
			}
			return exeDir, true, model.ConfidenceMedium,
				append(warnings, "可执行文件所在目录未找到，请确认后再放行。")
		}
		// 目录过宽（用户可写区）→ 仅放行该单个文件。
		if pathExists(exePath) {
			return exePath, false, model.ConfidenceMedium,
				append(warnings, "可执行文件位于用户可写目录，仅放行该单个文件而非整目录。")
		}
		return exePath, false, model.ConfidenceLow,
			append(warnings, "可执行文件路径无法验证存在，勾选前请务必人工确认。")
	}
	// 3. 无任何可用候选。
	return "", false, model.ConfidenceLow, warnings
}

// BuildDiscoveredApp 把一条原始卸载项加工成 DiscoveredApp。返回 (app, ok)；ok=false 表示
// 该条目不应展示（无名称、系统组件、或无法导出任何安全候选路径）。
//
// getenv / pathExists 均为注入依赖，便于纯函数单测；二者都只做“读”，不产生副作用。
func BuildDiscoveredApp(raw RawUninstallEntry, pathExists func(string) bool, getenv func(string) string) (model.DiscoveredApp, bool) {
	name := strings.TrimSpace(raw.DisplayName)
	if name == "" {
		return model.DiscoveredApp{}, false // 无名称：不展示
	}
	if raw.SystemComponent == 1 {
		return model.DiscoveredApp{}, false // 系统组件：跳过
	}

	// 解析 DisplayIcon → 可执行文件路径（安全：剥离引号/逗号索引 + 展开 env，绝不执行）。
	exePath := ""
	if raw.DisplayIcon != "" {
		p := ExpandWinEnvVars(StripDisplayIconIndex(raw.DisplayIcon), getenv)
		if strings.HasSuffix(strings.ToLower(p), ".exe") {
			exePath = p
		}
	}
	instLoc := ExpandWinEnvVars(trimPathValue(raw.InstallLocation), getenv)
	instLoc = strings.TrimRight(instLoc, `\`)

	candidate, isDir, confidence, warnings := deriveCandidate(instLoc, exePath, pathExists)
	if candidate == "" {
		return model.DiscoveredApp{}, false // 无法导出安全候选：不展示
	}

	app := model.DiscoveredApp{
		ID:              stableAppID(raw.Source, name, candidate),
		DisplayName:     name,
		Publisher:       strings.TrimSpace(raw.Publisher),
		DisplayVersion:  strings.TrimSpace(raw.DisplayVersion),
		InstallLocation: instLoc,
		DisplayIcon:     raw.DisplayIcon,
		UninstallString: raw.UninstallString, // 仅存档，绝不执行
		Source:          raw.Source,
		ExecutablePath:  exePath,
		CandidatePath:   candidate,
		CandidateIsDir:  isDir,
		Confidence:      confidence,
		Warnings:        warnings,
	}
	// 低置信度（路径无法验证存在）→ 不默认可勾选，促使用户人工确认或改用自定义路径。
	app.Selectable = confidence != model.ConfidenceLow
	return app, true
}

// stableAppID 生成稳定 id：来源 + 归一化(name|candidate) 的 FNV 哈希。
func stableAppID(source, name, candidate string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(name))))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(normalizeWinPath(candidate)))
	return "app-" + strings.ToLower(source) + "-" + intToHex(h.Sum32())
}

func intToHex(v uint32) string {
	const digits = "0123456789abcdef"
	if v == 0 {
		return "0"
	}
	var buf [8]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = digits[v&0xf]
		v >>= 4
	}
	return string(buf[i:])
}

// confidenceRank 用于去重时挑选更可信的重复项。
func confidenceRank(c string) int {
	switch c {
	case model.ConfidenceHigh:
		return 3
	case model.ConfidenceMedium:
		return 2
	default:
		return 1
	}
}

// DedupeApps 按“归一化(名称)|归一化(候选路径)”去重。同一应用常在 HKLM64/HKLM32/HKCU
// 多个视图重复出现；保留置信度更高者，置信度相同保留先出现者。纯函数，稳定顺序。
func DedupeApps(apps []model.DiscoveredApp) []model.DiscoveredApp {
	type slot struct {
		idx int
		app model.DiscoveredApp
	}
	best := map[string]slot{}
	order := make([]string, 0, len(apps))
	for i, a := range apps {
		key := strings.ToLower(strings.TrimSpace(a.DisplayName)) + "|" + normalizeWinPath(a.CandidatePath)
		if cur, ok := best[key]; ok {
			if confidenceRank(a.Confidence) > confidenceRank(cur.app.Confidence) {
				best[key] = slot{idx: cur.idx, app: a} // 保留原始出现顺序，替换为更可信数据
			}
			continue
		}
		best[key] = slot{idx: i, app: a}
		order = append(order, key)
	}
	out := make([]model.DiscoveredApp, 0, len(order))
	for _, k := range order {
		out = append(out, best[k].app)
	}
	return out
}
