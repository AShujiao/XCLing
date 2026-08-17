package apply

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"XCLing/internal/model"
)

// 本文件实现「厂商拦截名单」的纯函数内核：模式归一化、安全校验、增删。
//
// 拦截规则写入 SRP 的 0(Disallowed) 级别。SRP 判定「最具体路径优先」，与级别无关，
// 因此 `C:\Program Files\360\*`(disallow) 会压过 `C:\Program Files\*`(allow)，
// 且不依赖 DefaultLevel —— 白名单临时解锁时拦截依然生效。

// protectedFileNames 是绝不允许写成裸文件名拦截规则的系统可执行文件。
// 裸文件名规则在**任意目录**生效，误拦这些会导致无法登录、无法进入桌面或无法恢复策略。
var protectedFileNames = map[string]bool{
	"explorer.exe": true, "svchost.exe": true, "winlogon.exe": true, "csrss.exe": true,
	"lsass.exe": true, "services.exe": true, "smss.exe": true, "wininit.exe": true,
	"userinit.exe": true, "dwm.exe": true, "taskhost.exe": true, "taskhostw.exe": true,
	"rundll32.exe": true, "regedit.exe": true, "cmd.exe": true, "powershell.exe": true,
	"mmc.exe": true, "control.exe": true, "msiexec.exe": true, "dllhost.exe": true,
	"conhost.exe": true, "sihost.exe": true, "ctfmon.exe": true, "spoolsv.exe": true,
	"logonui.exe": true, "fontdrvhost.exe": true, "runtimebroker.exe": true,
	"searchindexer.exe": true, "wmiprvse.exe": true, "gpupdate.exe": true,
	"secpol.msc": true, "gpedit.msc": true,
}

// protectedPrefixes 是不允许被目录拦截规则覆盖的系统路径前缀（归一化小写、无尾 \*）。
var protectedPrefixes = []string{
	`c:\windows`,
}

