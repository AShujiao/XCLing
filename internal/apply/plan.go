package apply

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"XCLing/internal/model"
)

const (
	DefaultLevelDisallowed  = 0
	PolicyScopeAllUsers     = 0
	PolicyScopeExceptAdmins = 1
	TransparentEnabledEXE   = 1
)

type Rule struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	Description string `json:"description"`
	Level       int    `json:"level"`
}

type Plan struct {
	PolicyName         string `json:"policyName"`
	DefaultLevel       int    `json:"defaultLevel"`
	PolicyScope        int    `json:"policyScope"`
	TransparentEnabled int    `json:"transparentEnabled"`
	Rules              []Rule `json:"rules"`
	// BlockRules are explicit disallow rules written under the SRP 0 level.
	// They outrank broader allow rules by path specificity and stay in force
	// during a temporary unlock, which is what makes the vendor blocklist work.
	BlockRules []Rule `json:"blockRules,omitempty"`
}

func BuildPlan(draft model.WhitelistDraft, executablePath string) (Plan, error) {
	if draft.Policy.DefaultLevel != model.LevelDisallowed {
		return Plan{}, errors.New("只支持 defaultLevel=disallowed 的白名单草案")
	}
	name := strings.TrimSpace(draft.Policy.Name)
	if name == "" {
		return Plan{}, errors.New("策略名称不能为空")
	}
	rules := make([]Rule, 0, len(draft.Policy.Rules))
	seen := make(map[string]struct{}, len(draft.Policy.Rules))
	selfAllowed := false
	for _, source := range draft.Policy.Rules {
		if !source.Enabled {
			continue
		}
		if source.Action != model.ActionAllow {
			return Plan{}, errors.New("写入模式只支持 allow 路径规则")
		}
		path := canonicalPath(source.Path)
		if path == "" || !isAbsoluteWindowsPath(path) {
			return Plan{}, errors.New("规则包含空路径或非绝对 Windows 路径")
		}
		key := strings.ToLower(path)
		if _, exists := seen[key]; exists {
			return Plan{}, errors.New("规则包含重复路径")
		}
		seen[key] = struct{}{}
		id := deterministicRuleID(source.ID, path)
		rules = append(rules, Rule{ID: id, Path: path, Description: strings.TrimSpace(source.Name), Level: model.SrpLevelUnrestrictedRaw})
		if !strings.HasPrefix(source.ID, "base-") && pathMatches(path, executablePath) {
			selfAllowed = true
		}
	}
	if len(rules) == 0 {
		return Plan{}, errors.New("策略没有可写入的允许规则")
	}
	if strings.TrimSpace(executablePath) == "" || !selfAllowed {
		return Plan{}, fmt.Errorf("%s: %s 主程序未被允许", model.ApplyErrSelfNotAllowed, model.AppName)
	}
	sort.Slice(rules, func(i, j int) bool { return strings.ToLower(rules[i].Path) < strings.ToLower(rules[j].Path) })
	policyScope := PolicyScopeAllUsers
	if draft.AdminBypass {
		policyScope = PolicyScopeExceptAdmins
	}
	return Plan{PolicyName: name, DefaultLevel: DefaultLevelDisallowed, PolicyScope: policyScope, TransparentEnabled: TransparentEnabledEXE, Rules: rules}, nil
}

