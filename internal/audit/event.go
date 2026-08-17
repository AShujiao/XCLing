// Package audit 提供「拦截事件审计」与「临时放行草案」的**纯逻辑**与持久化。
//
// 安全边界（务必牢记）：
//   - 本包只**解析**事件日志 XML、在内存中过滤、生成草案并落用户数据目录；
//   - 绝不执行事件里出现的任何路径/命令，绝不修改任何日志或注册表；
//   - 事件日志的实际查询（wevtutil qe，只读）在 platform 包，本包只处理其只读输出；
//   - XML 字段抽取、路径清理、风险判定全部是**纯函数**，可跨平台单测。
package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"XCLing/internal/model"
)

// SourceSpec 描述一个将被只读查询的固定事件来源（通道 + 可选 Provider 过滤）。
// 全部为**固定常量**，绝不来自用户输入——这样进入查询命令的通道/Provider 是可信白名单。
type SourceSpec struct {
	Channel      string // 事件通道，如 "Application"
	ProviderName string // 可选 Provider Name 过滤（空表示不按 Provider 过滤）
	Label        string // 展示标签
}

// DefaultSources 返回 SRP / AppLocker 相关的固定查询来源白名单。
// 注意：不同 Windows 版本的具体 Event ID 可能不同，这里按 Provider/Channel 收敛，
// 不对 Event ID 做硬编码依赖。
func DefaultSources() []SourceSpec {
	return []SourceSpec{
		{Channel: "Application", ProviderName: "Microsoft-Windows-SoftwareRestrictionPolicies", Label: "软件限制策略（SRP）"},
		{Channel: "Application", ProviderName: "Software Restriction Policies", Label: "软件限制策略（旧源名）"},
		{Channel: "Microsoft-Windows-AppLocker/EXE and DLL", ProviderName: "", Label: "AppLocker · EXE/DLL"},
		{Channel: "Microsoft-Windows-AppLocker/MSI and Script", ProviderName: "", Label: "AppLocker · MSI/脚本"},
	}
}

// KnownChannels 返回来源白名单里的全部通道名（用于 filter.Channel 校验与能力展示）。
func KnownChannels() []string {
	seen := map[string]bool{}
	out := make([]string, 0, 4)
	for _, s := range DefaultSources() {
		if !seen[s.Channel] {
			seen[s.Channel] = true
			out = append(out, s.Channel)
		}
	}
	return out
}

// IsKnownChannel 判断某通道是否在白名单内。
func IsKnownChannel(ch string) bool {
	for _, c := range KnownChannels() {
		if c == ch {
			return true
		}
	}
	return false
}

// ValidateFilter 强制校验并归一化过滤条件（就地修改）。返回是否可用及说明。
//   - window：仅允许 24h / 7d，缺省/非法归一为 24h；
//   - max：夹取到 [MinAuditRecords, MaxAuditRecords]，缺省 DefaultAuditRecords；
//   - keyword：裁剪首尾空白，超长截断到 MaxAuditKeywordLen（keyword 绝不进入查询命令）；
//   - channel：非白名单值置空（表示查询全部白名单来源）。
func ValidateFilter(f *model.AuditFilter) {
	switch f.Window {
	case model.AuditWindow24h, model.AuditWindow7d:
	default:
		f.Window = model.AuditWindow24h
	}
	if f.Max == 0 {
		f.Max = model.DefaultAuditRecords
	}
	if f.Max < model.MinAuditRecords {
		f.Max = model.MinAuditRecords
	}
	if f.Max > model.MaxAuditRecords {
		f.Max = model.MaxAuditRecords
	}
	f.Keyword = strings.TrimSpace(f.Keyword)
	if len(f.Keyword) > model.MaxAuditKeywordLen {
		f.Keyword = f.Keyword[:model.MaxAuditKeywordLen]
	}
	if f.Channel != "" && !IsKnownChannel(f.Channel) {
		f.Channel = ""
	}
}

// WindowMillis 把窗口枚举转换为毫秒（用于事件日志时间过滤）。非法窗口按 24h。
func WindowMillis(window string) int64 {
	switch window {
	case model.AuditWindow7d:
		return 7 * 24 * 60 * 60 * 1000
	default:
		return 24 * 60 * 60 * 1000
	}
}

// ---- 事件 XML 解析（纯函数） ----

// xml 结构：只映射需要的字段，按本地名匹配（忽略命名空间）。
type xmlEvents struct {
	Events []xmlEvent `xml:"Event"`
}