// exactProtectedDirs 是不允许被整体拦截的目录（放行根本身，子目录可以拦）。
var exactProtectedDirs = []string{
	`c:\`, `c:\program files`, `c:\program files (x86)`, `c:\programdata`, `c:\users`,
}

// NormalizeBlockPattern 把用户输入归一化成可写入 SRP 的拦截模式，并做安全校验。
// selfExecutable 为当前主程序路径，用于拒绝「拦截自己」。
func NormalizeBlockPattern(raw, kind, selfExecutable string) (string, string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", "", errors.New(model.BlocklistErrInvalidPattern + ": 拦截模式不能为空")
	}
	resolved := kind
	if resolved == "" {
		// 未指定类型时按形态推断：含分隔符即路径，否则按裸文件名。
		if strings.ContainsAny(value, `\/`) {
			resolved = model.BlockKindDirectory
		} else {
			resolved = model.BlockKindFileName
		}
	}
	switch resolved {
	case model.BlockKindFileName:
		pattern, err := normalizeBlockFileName(value, selfExecutable)
		return pattern, model.BlockKindFileName, err
	case model.BlockKindDirectory:
		pattern, err := normalizeBlockDirectory(value, selfExecutable)
		return pattern, model.BlockKindDirectory, err
	case model.BlockKindFile:
		pattern, err := normalizeBlockFile(value, selfExecutable)
		return pattern, model.BlockKindFile, err
	default:
		return "", "", fmt.Errorf("%s: 未知拦截类型 %q", model.BlocklistErrInvalidPattern, kind)
	}
}

func normalizeBlockFileName(value, selfExecutable string) (string, error) {
	name := strings.TrimSpace(value)
	if strings.ContainsAny(name, `\/:`) {
		return "", errors.New(model.BlocklistErrInvalidPattern + ": 文件名规则不能包含路径分隔符或盘符")
	}
	if strings.ContainsAny(name, `*?`) {
		// `*.exe` 这类通配会拦掉全盘同扩展名程序，等同于关机。
		return "", errors.New(model.BlocklistErrInvalidPattern + ": 文件名规则不支持通配符，请填写完整文件名")
	}
	if filepath.Ext(name) == "" {
		return "", errors.New(model.BlocklistErrInvalidPattern + ": 请填写带扩展名的完整文件名，例如 360se.exe")
	}
	lower := strings.ToLower(name)
	if protectedFileNames[lower] {
		return "", fmt.Errorf("%s: %s 是系统关键程序，拦截后可能无法进入桌面", model.BlocklistErrProtected, name)
	}
	if self := strings.TrimSpace(selfExecutable); self != "" && strings.EqualFold(filepath.Base(self), name) {
		return "", fmt.Errorf("%s: 不能拦截 %s 自身", model.BlocklistErrProtected, model.AppName)
	}
	return name, nil
}

func normalizeBlockDirectory(value, selfExecutable string) (string, error) {
	dir := canonicalPath(strings.TrimSuffix(strings.TrimSpace(value), `\*`))
	dir = strings.TrimRight(dir, `\`)
	// 盘符根 `C:` 去掉尾斜杠后会退化，单独还原。
	if len(dir) == 2 && dir[1] == ':' {
		dir += `\`
	}
	if !isAbsoluteWindowsPath(strings.TrimSuffix(dir, `\`) + `\x`) {
		return "", errors.New(model.BlocklistErrInvalidPattern + ": 目录规则必须是绝对 Windows 路径")
	}
	if err := guardProtectedDirectory(dir, selfExecutable); err != nil {
		return "", err
	}
	return strings.TrimRight(dir, `\`) + `\*`, nil
}

func normalizeBlockFile(value, selfExecutable string) (string, error) {
	file := canonicalPath(value)
	if !isAbsoluteWindowsPath(file) {
		return "", errors.New(model.BlocklistErrInvalidPattern + ": 文件规则必须是绝对 Windows 路径")
	}
	if strings.HasSuffix(file, `\*`) {
		return "", errors.New(model.BlocklistErrInvalidPattern + ": 精确文件规则不能以通配符结尾")
	}
	lower := strings.ToLower(file)
	for _, prefix := range protectedPrefixes {
		if lower == prefix || strings.HasPrefix(lower, prefix+`\`) {
			return "", fmt.Errorf("%s: 不能拦截系统目录下的 %s", model.BlocklistErrProtected, file)
		}
	}
	if self := strings.TrimSpace(selfExecutable); self != "" && strings.EqualFold(canonicalPath(self), file) {
		return "", fmt.Errorf("%s: 不能拦截 %s 自身", model.BlocklistErrProtected, model.AppName)
	}
	return file, nil
}

func guardProtectedDirectory(dir, selfExecutable string) error {
	lower := strings.ToLower(strings.TrimRight(dir, `\`))
	if lower == "" {
		return errors.New(model.BlocklistErrInvalidPattern + ": 目录规则不能为空")
	}
	for _, exact := range exactProtectedDirs {
		if lower == strings.TrimRight(exact, `\`) || lower+`\` == exact {
			return fmt.Errorf("%s: 不能整体拦截 %s，请指定到具体的厂商子目录", model.BlocklistErrProtected, dir)
		}
	}
	for _, prefix := range protectedPrefixes {
		if lower == prefix || strings.HasPrefix(lower, prefix+`\`) {
			return fmt.Errorf("%s: 不能拦截系统目录 %s", model.BlocklistErrProtected, dir)
		}
	}
	if self := strings.TrimSpace(selfExecutable); self != "" {
		selfDir := strings.ToLower(strings.TrimRight(canonicalPath(filepath.Dir(self)), `\`))
		if selfDir != "" && (selfDir == lower || strings.HasPrefix(selfDir, lower+`\`)) {
			return fmt.Errorf("%s: 该目录包含 %s 自身，拦截后将无法恢复策略", model.BlocklistErrProtected, model.AppName)
		}
	}
	return nil
}

// BlockRuleID 由模式派生稳定的 SRP 子键名。同一模式永远得到同一 id，
// 因此重复添加可被检出，删除也不依赖外部记录。
func BlockRuleID(pattern string) string {
	return deterministicRuleID("block:"+strings.ToLower(pattern), strings.ToLower(pattern))
}

// NewBlockRule 构造一条已归一化的拦截规则。
func NewBlockRule(pattern, description string) Rule {
	return Rule{
		ID:          BlockRuleID(pattern),
		Path:        pattern,
		Description: strings.TrimSpace(description),
		Level:       model.SrpLevelDisallowedRaw,
	}
}

// AddBlockRules 把一批已归一化的模式加入计划的拦截规则，返回新计划与实际新增数。
// 已存在的模式被静默跳过（按厂商包整组添加时这是期望行为）。
func AddBlockRules(plan Plan, patterns []string, descriptions map[string]string) (Plan, int) {
	existing := make(map[string]bool, len(plan.BlockRules))
	for _, rule := range plan.BlockRules {
		existing[strings.ToLower(rule.Path)] = true
	}
	updated := plan
	updated.BlockRules = append([]Rule(nil), plan.BlockRules...)
	added := 0
	for _, pattern := range patterns {
		key := strings.ToLower(pattern)
		if pattern == "" || existing[key] {
			continue
		}
		existing[key] = true
		updated.BlockRules = append(updated.BlockRules, NewBlockRule(pattern, descriptions[pattern]))
		added++
	}
	sortBlockRules(updated.BlockRules)
	return updated, added
}

// RemoveBlockRules removes every rule whose ID appears in ids.
func RemoveBlockRules(plan Plan, ids []string) (Plan, int) {
	drop := make(map[string]bool, len(ids))
	for _, id := range ids {
		drop[strings.ToLower(strings.TrimSpace(id))] = true
	}
	updated := plan
	updated.BlockRules = make([]Rule, 0, len(plan.BlockRules))
	removed := 0
	for _, rule := range plan.BlockRules {
		if drop[strings.ToLower(rule.ID)] {
			removed++
			continue
		}
		updated.BlockRules = append(updated.BlockRules, rule)
	}
	if len(updated.BlockRules) == 0 {
		updated.BlockRules = nil
	}
	return updated, removed
}

func sortBlockRules(rules []Rule) {
	sort.Slice(rules, func(i, j int) bool { return strings.ToLower(rules[i].Path) < strings.ToLower(rules[j].Path) })
}

// BlockedPatterns returns every pattern currently blocked, lowercased.
func BlockedPatterns(plan Plan) map[string]bool {
	result := make(map[string]bool, len(plan.BlockRules))
	for _, rule := range plan.BlockRules {
		result[strings.ToLower(rule.Path)] = true
	}
	return result
}
