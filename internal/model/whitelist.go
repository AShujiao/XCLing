package model

// 本文件定义 Phase 1a「白名单向导」链路的全部跨 IPC 数据结构：
//   已安装软件发现 → 勾选/自定义路径 → 生成草案 → 预检模拟。
//
// 安全边界（务必牢记）：Phase 1a 仍然**绝不写入/删除/修改** HKLM SRP 或任何注册表。
// 这里的 DiscoveredApp 仅来自“卸载项注册表”的只读枚举；UninstallString 只存不执行；
// WhitelistDraft / PreflightReport 都是纯内存计算的“意图”与“模拟”，不会落地为真实规则。
//
// RPC 序列化约束：字段只能是 string / int / bool / struct / slice / map，
// 禁止 time.Time / interface{} / []byte / json.RawMessage。时间统一 RFC3339 字符串。

// 发现来源标签（DiscoveredApp.Source）。
const (
	SourceHKLM64     = "HKLM64"     // HKLM 64 位视图 Uninstall
	SourceHKLM32     = "HKLM32"     // HKLM 32 位视图（Wow6432Node）
	SourceHKCU       = "HKCU"       // 当前用户 Uninstall
	SourceUserCustom = "userCustom" // 用户手工添加（目录/exe）
	SourceMockDemo   = "mockDemo"   // 浏览器降级演示数据
)

// 置信度（DiscoveredApp.Confidence）：候选路径的可信/可用程度。
const (
	ConfidenceHigh   = "high"   // 安装目录明确且存在，落在程序目录下
	ConfidenceMedium = "medium" // 由可执行文件路径推导，目录较具体
	ConfidenceLow    = "low"    // 路径过宽/无法验证存在，需人工确认
)

// 自定义路径类型（CustomPathEntry.Kind）。
const (
	CustomKindDirectory = "directory" // 目录（生成 目录\* 规则）
	CustomKindFile      = "file"      // 单个可执行文件（生成精确路径规则）
)

// DiscoveredApp 描述一条“已安装软件”候选。全部字段来自卸载项注册表的**只读**枚举，
// 或用户手工输入。绝不执行 UninstallString / ExecutablePath。
type DiscoveredApp struct {
	ID              string   `json:"id"`              // 稳定 id（来源 + 归一化 key 派生）
	DisplayName     string   `json:"displayName"`     // 显示名（DisplayName）
	Publisher       string   `json:"publisher"`       // 发行商（Publisher）
	DisplayVersion  string   `json:"displayVersion"`  // 版本（DisplayVersion）
	InstallLocation string   `json:"installLocation"` // 安装目录（InstallLocation，可能为空）
	DisplayIcon     string   `json:"displayIcon"`     // 原始 DisplayIcon 值（仅展示，不执行）
	UninstallString string   `json:"uninstallString"` // 原始卸载命令（仅存档，绝不执行）
	Source          string   `json:"source"`          // 发现来源（见上方常量）
	ExecutablePath  string   `json:"executablePath"`  // 解析出的可执行文件路径（安全解析，不执行）
	CandidatePath   string   `json:"candidatePath"`   // 生成规则时采用的安全候选（目录优先，否则单 exe）
	CandidateIsDir  bool     `json:"candidateIsDir"`  // 候选是否为目录（true→目录\*，false→单文件）
	Confidence      string   `json:"confidence"`      // high | medium | low
	Selectable      bool     `json:"selectable"`      // 是否可安全勾选（候选路径足够具体时为 true）
	Warnings        []string `json:"warnings"`        // 只读提示（路径过宽/无法验证存在/系统组件等）
}

// CustomPathEntry 用户手工添加的白名单目标（目录或单个 exe）。仅用于生成草案，绝不执行。
type CustomPathEntry struct {
	ID    string `json:"id"`    // 前端生成的稳定 id
	Path  string `json:"path"`  // 目录或可执行文件路径
	Kind  string `json:"kind"`  // directory | file
	Label string `json:"label"` // 展示名（可空，缺省用路径末段）
}

// WhitelistSelection 是 BuildWhitelistDraft 的输入（跨 IPC 以 JSON 字符串传入）。
// 前端把用户勾选的 DiscoveredApp 完整对象和自定义路径打包送后端，
// 后端据此纯内存生成草案，不再回读注册表。
type WhitelistSelection struct {
	Apps        []DiscoveredApp   `json:"apps"`        // 用户勾选的应用（完整对象）
	CustomPaths []CustomPathEntry `json:"customPaths"` // 用户手工添加的目录/exe
	PolicyName  string            `json:"policyName"`  // 可选：草案名称
	AdminBypass bool              `json:"adminBypass"` // true 时本地管理员不受策略限制
}

