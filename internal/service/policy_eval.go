package service

import (
	"sort"
	"strings"

	"XCLing/internal/model"
)

var criticalPrefixes = []string{
	`c:\windows`,
}

func normalizePath(value string) string {
	p := strings.TrimSpace(strings.ReplaceAll(value, "/", `\`))
	for strings.Contains(p, `\\`) {
		p = strings.ReplaceAll(p, `\\`, `\`)
	}
	if len(p) >= 2 && p[1] == ':' {
		p = strings.ToUpper(p[:1]) + p[1:]
	}
	return p
}

func wildcardMatch(pattern, s string) bool {
	pIdx, sIdx := 0, 0
	starIdx, matchIdx := -1, 0
	for sIdx < len(s) {
		if pIdx < len(pattern) && (pattern[pIdx] == '?' || pattern[pIdx] == s[sIdx]) {
			pIdx++
			sIdx++
		} else if pIdx < len(pattern) && pattern[pIdx] == '*' {
			starIdx = pIdx
			matchIdx = sIdx
			pIdx++
		} else if starIdx != -1 {
			pIdx = starIdx + 1
			matchIdx++
			sIdx = matchIdx
		} else {
			return false
		}
	}
	for pIdx < len(pattern) && pattern[pIdx] == '*' {
		pIdx++
	}
	return pIdx == len(pattern)
}

func patternSpecificity(pattern string) int {
	n := 0
	for i := 0; i < len(pattern); i++ {
		if pattern[i] != '*' && pattern[i] != '?' {
			n++
		}
	}
	return n
}

func isBroadPattern(norm string) bool {
	switch norm {
	case "", "*", `*.*`, `\*`, `*\*`:
		return true
	}
	if len(norm) >= 3 && norm[1] == ':' && strings.HasSuffix(norm, `:\*`) {
		return true
	}
	return false
}

func ValidatePolicyDoc(p model.Policy) model.ValidationResult {
	res := model.ValidationResult{
		Valid:     true,
		RuleCount: len(p.Rules),
	}

	if strings.TrimSpace(p.Version) == "" {
		res.Errors = append(res.Errors, model.ValidationIssue{
			Field: "version", Code: "VERSION_REQUIRED", Message: "策略缺少 version 字段。",
		})
	}
	switch p.DefaultLevel {
	case model.LevelDisallowed, model.LevelUnrestricted:
	default:
		res.Errors = append(res.Errors, model.ValidationIssue{
			Field: "defaultLevel", Code: "DEFAULT_LEVEL_INVALID",
			Message: "defaultLevel 只能是 disallowed 或 unrestricted，当前为：" + p.DefaultLevel,
		})
	}

	seenID := map[string]bool{}
	seenPath := map[string]string{}
	for _, r := range p.Rules {
		if strings.TrimSpace(r.ID) == "" {
			res.Errors = append(res.Errors, model.ValidationIssue{
				Field: "id", Code: "RULE_ID_REQUIRED", Message: "存在缺少 id 的规则：" + r.Name,
			})
		} else if seenID[r.ID] {
			res.Errors = append(res.Errors, model.ValidationIssue{
				RuleID: r.ID, Field: "id", Code: "RULE_ID_DUPLICATE",
				Message: "规则 id 重复：" + r.ID,
			})
		} else {
			seenID[r.ID] = true
		}

		if strings.TrimSpace(r.Path) == "" {
			res.Errors = append(res.Errors, model.ValidationIssue{
				RuleID: r.ID, Field: "path", Code: "PATH_REQUIRED", Message: "规则 path 不能为空。",
			})
		} else if strings.ContainsRune(r.Path, '\x00') {
			res.Errors = append(res.Errors, model.ValidationIssue{
				RuleID: r.ID, Field: "path", Code: "PATH_INVALID", Message: "规则 path 包含非法空字符。",
			})
		}

		switch r.Action {
		case model.ActionAllow, model.ActionDisallow:
		default:
			res.Errors = append(res.Errors, model.ValidationIssue{
				RuleID: r.ID, Field: "action", Code: "ACTION_INVALID",
				Message: "action 只能是 allow 或 disallow，当前为：" + r.Action,
			})
		}

		norm := normalizePath(r.Path)
		if norm != "" {
			if first, ok := seenPath[norm]; ok {
				res.Warnings = append(res.Warnings, model.ValidationIssue{
					RuleID: r.ID, Field: "path", Code: "PATH_DUPLICATE",
					Message: "路径与规则 " + first + " 重复，可能产生冗余或冲突：" + r.Path,
				})
			} else {
				seenPath[norm] = r.ID
			}

			if isBroadPattern(norm) {
				res.Warnings = append(res.Warnings, model.ValidationIssue{
					RuleID: r.ID, Field: "path", Code: "PATH_TOO_BROAD",
					Message: "路径模式过宽，几乎匹配所有文件，可能使白/黑名单失去意义：" + r.Path,
				})
			}
			if r.Action == model.ActionDisallow {
				for _, cp := range criticalPrefixes {
					if strings.HasPrefix(norm, cp) {
						res.Warnings = append(res.Warnings, model.ValidationIssue{
							RuleID: r.ID, Field: "path", Code: "DISALLOW_SYSTEM_PATH",
							Message: "对系统关键路径执行 disallow 可能导致系统无法启动/登录：" + r.Path,
						})
						break
					}
				}
			}
		}
	}

	if len(res.Errors) > 0 {
		res.Valid = false
	}
	return res
}

func SimulatePolicyDoc(p model.Policy, target string) model.SimulationResult {
	normTarget := normalizePath(target)
	res := model.SimulationResult{Path: normTarget}

	type cand struct {
		rule model.PathRule
		spec int
	}
	var matches []cand
	for _, r := range p.Rules {
		if !r.Enabled {
			continue
		}
		np := normalizePath(r.Path)
		if np == "" {
			continue
		}
		if wildcardMatch(np, normTarget) {
			matches = append(matches, cand{rule: r, spec: patternSpecificity(np)})
		}
	}

	if len(matches) == 0 {
		res.DefaultUsed = true
		if p.DefaultLevel == model.LevelUnrestricted {
			res.Decision = model.ActionAllow
			res.Reason = "无规则命中，按默认级别 unrestricted 放行。"
		} else {
			res.Decision = model.ActionDisallow
			res.Reason = "无规则命中，按默认级别 disallowed 拒绝。"
		}
		return res
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].spec != matches[j].spec {
			return matches[i].spec > matches[j].spec
		}
		return matches[i].rule.Action == model.ActionDisallow && matches[j].rule.Action != model.ActionDisallow
	})

	win := matches[0]
	res.Decision = win.rule.Action
	res.MatchedRuleID = win.rule.ID
	res.MatchedRule = win.rule.Name
	res.MatchedPattern = win.rule.Path
	res.Specificity = win.spec
	if win.rule.Action == model.ActionAllow {
		res.Reason = "命中最具体的 allow 规则「" + win.rule.Name + "」，放行。"
	} else {
		res.Reason = "命中最具体的 disallow 规则「" + win.rule.Name + "」，拒绝。"
	}
	return res
}