type xmlEvent struct {
	System struct {
		Provider struct {
			Name string `xml:"Name,attr"`
		} `xml:"Provider"`
		EventID     string `xml:"EventID"`
		Level       string `xml:"Level"`
		TimeCreated struct {
			SystemTime string `xml:"SystemTime,attr"`
		} `xml:"TimeCreated"`
		Channel       string `xml:"Channel"`
		EventRecordID string `xml:"EventRecordID"`
		Security      struct {
			UserID string `xml:"UserID,attr"`
		} `xml:"Security"`
	} `xml:"System"`
	EventData struct {
		Data []struct {
			Name  string `xml:"Name,attr"`
			Value string `xml:",chardata"`
		} `xml:"Data"`
	} `xml:"EventData"`
	UserData struct {
		Raw string `xml:",innerxml"`
	} `xml:"UserData"`
	RenderingInfo struct {
		Message string `xml:"Message"`
	} `xml:"RenderingInfo"`
}

// ParseEvents 解析 wevtutil qe 的（只读）合并输出为审计事件列表。
// rawXML 是若干 <Event>…</Event> 的拼接；本函数包裹为单根后解析。source 提供通道兜底。
// 解析失败不 panic：尽力而为返回已成功解析的部分。
func ParseEvents(rawXML string, source SourceSpec) []model.AuditEvent {
	raw := strings.TrimSpace(rawXML)
	if raw == "" {
		return nil
	}
	// 去掉可能的 BOM 与 XML 声明，避免多段声明导致包裹后非法。
	raw = strings.ReplaceAll(raw, "\uFEFF", "")
	raw = stripXMLDeclarations(raw)
	// 移除XML命名空间声明，解决Windows事件XML带默认xmlns导致Go无法解析元素的问题
	raw = stripXMLNamespaces(raw)

	var parsed xmlEvents
	wrapped := "<Events>" + raw + "</Events>"
	var out []model.AuditEvent
	if err := xml.Unmarshal([]byte(wrapped), &parsed); err != nil {
		// 兜底：逐个 <Event> 尝试解析。
		out = parsePerEvent(raw, source)
	} else {
		out = make([]model.AuditEvent, 0, len(parsed.Events))
		for i := range parsed.Events {
			rawEvent := extractRawEventXML(raw, i)
			out = append(out, toAuditEvent(parsed.Events[i], source, rawEvent))
		}
	}
	return deduplicateAndSort(out)
}

func parsePerEvent(raw string, source SourceSpec) []model.AuditEvent {
	out := make([]model.AuditEvent, 0)
	parts := strings.SplitAfter(raw, "</Event>")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if !strings.Contains(p, "<Event") {
			continue
		}
		var e xmlEvent
		// 移除命名空间
		pClean := stripXMLNamespaces(p)
		if err := xml.Unmarshal([]byte(pClean), &e); err != nil {
			continue
		}
		out = append(out, toAuditEvent(e, source, p))
	}
	return deduplicateAndSort(out)
}

// deduplicateAndSort 去重：同一可执行路径5分钟内的多次拦截只保留最新一条；并按时间倒序排列
func deduplicateAndSort(events []model.AuditEvent) []model.AuditEvent {
	if len(events) == 0 {
		return events
	}

	// 先按时间倒序排序（最新在前）
	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp > events[j].Timestamp
	})

	// 去重：相同路径，5分钟内只保留一条
	const dedupWindow = 5 * time.Minute
	type seenKey struct {
		path string
		hour int64 // 按时间窗口分桶
	}
	seen := make(map[seenKey]struct{})
	result := make([]model.AuditEvent, 0, len(events))

	for _, ev := range events {
		if ev.ExecutablePath == "" {
			// 没有路径的事件不去重，直接保留
			result = append(result, ev)
			continue
		}

		// 解析时间
		t, err := time.Parse(time.RFC3339, ev.Timestamp)
		if err != nil {
			t, err = time.Parse(time.RFC3339Nano, ev.Timestamp)
		}
		if err != nil {
			// 解析失败不去重
			result = append(result, ev)
			continue
		}

		// 按5分钟窗口分桶：同一窗口内相同路径只保留第一条（最新）
		bucket := t.Unix() / int64(dedupWindow.Seconds())
		key := seenKey{path: strings.ToLower(ev.ExecutablePath), hour: bucket}
		if _, exists := seen[key]; !exists {
			seen[key] = struct{}{}
			result = append(result, ev)
		}
	}

	return result
}

// extractRawEventXML extracts the raw XML for the i-th event from the concatenated output
func extractRawEventXML(raw string, index int) string {
	startTag := "<Event"
	endTag := "</Event>"
	pos := 0
	for i := 0; i <= index; i++ {
		start := strings.Index(raw[pos:], startTag)
		if start < 0 {
			return ""
		}
		start += pos
		end := strings.Index(raw[start:], endTag)
		if end < 0 {
			return ""
		}
		end += start + len(endTag)
		if i == index {
			return raw[start:end]
		}
		pos = end
	}
	return ""
}

