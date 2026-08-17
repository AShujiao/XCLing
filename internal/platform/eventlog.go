package platform

import "strings"

// 本文件定义「事件日志只读查询」的**跨平台类型与纯参数构造**。
//
// 安全关键：本工具查询事件日志**只**使用 wevtutil 的 `qe`（query-events）只读子命令，
// 绝不使用任何会**修改/清空/卸载**日志的子命令（如 sl / cl / um / im / ep / epl）。
// 进入命令行的参数只有：固定通道名、固定 Provider 名（白名单常量）、以及经校验的整数
// （最大条数、时间窗毫秒）。**绝不**把用户关键词或任意字符串拼进查询——关键词过滤只在
// 解析后于内存中进行（见 internal/audit）。

const (
	// EventLogTool 是唯一使用的事件日志工具。
	EventLogTool = "wevtutil"
	// EventLogVerbQuery 是唯一使用的子命令：query-events（只读）。
	// 严禁改为 sl(set-log) / cl(clear-log) / um(uninstall-manifest) / im(install-manifest)
	// / ep(enum-publishers) 之外任何具有写/清除语义的子命令。
	EventLogVerbQuery = "qe"
)

// EventQuery 是一次只读事件查询的参数。Channel/ProviderName 必须来自调用方的固定白名单，
// 绝不接受任意用户输入；MaxRecords/WithinMillis 为已校验的整数。
type EventQuery struct {
	Channel      string // 事件通道（白名单常量）
	ProviderName string // 可选 Provider 过滤（白名单常量，空表示不过滤）
	MaxRecords   int    // 最大返回条数（将被夹取到 [1,100]）
	WithinMillis int64  // 只取距今该毫秒数内的事件（<=0 表示不加时间过滤）
}

// EventLogCapability 报告事件日志只读查询能力（原始探测结果，域层再包装为 model.AuditCapability）。
type EventLogCapability struct {
	Available bool   // 是否可用（Windows 且工具可用）
	Tool      string // 工具名（wevtutil）
	Reason    string // 不可用原因
}

// safeQueryValue 只允许安全字符出现在 XPath 字面量里（字母/数字/空格/点/横杠/下划线/斜杠）。
// Provider/Channel 均为白名单常量，此校验是纵深防御，杜绝任何引号/注入字符进入查询。
func safeQueryValue(s string) bool {
	if s == "" {
		return true
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == ' ', r == '.', r == '-', r == '_', r == '/', r == '\\':
		default:
			return false
		}
	}
	return true
}

func clampMaxRecords(n int) int {
	if n < 1 {
		return 1
	}
	if n > 100 {
		return 100
	}
	return n
}

// buildEventXPath 构造只读查询用的 XPath。providerName 非法时降级为“不按 Provider 过滤”。
func buildEventXPath(providerName string, withinMillis int64) string {
	var conds []string
	if providerName != "" && safeQueryValue(providerName) {
		conds = append(conds, "Provider[@Name='"+providerName+"']")
	}
	if withinMillis > 0 {
		conds = append(conds, "TimeCreated[timediff(@SystemTime) <= "+itoa64(withinMillis)+"]")
	}
	if len(conds) == 0 {
		return "*"
	}
	return "*[System[" + strings.Join(conds, " and ") + "]]"
}

// buildWevtutilArgs 构造 wevtutil 只读查询参数。**纯函数**，便于单测验证其永远只用 `qe`
// 且不含任何用户关键词。
func buildWevtutilArgs(q EventQuery) []string {
	xpath := buildEventXPath(q.ProviderName, q.WithinMillis)
	return []string{
		EventLogVerbQuery,          // 只读子命令
		q.Channel,                  // 通道（白名单常量）
		"/q:" + xpath,              // XPath（仅含白名单常量与整数）
		"/c:" + itoa(clampMaxRecords(q.MaxRecords)), // 最大条数（整数）
		"/rd:true",                 // 从最新事件开始
		"/f:RenderedXml",           // 渲染格式（含本地化 Message），仍为只读
	}
}

func itoa(n int) string {
	return itoa64(int64(n))
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
