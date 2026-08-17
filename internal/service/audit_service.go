package service

import (
	"encoding/json"
	"errors"
	"runtime"
	"strconv"
	"strings"
	"time"

	"XCLing/internal/audit"
	"XCLing/internal/model"
	"XCLing/internal/platform"
)

// AuditService 暴露给前端的「拦截事件审计」只读能力。
//
// 安全边界：
//   - 事件读取**只读**：仅通过 platform.QueryEventLog（wevtutil qe）查询固定通道白名单；
//     关键词过滤只在解析后于内存进行，绝不拼进查询命令；
//   - **绝不**执行事件中出现的任何路径/命令，**绝不**修改日志或注册表；
//   - 无 Apply/Enable/Disable/Restore 等 SRP 写方法。
type AuditService struct {
	sources []audit.SourceSpec

	now         func() time.Time
	isWindows   func() bool
	capability  func() platform.EventLogCapability
	queryEvents func(q platform.EventQuery) (string, error)
}

// NewAuditService 以生产默认依赖构造。
func NewAuditService() *AuditService {
	return &AuditService{
		sources:     audit.DefaultSources(),
		now:         time.Now,
		isWindows:   func() bool { return runtime.GOOS == "windows" },
		capability:  platform.EventLogAvailable,
		queryEvents: platform.QueryEventLog,
	}
}

// auditNotes 返回始终展示的只读/弃用/审核策略相关提示。
func auditNotes() []string {
	return []string{
		"审计为纯只读：仅查询事件日志（wevtutil qe），绝不执行日志中的任何路径/命令，绝不修改日志或注册表。",
		"能否看到拦截事件取决于本机的事件日志审核策略与 SRP/AppLocker 是否启用并记录；未开启时列表可能为空，这不代表没有风险。",
		"SRP 已被微软标记为弃用（deprecated），但功能仍然存在、仍可正常使用；弃用仅表示不再积极开发，新部署建议优先使用 WDAC / AppLocker。",
	}
}

// GetAuditCapability 探测审计只读能力（是否 Windows、查询工具是否可用）。无副作用。
func (s *AuditService) GetAuditCapability() (model.AuditCapability, error) {
	isWin := s.isWindows()
	cap := s.capability()
	auditStatus := platform.GetAuditPolicyStatus()
	res := model.AuditCapability{
		OS:             runtime.GOOS,
		IsWindows:      isWin,
		Available:      isWin && cap.Available,
		Tool:           cap.Tool,
		Channels:       audit.KnownChannels(),
		Notes:          auditNotes(),
		ProbedAt:       s.now().UTC().Format(time.RFC3339),
		AuditEnabled:   auditStatus.Enabled,
		AuditAvailable: auditStatus.Available,
		AuditDetail:    auditStatus.Detail,
	}
	if !isWin {
		res.Reason = "当前平台非 Windows：无 Windows 事件日志，审计能力不可用（界面数据仅供演示）。"
	} else if !cap.Available {
		res.Reason = cap.Reason
	} else if auditStatus.ErrorDetail != "" {
		res.Notes = append(res.Notes, "审核策略状态检查失败："+auditStatus.ErrorDetail)
	} else if !auditStatus.Enabled && auditStatus.Available {
		res.Notes = append(res.Notes, "提示：当前Application事件日志被禁用，拦截事件无法被记录。可点击页面上的「启用日志」按钮自动配置（需要管理员权限）。")
	}
	return res, nil
}

// EnableAuditPolicy 自动启用 SRP 拦截事件所需的 Windows 审核策略（需要管理员权限）。
func (s *AuditService) EnableAuditPolicy() error {
	if !s.isWindows() {
		return errors.New("仅 Windows 平台支持")
	}
	return platform.EnableAuditPolicy()
}

// ListBlockedEvents 只读查询并解析（可能的）拦截事件。filterJSON 为 model.AuditFilter 的 JSON。
// 过滤条件经后端强制校验；关键词仅在内存过滤，绝不进入查询命令。
func (s *AuditService) ListBlockedEvents(filterJSON string) (model.ListEventsResult, error) {
	var f model.AuditFilter
	if strings.TrimSpace(filterJSON) != "" {
		if err := json.Unmarshal([]byte(strings.TrimSpace(filterJSON)), &f); err != nil {
			return model.ListEventsResult{}, err
		}
	}
	audit.ValidateFilter(&f)

	res := model.ListEventsResult{
		Window:      f.Window,
		Max:         f.Max,
		Warnings:    auditNotes(),
		GeneratedAt: s.now().UTC().Format(time.RFC3339),
	}

	isWin := s.isWindows()
	cap := s.capability()
	if !isWin || !cap.Available {
		res.Available = false
		if !isWin {
			res.Message = "当前平台非 Windows：无法读取事件日志。"
		} else {
			res.Message = "事件日志查询能力不可用：" + cap.Reason
		}
		return res, nil
	}
	res.Available = true

	withinMs := audit.WindowMillis(f.Window)
	all := make([]model.AuditEvent, 0, 32)
	for _, src := range s.sources {
		if f.Channel != "" && src.Channel != f.Channel {
			continue
		}
		raw, err := s.queryEvents(platform.EventQuery{
			Channel:      src.Channel,
			ProviderName: src.ProviderName,
			MaxRecords:   f.Max,
			WithinMillis: withinMs,
		})
		if err != nil {
			res.Warnings = append(res.Warnings, "跳过来源「"+src.Label+"」："+err.Error())
			continue
		}
		all = append(all, audit.ParseEvents(raw, src)...)
	}

	res.Scanned = len(all)
	events, truncated := audit.FilterAndSort(all, f.Keyword, f.Max)
	res.Events = events
	res.Truncated = truncated
	if len(events) == 0 {
		res.Message = "在所选时间窗内未解析到相关事件。注意：这可能是因为未启用相应的审核/日志记录，而非确实没有拦截。"
	} else {
		msg := "共呈现 " + strconv.Itoa(len(events)) + " 条事件（扫描 " + strconv.Itoa(res.Scanned) + " 条）。"
		if truncated {
			msg += "已按上限截断。"
		}
		res.Message = msg
	}
	return res, nil
}