func stripXMLDeclarations(s string) string {
	for {
		i := strings.Index(s, "<?xml")
		if i < 0 {
			return s
		}
		j := strings.Index(s[i:], "?>")
		if j < 0 {
			return s
		}
		s = s[:i] + s[i+j+2:]
	}
}

// stripXMLNamespaces removes default xmlns and xmlns:* attributes from XML to make parsing easier
// Windows event XML includes default namespaces that Go's XML parser struggles with for simple extraction
func stripXMLNamespaces(s string) string {
	// Remove xmlns="..." or xmlns='...' attributes (default namespace)
	xmlnsRe := regexp.MustCompile(`\s+xmlns=(?:"[^"]*"|'[^']*')`)
	s = xmlnsRe.ReplaceAllString(s, "")
	// Remove xmlns:prefix="..." or xmlns:prefix='...' attributes (prefixed namespaces)
	xmlnsPrefixRe := regexp.MustCompile(`\s+xmlns:[a-zA-Z0-9]+=(?:"[^"]*"|'[^']*')`)
	s = xmlnsPrefixRe.ReplaceAllString(s, "")
	return s
}

func toAuditEvent(e xmlEvent, source SourceSpec, rawEventXML string) model.AuditEvent {
	channel := strings.TrimSpace(e.System.Channel)
	if channel == "" {
		channel = source.Channel
	}
	provider := strings.TrimSpace(e.System.Provider.Name)
	if provider == "" {
		provider = source.ProviderName
	}

	// 组合可供抽取路径的文本：EventData 各字段 + 渲染消息 + 整个原始XML（最可靠方式）
	var dataParts []string
	for _, d := range e.EventData.Data {
		v := strings.TrimSpace(d.Value)
		if v == "" {
			continue
		}
		if d.Name != "" {
			dataParts = append(dataParts, d.Name+"="+v)
		} else {
			dataParts = append(dataParts, v)
		}
	}
	rawSummary := SanitizeText(strings.Join(dataParts, " · "))
	message := SanitizeText(collapseSpaces(e.RenderingInfo.Message))

	// 在所有文本 + 整个原始XML中查找路径，包括直接匹配 <AttemptedPath> 标签
	combined := strings.Join(dataParts, " ") + " " + e.RenderingInfo.Message + " " + rawEventXML

	// 优先从 <AttemptedPath> 标签直接提取（中文Windows SRP 865事件路径位置）
	attemptedPath := ""
	if pathStart := strings.Index(combined, "<AttemptedPath>"); pathStart >= 0 {
		pathStart += len("<AttemptedPath>")
		if pathEnd := strings.Index(combined[pathStart:], "</AttemptedPath>"); pathEnd >= 0 {
			attemptedPath = strings.TrimSpace(combined[pathStart : pathStart+pathEnd])
		}
	}

	// 正则匹配可执行文件路径
	exePath := ExtractExecutablePath(combined)
	// 如果正则没找到但找到了AttemptedPath，直接使用
	if exePath == "" && attemptedPath != "" {
		exePath = SanitizePath(attemptedPath)
	}
	risk, reasons := ClassifyRisk(exePath)

	ev := model.AuditEvent{
		Timestamp:      normalizeTime(e.System.TimeCreated.SystemTime),
		Provider:       provider,
		Channel:        channel,
		EventID:        parseIntSafe(e.System.EventID),
		Level:          decodeLevel(e.System.Level),
		ExecutablePath: exePath,
		User:           SanitizeText(strings.TrimSpace(e.System.Security.UserID)),
		Message:        truncate(message, 600),
		RawSummary:     truncate(rawSummary, 400),
		Risk:           risk,
		RiskReasons:    reasons,
	}
	ev.ID = eventID(channel, e.System.EventRecordID, ev)
	return ev
}

func eventID(channel, recordID string, ev model.AuditEvent) string {
	if strings.TrimSpace(recordID) != "" {
		return sanitizeIDPart(channel) + "-" + sanitizeIDPart(recordID)
	}
	h := sha256.Sum256([]byte(channel + "|" + ev.Provider + "|" +
		strconv.Itoa(ev.EventID) + "|" + ev.Timestamp + "|" + ev.ExecutablePath))
	return "ev-" + hex.EncodeToString(h[:8])
}

func sanitizeIDPart(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if len(out) > 60 {
		out = out[:60]
	}
	return out
}

var exePathRe = regexp.MustCompile(`(?i)[a-z]:\\[^\r\n"<>|?*]*?\.(?:exe|dll|com|scr|bat|cmd|ps1|vbs|js|jse|wsf|msi|jar|hta|cpl|msc|pif|sys)`)

// ExtractExecutablePath 从文本中抽取第一个 Windows 可执行文件路径。纯函数，绝不访问文件系统。
func ExtractExecutablePath(text string) string {
	if text == "" {
		return ""
	}
	m := exePathRe.FindString(text)
	return SanitizePath(m)
}