// WhitelistDraft 生成的白名单草案。它包裹一份声明式 Policy（defaultLevel=disallowed）
// 以及生成元信息。**纯内存**，Phase 1a 不持久化、不落地为真实 SRP 规则。
type WhitelistDraft struct {
	Policy          Policy   `json:"policy"`          // 生成的声明式策略（含基础规则 + 用户规则）
	SelectedCount   int      `json:"selectedCount"`   // 纳入的已安装应用数
	CustomPathCount int      `json:"customPathCount"` // 纳入的自定义路径数
	BaseRuleCount   int      `json:"baseRuleCount"`   // 自动附加的基础规则数
	SkippedApps     []string `json:"skippedApps"`     // 被跳过的应用名（不可安全导出候选）
	Notes           []string `json:"notes"`           // 生成说明（安全声明、跳过原因等）
	GeneratedAt     string   `json:"generatedAt"`     // 生成时间，RFC3339
	AdminBypass     bool     `json:"adminBypass"`     // true 时本地管理员不受策略限制
}

// 预检检查状态（PreflightCheck.Status）。
const (
	CheckPass  = "pass"  // 通过
	CheckWarn  = "warn"  // 告警（不阻断，但需关注）
	CheckBlock = "block" // 阻断（若允许应用则必须先解决——但 Phase 1a 本就不允许应用）
)

// 预检/健康检查的稳定编码（surfaced 于 Detail，便于前端与文档引用）。
// 这些是「严格白名单语义」重构（Phase 4）引入的核心判定码。
const (
	// CodeBroadProgramFilesAllow 标记“对整个 Program Files / Program Files (x86)
	// 根目录的通配放行”。这类规则会让**所有已安装程序**默认可运行，等价于取消白名单，
	// 因此在预检中作为阻断项、在健康评分中作为严重扣分。
	CodeBroadProgramFilesAllow = "BROAD_PROGRAMFILES_ALLOW"
	// CodeMainAppNotCovered 标记“当前 XCLing GUI 主程序未被任何 allow 规则覆盖”。
	// 仅作为提示：绝不自动把 GUI 路径加入草案，需用户显式确认后以“自定义精确文件”方式加入。
	CodeMainAppNotCovered = "MAIN_APP_NOT_COVERED"
)

// PreflightCheck 单条预检项。
type PreflightCheck struct {
	ID      string `json:"id"`      // 稳定检查编码
	Title   string `json:"title"`   // 展示标题（中文）
	Status  string `json:"status"`  // pass | warn | block
	Message string `json:"message"` // 结论说明
	Detail  string `json:"detail"`  // 补充细节（命中路径、模拟结果等，可空）
}

// PreflightReport 预检报告。对草案做纯内存校验与路径模拟，输出阻断项/告警/通过项。
// 本报告只说明草案质量，不提供任何应用能力。
type PreflightReport struct {
	Blocked       bool             `json:"blocked"`       // 是否存在 block 级检查
	BlockingCount int              `json:"blockingCount"` // block 数量
	WarningCount  int              `json:"warningCount"`  // warn 数量
	PassCount     int              `json:"passCount"`     // pass 数量
	Checks        []PreflightCheck `json:"checks"`        // 全部检查项
	Summary       string           `json:"summary"`       // 总体结论（中文）
	GeneratedAt   string           `json:"generatedAt"`   // 生成时间，RFC3339
}

// 规则覆盖来源分类（CoverageExplanation.Category）。用于「规则覆盖解释器」清楚标识
// 命中的是哪一类规则：系统基础 / 用户所选软件 / 自定义精确项 / 显式拦截 / 默认级别。
const (
	CoverageCategoryBase     = "base"     // 系统基础放行（base-*，如 Windows 系统目录）
	CoverageCategorySelected = "selected" // 用户在向导勾选的软件（sel-*，非 sel-custom-*）
	CoverageCategoryCustom   = "custom"   // 用户手工添加的目录/精确文件（sel-custom-*）
	CoverageCategoryGuard    = "guard"    // 显式拦截规则（guard-*，disallow）
	CoverageCategoryOther    = "other"    // 其它命中规则（导入/自定义 id）
	CoverageCategoryDefault  = "default"  // 无规则命中，回退默认级别
)

// CoverageExplanation 「规则覆盖解释器」的结果：给定一个可执行文件路径，纯内存模拟它在
// 某份草案/策略下命中哪条规则、最终允许还是拒绝，并把命中规则归类为 base/selected/custom/guard。
// 纯推理，不访问文件系统、不写任何东西。
type CoverageExplanation struct {
	Target        string `json:"target"`        // 归一化后的目标路径
	Decision      string `json:"decision"`      // allow | disallow
	Allowed       bool   `json:"allowed"`       // 便于前端直接判断（decision==allow）
	MatchedRuleID string `json:"matchedRuleId"` // 命中的规则 id（空表示走默认级别）
	MatchedRule   string `json:"matchedRule"`   // 命中的规则展示名
	MatchedPath   string `json:"matchedPath"`   // 命中的路径模式
	Category      string `json:"category"`      // base | selected | custom | guard | other | default
	CategoryLabel string `json:"categoryLabel"` // 分类的中文标签
	DefaultUsed   bool   `json:"defaultUsed"`   // 是否因无规则命中而回退默认级别
	Reason        string `json:"reason"`        // 判定理由（中文）
	GeneratedAt   string `json:"generatedAt"`   // 生成时间，RFC3339
}
