package model

// 本文件定义 Phase 2「拦截事件审计」与「临时放行草案」链路的跨 IPC 数据结构。
//
// 安全边界（Phase 2 关键）：
//   - 审计能力**只读**：仅查询 Windows 事件日志（wevtutil qe，只查询），
//     绝不执行日志中出现的任何路径/命令，绝不修改任何日志或注册表。
//   - 临时放行只能生成**可过期的草案规则**并落到用户数据目录（proposals/），
//     永远不会写入 SRP，也永远没有“应用”入口——UI 无应用按钮。
//   - 审计可用性随本机“事件日志审核策略/SRP 是否开启”而变化；不可用时明确告知。
//
// RPC 序列化约束同前：字段仅 string / int / bool / struct / slice / map；时间用 RFC3339。

// 风险等级（AuditEvent.Risk / Proposal.Risk）。
const (
	RiskLow     = "low"
	RiskMedium  = "medium"
	RiskHigh    = "high"
	RiskUnknown = "unknown"
)

// 审计时间窗口（AuditFilter.Window）——固定枚举，杜绝任意时间输入拼进查询。
const (
	AuditWindow24h = "24h"
	AuditWindow7d  = "7d"
)

// 审计过滤阈值（后端强制校验，前端仅辅助）。
const (
	MinAuditRecords     = 1
	MaxAuditRecords     = 100
	DefaultAuditRecords = 50
	MaxAuditKeywordLen  = 128
)

// 临时放行草案状态与时长阈值。
const (
	ProposalStateDraft   = "draft"
	ProposalStateExpired = "expired"

	MinProposalMinutes     = 5
	MaxProposalMinutes     = 1440 // 24 小时
	DefaultProposalMinutes = 60
)

// ProposalSchemaVersion 临时放行草案文件的架构版本（用于损坏/未来版本友好判定）。
const ProposalSchemaVersion = "1.0"

// AuditEvent 一条（可能的）拦截事件的**只读、已清理**视图。
// 所有字段均来自事件日志 XML 的只读解析；ExecutablePath 仅为字符串，**绝不执行**。
type AuditEvent struct {
	ID             string `json:"id"`             // 稳定 id（channel+eventRecordId 或内容哈希派生）
	Timestamp      string `json:"timestamp"`      // 事件时间，RFC3339（解析失败为空）
	Provider       string `json:"provider"`       // 事件源（Provider Name）
	Channel        string `json:"channel"`        // 日志通道（Application / AppLocker...）
	EventID        int    `json:"eventId"`        // 事件 ID（不同版本可能不同，不做硬编码依赖）
	Level          string `json:"level"`          // 解码后的级别（critical/error/warning/information/verbose）
	ExecutablePath string `json:"executablePath"` // 从事件抽取并清理的可执行文件路径（仅展示，绝不执行）
	User           string `json:"user"`           // 关联用户（SID 或账户名，可空）
	Message        string `json:"message"`        // 渲染后的消息（RenderedXml 提供时）
	RawSummary     string `json:"rawSummary"`     // EventData 字段摘要（清理后的短文本）
	Risk           string `json:"risk"`           // 依据路径位置推断的风险等级
	RiskReasons    []string `json:"riskReasons"`  // 风险判定理由
}

// AuditFilter ListBlockedEvents 的过滤输入（以 JSON 字符串跨 IPC 传入）。
// 后端逐项强制校验：window 必须是固定枚举；max 限定 [1,100]；keyword 限长且不进查询命令。
type AuditFilter struct {
	Window  string `json:"window"`  // 24h | 7d（缺省 24h）
	Keyword string `json:"keyword"` // 关键词（仅在解析后于内存中过滤，绝不拼入 wevtutil 命令）
	Max     int    `json:"max"`     // 最大返回条数 [1,100]
	Channel string `json:"channel"` // 可选：限定某个**已知**通道（非白名单值将被忽略）
}

// AuditCapability GetAuditCapability 的结果：报告审计只读能力是否可用及其边界。
type AuditCapability struct {
	OS             string   `json:"os"`
	IsWindows      bool     `json:"isWindows"`
	Available      bool     `json:"available"` // 是否可用（Windows + 查询工具可用）
	Tool           string   `json:"tool"`      // 查询工具名（wevtutil，只用其 qe 只读子命令）
	Channels       []string `json:"channels"`  // 将被只读查询的固定通道白名单
	Reason         string   `json:"reason"`    // 不可用时的原因
	Notes          []string `json:"notes"`     // 只读/弃用/审核策略相关提示
	ProbedAt       string   `json:"probedAt"`  // 探测时间，RFC3339
	AuditEnabled   bool     `json:"auditEnabled"`   // 进程创建审核策略是否已启用
	AuditAvailable bool     `json:"auditAvailable"` // auditpol 工具是否可用
	AuditDetail    string   `json:"auditDetail"`    // 审核状态详情
}