func Fingerprint(plan Plan) string {
	canonical := plan
	canonical.PolicyName = ""
	canonical.Rules = append([]Rule(nil), plan.Rules...)
	sort.Slice(canonical.Rules, func(i, j int) bool {
		return strings.ToLower(canonical.Rules[i].Path) < strings.ToLower(canonical.Rules[j].Path)
	})
	// nil and empty both collapse to nil here, so a policy that never had block
	// rules keeps the exact fingerprint it had before blocklists existed.
	canonical.BlockRules = append([]Rule(nil), plan.BlockRules...)
	sortBlockRules(canonical.BlockRules)
	data, _ := json.Marshal(canonical)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func RulesDigest(plan Plan) string {
	return Fingerprint(plan)
}

// DetectProtectionState compares the live SRP plan with the plan owned by
// XCLing and distinguishes a locked policy from a temporary unlock.
func DetectProtectionState(actual, expected Plan) string {
	locked := expected
	locked.DefaultLevel = DefaultLevelDisallowed
	if Fingerprint(actual) == Fingerprint(locked) {
		return model.ProtectionStateLocked
	}
	unlocked := expected
	unlocked.DefaultLevel = model.SrpLevelUnrestrictedRaw
	if Fingerprint(actual) == Fingerprint(unlocked) {
		return model.ProtectionStateUnlocked
	}
	return model.ProtectionStateAttention
}

// DetectBlockOnlyProtectionState 判定"仅拦截模式"策略的状态。该模式下
// DefaultLevel 恒为 Unrestricted，锁定/解锁的区别在于 level-0 拦截规则
// 是否写在注册表里：
//   locked   —— 注册表与期望计划（含拦截规则）完全一致，拦截生效；
//   unlocked —— 注册表等于期望计划去掉全部拦截规则（规则保留在恢复记录里）；
//   attention—— 其它任何形态都视为外部修改。
func DetectBlockOnlyProtectionState(actual, expected Plan) string {
	if Fingerprint(actual) == Fingerprint(expected) {
		return model.ProtectionStateLocked
	}
	unlocked := expected
	unlocked.BlockRules = nil
	if Fingerprint(actual) == Fingerprint(unlocked) {
		return model.ProtectionStateUnlocked
	}
	return model.ProtectionStateAttention
}

// AddTrustedRule appends one exact-file or directory allow rule to a managed plan.
func AddTrustedRule(plan Plan, path string, directory bool, description string) (Plan, Rule, error) {
	canonical := canonicalPath(path)
	if directory {
		canonical = strings.TrimRight(canonical, `\`) + `\*`
	}
	if canonical == "" || !isAbsoluteWindowsPath(canonical) {
		return Plan{}, Rule{}, errors.New("可信路径必须是绝对 Windows 路径")
	}
	for _, existing := range plan.Rules {
		if strings.EqualFold(canonicalPath(existing.Path), canonical) {
			return Plan{}, Rule{}, errors.New("该可信路径已存在")
		}
	}
	rule := Rule{
		ID:          deterministicRuleID("trusted:"+strings.ToLower(canonical), canonical),
		Path:        canonical,
		Description: strings.TrimSpace(description),
		Level:       model.SrpLevelUnrestrictedRaw,
	}
	updated := plan
	updated.Rules = append(append([]Rule(nil), plan.Rules...), rule)
	return updated, rule, nil
}

// RemoveTrustedRule removes one rule by its stable ID.
func RemoveTrustedRule(plan Plan, id string) (Plan, error) {
	updated := plan
	updated.Rules = make([]Rule, 0, len(plan.Rules))
	found := false
	for _, rule := range plan.Rules {
		if strings.EqualFold(rule.ID, strings.TrimSpace(id)) {
			found = true
			continue
		}
		updated.Rules = append(updated.Rules, rule)
	}
	if !found {
		return Plan{}, errors.New("未找到要删除的可信规则")
	}
	return updated, nil
}

// PartialMatches reports whether every currently readable rule belongs to the
// expected plan unchanged. Root activation values are intentionally ignored:
// a crash can happen between writing PolicyScope and TransparentEnabled.
func PartialMatches(actual, expected Plan) bool {
	return rulesSubsetOf(actual.Rules, expected.Rules) && rulesSubsetOf(actual.BlockRules, expected.BlockRules)
}

func rulesSubsetOf(actual, expected []Rule) bool {
	want := make(map[string]Rule, len(expected))
	for _, rule := range expected {
		want[strings.ToLower(rule.ID)] = rule
	}
	for _, rule := range actual {
		expectedRule, ok := want[strings.ToLower(rule.ID)]
		if !ok || expectedRule.Path != rule.Path || expectedRule.Description != rule.Description || expectedRule.Level != rule.Level {
			return false
		}
	}
	return true
}

func canonicalPath(value string) string {
	p := strings.TrimSpace(strings.ReplaceAll(value, "/", `\`))
	for strings.Contains(p, `\\`) {
		p = strings.ReplaceAll(p, `\\`, `\`)
	}
	if len(p) >= 2 && p[1] == ':' {
		p = strings.ToUpper(p[:1]) + p[1:]
	}
	return p
}

func isAbsoluteWindowsPath(path string) bool {
	return len(path) >= 3 && ((path[1] == ':' && path[2] == '\\') || strings.HasPrefix(path, `\\`))
}

func pathMatches(pattern, target string) bool {
	p := strings.ToLower(canonicalPath(pattern))
	t := strings.ToLower(canonicalPath(target))
	if strings.HasSuffix(p, `\*`) {
		return strings.HasPrefix(t, strings.TrimSuffix(p, `*`))
	}
	return p == t
}

func deterministicRuleID(sourceID, path string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(sourceID) + "\x00" + path)))
	hexID := hex.EncodeToString(sum[:16])
	return "{" + hexID[:8] + "-" + hexID[8:12] + "-" + hexID[12:16] + "-" + hexID[16:20] + "-" + hexID[20:] + "}"
}