// SanitizePath 清理路径：去首尾空白与包裹引号、去控制字符、折叠内部多余空白。
// **仅字符串处理**，不解析、不访问真实文件系统、绝不执行。
func SanitizePath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, `"'`)
	p = strings.TrimSpace(p)
	p = stripControl(p)
	return p
}

// SanitizeText 清理任意文本：去控制字符、折叠空白。用于展示，绝不执行其中内容。
func SanitizeText(s string) string {
	return collapseSpaces(stripControl(s))
}

func stripControl(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '\t' || r == '\n' || r == '\r' {
			b.WriteRune(' ')
			continue
		}
		if r < 0x20 || r == 0x7f {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// ClassifyRisk 依据可执行文件路径所在位置推断风险等级。纯函数。
//   - 用户可写区（Downloads/Temp/AppData/Users 根等）：high；
//   - 系统/程序目录：low；
//   - 其它可解析路径：medium；空/不可解析：unknown。
func ClassifyRisk(path string) (string, []string) {
	if strings.TrimSpace(path) == "" {
		return model.RiskUnknown, []string{"事件中未能解析出明确的可执行文件路径。"}
	}
	n := strings.ToLower(strings.ReplaceAll(path, "/", `\`))
	reasons := make([]string, 0, 2)

	// 高风险：常见恶意投放/用户可写目录。
	highMarkers := []struct{ frag, why string }{
		{`\downloads\`, "位于下载目录，是常见的恶意程序投放位置。"},
		{`\appdata\local\temp\`, "位于用户临时目录（Temp），常被用于落地并执行未知程序。"},
		{`\temp\`, "位于临时目录，常被用于落地并执行未知程序。"},
		{`\appdata\`, "位于 AppData（用户可写），放行需格外谨慎。"},
		{`\users\public\`, "位于公共用户目录（可被任意用户写入）。"},
	}
	for _, m := range highMarkers {
		if strings.Contains(n, m.frag) {
			reasons = append(reasons, m.why)
			return model.RiskHigh, reasons
		}
	}
	// 系统/程序目录：低风险。
	lowPrefixes := []string{`c:\windows`, `c:\program files`, `c:\program files (x86)`}
	for _, p := range lowPrefixes {
		if strings.HasPrefix(n, p) {
			reasons = append(reasons, "位于系统/程序目录，通常为受信任位置（仍建议核对来源）。")
			return model.RiskLow, reasons
		}
	}
	// 其它用户根目录（c:\users\<name>\...，非上面高危子目录）：中等。
	if strings.HasPrefix(n, `c:\users\`) {
		reasons = append(reasons, "位于用户主目录下（用户可写），放行前请确认来源可信。")
		return model.RiskMedium, reasons
	}
	reasons = append(reasons, "路径不在常见系统目录内，放行前请人工确认来源。")
	return model.RiskMedium, reasons
}

func decodeLevel(raw string) string {
	switch strings.TrimSpace(raw) {
	case "1":
		return "critical"
	case "2":
		return "error"
	case "3":
		return "warning"
	case "4":
		return "information"
	case "5":
		return "verbose"
	case "0":
		return "information"
	default:
		return "unknown"
	}
}

func parseIntSafe(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

func normalizeTime(systemTime string) string {
	s := strings.TrimSpace(systemTime)
	if s == "" {
		return ""
	}
	// 事件 SystemTime 形如 2024-01-02T03:04:05.1234567Z；解析后统一 RFC3339（本地时间）
	layouts := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.9999999Z07:00"}
	for _, l := range layouts {
		if t, err := time.Parse(l, s); err == nil {
			return t.Local().Format(time.RFC3339)
		}
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// FilterAndSort 对已解析事件按关键词过滤（内存内），按时间倒序排序，并夹取到 max。
// 返回过滤排序后的列表与是否被截断。keyword 在此匹配 provider/message/path/rawSummary。
func FilterAndSort(events []model.AuditEvent, keyword string, max int) ([]model.AuditEvent, bool) {
	kw := strings.ToLower(strings.TrimSpace(keyword))
	filtered := make([]model.AuditEvent, 0, len(events))
	for _, e := range events {
		if kw != "" {
			hay := strings.ToLower(e.Provider + " " + e.Message + " " + e.ExecutablePath + " " + e.RawSummary + " " + e.User)
			if !strings.Contains(hay, kw) {
				continue
			}
		}
		filtered = append(filtered, e)
	}
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].Timestamp > filtered[j].Timestamp })
	truncated := false
	if max > 0 && len(filtered) > max {
		filtered = filtered[:max]
		truncated = true
	}
	return filtered, truncated
}