// ListEventsResult ListBlockedEvents 的结果。
type ListEventsResult struct {
	Events      []AuditEvent `json:"events"`
	Available   bool         `json:"available"`   // 审计能力是否可用
	Window      string       `json:"window"`      // 实际生效窗口
	Max         int          `json:"max"`         // 实际生效上限
	Scanned     int          `json:"scanned"`     // 解析到的事件总数（过滤前）
	Truncated   bool         `json:"truncated"`   // 是否因超过 max 被截断
	Message     string       `json:"message"`     // 面向用户的说明
	Warnings    []string     `json:"warnings"`    // 只读/审核策略/弃用提示
	GeneratedAt string       `json:"generatedAt"` // RFC3339
}

// TemporaryAllowProposal 由单条事件生成的**可过期的临时放行草案规则**。
// 它永远不会被自动写入 SRP：NeverAutoApplies 恒为 true，UI 无“应用”入口。
type TemporaryAllowProposal struct {
	ID              string   `json:"id"`              // 稳定 id（UUID）
	SchemaVersion   string   `json:"schemaVersion"`   // 草案文件架构版本（ProposalSchemaVersion）
	CreatedAt       string   `json:"createdAt"`       // RFC3339
	ExpiresAt       string   `json:"expiresAt"`       // RFC3339，过期时间
	DurationMinutes int      `json:"durationMinutes"` // 有效分钟数 [5,1440]
	State           string   `json:"state"`           // draft | expired
	ExecutablePath  string   `json:"executablePath"`  // 来源事件的可执行文件路径（清理后）
	RuleName        string   `json:"ruleName"`        // 建议规则名
	RulePath        string   `json:"rulePath"`        // 建议放行路径——**默认精确到 exe**，非目录
	IsExactFile     bool     `json:"isExactFile"`     // 是否精确到单文件（默认 true）
	Reason          string   `json:"reason"`          // 生成原因（引用来源事件）
	Risk            string   `json:"risk"`            // 风险等级
	RiskWarnings    []string `json:"riskWarnings"`    // 风险警告（下载/临时/AppData 目录等）
	SourceEventID   string   `json:"sourceEventId"`   // 来源事件 id
	SourceEventRef  string   `json:"sourceEventRef"`  // 来源事件摘要（provider/channel/eventId/time）
	NeverAutoApplies bool    `json:"neverAutoApplies"` // 恒 true：绝不自动生效
	Note            string   `json:"note"`            // “不会自动生效”声明
}

// ProposalSummary 临时放行草案的列表视图（含按当前时间重算的过期状态）。
type ProposalSummary struct {
	ID               string   `json:"id"`
	CreatedAt        string   `json:"createdAt"`
	ExpiresAt        string   `json:"expiresAt"`
	State            string   `json:"state"`            // 存储的状态（draft/expired）
	Expired          bool     `json:"expired"`          // 按当前时间重算是否已过期
	RemainingMinutes int      `json:"remainingMinutes"` // 剩余分钟（过期为 0）
	ExecutablePath   string   `json:"executablePath"`
	RulePath         string   `json:"rulePath"`
	IsExactFile      bool     `json:"isExactFile"`
	Risk             string   `json:"risk"`
	RiskWarnings     []string `json:"riskWarnings"`
	SourceEventRef   string   `json:"sourceEventRef"`
	NeverAutoApplies bool     `json:"neverAutoApplies"`
	Note             string   `json:"note"`
}

// 临时放行草案拒绝编码。
const (
	ProposalRejectBadJSON        = "PROPOSAL_BAD_JSON"
	ProposalRejectNoExecutable   = "PROPOSAL_NO_EXECUTABLE"   // 事件无可用可执行路径，无法生成
	ProposalRejectBadDuration    = "PROPOSAL_BAD_DURATION"    // 时长非法（已被夹取时不会触发）
	ProposalRejectDangerousPath  = "PROPOSAL_DANGEROUS_PATH"  // 路径过宽（等价放行用户可写区）
)
