package service

import (
	"strconv"
	"strings"
	"time"

	"XCLing/internal/model"
)

// 本文件是「白名单向导」的纯函数内核：从用户选择生成草案（BuildDraftFromSelection）、
// 对草案做预检（PreflightDraft）。全部纯内存计算，不碰注册表、不落地为真实 SRP 规则。
//
// 安全边界（Phase 1a）：即便预检全部通过，也**不允许应用**——本阶段根本不写 SRP。

const (
	baseRuleWindows         = "base-windows"
	baseRuleProgramFiles    = "base-program-files"
	baseRuleProgramFilesX86 = "base-program-files-x86"
	selRulePrefix           = "sel-" // 由用户选择/自定义生成的规则统一前缀
	selCustomPrefix         = "sel-custom-"
	preflightProbeFile      = "__preflight_probe__.exe"
)

// programFilesRoots 是「候选/扫描根」——严格白名单语义下它们**不生成任何 allow 规则**：
// 已安装程序必须由用户在向导里逐一勾选、生成“该安装目录\*”或“精确 exe”规则后才被放行。
// 直接对这些根做通配放行会让所有已安装程序默认可运行，等价于取消白名单（见 isBroadProgramFilesAllow）。
var programFilesRoots = []string{
	`c:\program files`,
	`c:\program files (x86)`,
}

// baseWhitelistRules returns the system-managed directories trusted by the
// SoftwarePolicy-style policy. Standard installed programs work without a scan.
func baseWhitelistRules() []model.PathRule {
	return []model.PathRule{
		{
			ID: baseRuleWindows, Name: "Windows 系统目录",
			Path: `C:\Windows\*`, Action: model.ActionAllow, Enabled: true,
			Description: "系统基础（必要权衡）：系统自身可执行文件必须放行，否则无法登录/启动。" +
				"注意其下仍有可写子目录（Temp/Tasks 等），如需可执行文件级严格控制请改用 WDAC/AppLocker。",
		},
		{
			ID: baseRuleProgramFiles, Name: "Program Files",
			Path: `C:\Program Files\*`, Action: model.ActionAllow, Enabled: true,
			Description: "可信安装目录：普通用户默认不能写入，标准安装的软件无需逐个扫描。",
		},
		{
			ID: baseRuleProgramFilesX86, Name: "Program Files (x86)",
			Path: `C:\Program Files (x86)\*`, Action: model.ActionAllow, Enabled: true,
			Description: "可信安装目录：允许 32 位标准安装程序运行。",
		},
	}
}

// isBroadProgramFilesAllow 判断某放行模式是否等价于“放行整个 Program Files /
// Program Files (x86) 根”。命中意味着所有已安装程序都会被默认放行，白名单形同虚设。
// 具体应用子目录（如 C:\Program Files\Acme\*）不在此列——那正是我们要生成的规则。
func isBroadProgramFilesAllow(pattern string) bool {
	n := normalizePath(pattern)
	n = strings.TrimSuffix(n, `\*`)
	n = strings.TrimRight(n, `\`)
	for _, root := range programFilesRoots {
		if n == root {
			return true
		}
	}
	return false
}

// ruleForCandidate 依据候选是目录还是单文件，构造路径模式。
func ruleForCandidate(candidatePath string, isDir bool) string {
	p := strings.TrimRight(strings.TrimSpace(candidatePath), `\`)
	if isDir {
		return p + `\*`
	}
	return p
}

// isDangerousUserAllow 判断某放行模式是否等价于“放行整个用户可写区”（C:\Users、AppData、
// Downloads、Temp、Desktop 等容器）。基础的系统/程序目录不在此列。用于草案生成时拒绝、
// 以及预检时阻断。
func isDangerousUserAllow(pattern string) bool {
	n := normalizePath(pattern)
	n = strings.TrimSuffix(n, `\*`)
	n = strings.TrimRight(n, `\`)
	if n == `c:\users` {
		return true
	}
	segs := strings.Split(n, `\`)
	if len(segs) == 3 && segs[1] == "users" { // c:\users\<name> 配置根
		return true
	}
	leaf := segs[len(segs)-1]
	if len(segs) >= 3 && segs[1] == "users" {
		switch leaf {
		case "appdata", "local", "locallow", "roaming", "programs",
			"downloads", "desktop", "documents", "temp", "tmp", "cache":
			return true
		}
	}
	if leaf == "temp" || leaf == "tmp" {
		return true
	}
	return false
}

// BuildDraftFromSelection 从用户选择生成白名单草案。纯函数。
func BuildDraftFromSelection(sel model.WhitelistSelection) model.WhitelistDraft {
	base := baseWhitelistRules()
	rules := make([]model.PathRule, 0, len(base)+len(sel.Apps)+len(sel.CustomPaths))
	rules = append(rules, base...)

	seenID := map[string]bool{}
	for _, r := range rules {
		seenID[r.ID] = true
	}
	uniqueID := func(want string) string {
		id := want
		for i := 2; seenID[id]; i++ {
			id = want + "-" + strconv.Itoa(i)
		}
		seenID[id] = true
		return id
	}

	selected := 0
	skipped := make([]string, 0)
	notes := []string{
		"草案为“意图”描述：默认禁止执行（disallowed），仅放行下列规则命中的路径。",
		"可信目录模式：Windows、Program Files 和 Program Files (x86) 默认放行；扫描仅用于发现非标准安装位置。",
		"生成草案和预检本身不会写注册表；只有管理员显式确认后，独立 ApplyService 才会尝试受控应用。",
	}

	for _, app := range sel.Apps {
		name := strings.TrimSpace(app.DisplayName)
		if app.CandidatePath == "" {
			skipped = append(skipped, name+"（无安全候选路径）")
			continue
		}
		pattern := ruleForCandidate(app.CandidatePath, app.CandidateIsDir)
		if isDangerousUserAllow(pattern) {
			skipped = append(skipped, name+"（候选路径过宽，拒绝整目录放行）")
			continue
		}
		desc := "来源 " + app.Source
		if app.Publisher != "" {
			desc += " · 发行商 " + app.Publisher
		}
		if app.DisplayVersion != "" {
			desc += " · 版本 " + app.DisplayVersion
		}
		if !app.CandidateIsDir {
			desc += " · 仅放行单个可执行文件"
		}
		rules = append(rules, model.PathRule{
			ID:          uniqueID(selRulePrefix + app.ID),
			Name:        name,
			Path:        pattern,
			Action:      model.ActionAllow,
			Description: desc,
			Enabled:     true,
		})
		selected++
	}

	customCount := 0
	for _, c := range sel.CustomPaths {
		raw := strings.TrimSpace(c.Path)
		if raw == "" {
			continue
		}
		isDir := c.Kind != model.CustomKindFile
		pattern := ruleForCandidate(raw, isDir)
		if isDangerousUserAllow(pattern) {
			skipped = append(skipped, raw+"（自定义路径过宽，拒绝整目录放行）")
			continue
		}
		label := strings.TrimSpace(c.Label)
		if label == "" {
			label = "自定义：" + winTailForLabel(raw)
		}
		kindText := "目录"
		if !isDir {
			kindText = "单个可执行文件"
		}
		rules = append(rules, model.PathRule{
			ID:          uniqueID(selCustomPrefix + c.ID),
			Name:        label,
			Path:        pattern,
			Action:      model.ActionAllow,
			Description: "来源 用户手工添加 · " + kindText,
			Enabled:     true,
		})
		customCount++
	}

	name := strings.TrimSpace(sel.PolicyName)
	if name == "" {
		name = "白名单草案（Phase 1a · 未应用）"
	}

	if len(skipped) > 0 {
		notes = append(notes, "有 "+strconv.Itoa(len(skipped))+" 项因无法安全导出候选或路径过宽而被跳过。")
	}

	return model.WhitelistDraft{
		Policy: model.Policy{
			Version:      "1.0",
			Name:         name,
			Description:  "可信目录软件限制策略：默认禁止用户可写位置，仅放行系统、标准安装目录和用户添加的可信路径。",
			DefaultLevel: model.LevelDisallowed,
			Rules:        rules,
			UpdatedAt:    time.Now().Format(time.RFC3339),
		},
		SelectedCount:   selected,
		CustomPathCount: customCount,
		BaseRuleCount:   len(base),
		SkippedApps:     skipped,
		Notes:           notes,
		GeneratedAt:     time.Now().Format(time.RFC3339),
		AdminBypass:     sel.AdminBypass,
	}
}

func winTailForLabel(p string) string {
	p = strings.TrimRight(strings.ReplaceAll(p, "/", `\`), `\`)
	if i := strings.LastIndexByte(p, '\\'); i >= 0 {
		return p[i+1:]
	}
	return p
}

// probeTargetFor 依据规则模式构造一个“落在该规则内”的探测路径，用于预检模拟覆盖。
func probeTargetFor(pattern string) string {
	if strings.HasSuffix(pattern, `\*`) {
		return strings.TrimSuffix(pattern, `*`) + preflightProbeFile
	}
	return pattern
}

// PreflightDraft 对草案做纯内存预检：校验策略、检查系统基础规则、检查选中项是否落入 allow、
// 检测“对 Program Files 根的过宽放行”（会取消白名单意义）、对下载/临时目录做模拟并要求 disallow、
// 识别对系统路径的 disallow，并检查当前 GUI 主程序覆盖。
// mainExePath 为空表示未知或不检查。
func PreflightDraft(draft model.WhitelistDraft, mainExePath string) model.PreflightReport {
	p := draft.Policy
	report := model.PreflightReport{GeneratedAt: time.Now().Format(time.RFC3339)}
	add := func(id, title, status, message, detail string) {
		report.Checks = append(report.Checks, model.PreflightCheck{
			ID: id, Title: title, Status: status, Message: message, Detail: detail,
		})
	}

	// 1) 策略结构校验。
	vres := ValidatePolicyDoc(p)
	if vres.Valid {
		add("policy_valid", "策略结构校验", model.CheckPass,
			"策略结构合法，共 "+strconv.Itoa(vres.RuleCount)+" 条规则。",
			warningDigest(vres))
	} else {
		add("policy_valid", "策略结构校验", model.CheckBlock,
			"策略存在 "+strconv.Itoa(len(vres.Errors))+" 项结构错误，需先修正。",
			errorDigest(vres))
	}

	// 2) 系统基础规则齐备（严格白名单：只要求 Windows 系统目录被放行）。
	//    Program Files 不再是基础放行——它只是候选根，程序需逐一勾选。
	if SimulatePolicyDoc(p, `C:\Windows\System32\notepad.exe`).Decision == model.ActionAllow {
		add("base_rules", "系统基础放行规则", model.CheckPass,
			"Windows 系统目录已被放行（登录/资源管理器等系统程序可运行）。",
			"严格白名单：Program Files 不作默认放行，程序需在向导中逐一勾选。")
	} else {
		add("base_rules", "系统基础放行规则", model.CheckBlock,
			"缺少对 Windows 系统目录的放行。",
			"默认 disallowed 下若不放行 C:\\Windows，会导致无法登录/系统无法启动。")
	}

	programFilesTrusted := SimulatePolicyDoc(p, `C:\Program Files\XCLingProbe\probe.exe`).Decision == model.ActionAllow &&
		SimulatePolicyDoc(p, `C:\Program Files (x86)\XCLingProbe\probe.exe`).Decision == model.ActionAllow
	if programFilesTrusted {
		add("trusted_program_dirs", "标准安装目录", model.CheckPass,
			"Program Files 和 Program Files (x86) 已作为可信安装目录放行。", "")
	} else {
		add("trusted_program_dirs", "标准安装目录", model.CheckBlock,
			"缺少标准安装目录放行，常规安装的软件可能无法运行。", "")
	}

	// 3) 选中项/自定义路径是否都落入 allow，且是被“具体规则”（sel-*/自定义）覆盖而非仅靠宽泛规则。
	uncovered := make([]string, 0)
	coveredByBroad := make([]string, 0)
	selRuleCount := 0
	for _, r := range p.Rules {
		if !strings.HasPrefix(r.ID, selRulePrefix) {
			continue
		}
		selRuleCount++
		target := probeTargetFor(r.Path)
		sim := SimulatePolicyDoc(p, target)
		if sim.Decision != model.ActionAllow {
			uncovered = append(uncovered, r.Name+"（"+r.Path+"）")
			continue
		}
		// 覆盖它的应当是该 sel 规则自身（或另一条 sel/自定义具体规则），
		// 不能只是靠 Windows 基础规则或过宽的 Program Files 放行“虚假通过”。
		if !strings.HasPrefix(sim.MatchedRuleID, selRulePrefix) {
			coveredByBroad = append(coveredByBroad, r.Name+"（"+r.Path+" ← "+ruleLabel(sim)+"）")
		}
	}
	switch {
	case selRuleCount == 0:
		add("selection_covered", "所选项放行覆盖", model.CheckWarn,
			"草案未包含任何用户所选应用或自定义路径，仅有系统基础规则。",
			"可返回上一步勾选需要放行的软件。")
	case len(uncovered) > 0:
		add("selection_covered", "所选项放行覆盖", model.CheckBlock,
			"有 "+strconv.Itoa(len(uncovered))+" 项未落入 allow（可能被更具体的 disallow 规则遮蔽）。",
			strings.Join(uncovered, "；"))
	case len(coveredByBroad) > 0:
		add("selection_covered", "所选项放行覆盖", model.CheckWarn,
			"有 "+strconv.Itoa(len(coveredByBroad))+" 项并非由其自身的具体规则覆盖，而是被宽泛规则命中，白名单区分度不足。",
			strings.Join(coveredByBroad, "；"))
	default:
		add("selection_covered", "所选项放行覆盖", model.CheckPass,
			"全部 "+strconv.Itoa(selRuleCount)+" 条所选/自定义规则均由其自身的具体规则放行。", "")
	}

	// 4) 当前 XCLing GUI 主程序是否被覆盖。仅提示，绝不自动加入草案。
	//     mainExePath 为空（未知/降级/单测）时跳过此项。
	if mainExe := strings.TrimSpace(mainExePath); mainExe != "" {
		mainSim := SimulatePolicyDoc(p, mainExe)
		if mainSim.Decision == model.ActionAllow {
			add("main_app_covered", "GUI 主程序覆盖", model.CheckPass,
				"当前 "+model.AppName+" 主程序落入 allow 规则，SRP 生效后自身仍可运行。",
				"命中规则："+ruleLabel(mainSim)+"（路径："+mainExe+"）")
		} else {
			add("main_app_covered", "GUI 主程序覆盖", model.CheckWarn,
				"当前 "+model.AppName+" 主程序未被任何 allow 规则覆盖；SRP 生效后可能无法启动本工具。",
				model.CodeMainAppNotCovered+"：若确需放行，请在向导中以“自定义精确文件”手动加入 "+mainExe+
					"（需你显式确认加入草案；"+model.AppName+" 不会自动加入）。")
		}
	}

	// 5) 下载 / 临时目录必须仍被拒绝（模拟）。
	downloadProbe := `C:\Users\preflight\Downloads\` + preflightProbeFile
	tempProbe := `C:\Users\preflight\AppData\Local\Temp\` + preflightProbeFile
	if SimulatePolicyDoc(p, downloadProbe).Decision == model.ActionDisallow {
		add("downloads_blocked", "下载目录拦截", model.CheckPass,
			"下载目录中的可执行文件仍会被拒绝执行。", downloadProbe)
	} else {
		add("downloads_blocked", "下载目录拦截", model.CheckWarn,
			"下载目录中的可执行文件会被放行，可能存在过宽规则。", downloadProbe)
	}
	if SimulatePolicyDoc(p, tempProbe).Decision == model.ActionDisallow {
		add("temp_blocked", "临时目录拦截", model.CheckPass,
			"临时目录中的可执行文件仍会被拒绝执行。", tempProbe)
	} else {
		add("temp_blocked", "临时目录拦截", model.CheckWarn,
			"临时目录中的可执行文件会被放行，常见恶意投放位置未被拦截。", tempProbe)
	}

	// 6) 不得放行整个用户可写区（C:\Users / AppData / Downloads ...）。
	broadAllows := make([]string, 0)
	for _, r := range p.Rules {
		if r.Action == model.ActionAllow && isDangerousUserAllow(r.Path) {
			broadAllows = append(broadAllows, r.Name+"（"+r.Path+"）")
		}
	}
	if len(broadAllows) == 0 {
		add("no_broad_user_allow", "无过宽用户目录放行", model.CheckPass,
			"未发现对整个用户可写区（Users/AppData/Downloads 等）的放行。", "")
	} else {
		add("no_broad_user_allow", "无过宽用户目录放行", model.CheckBlock,
			"存在对用户可写区的过宽放行，会使白名单失去意义。",
			strings.Join(broadAllows, "；"))
	}

	// 7) 识别对系统关键路径的 disallow（白名单草案通常不该出现）。
	sysDisallows := make([]string, 0)
	for _, r := range p.Rules {
		if r.Action != model.ActionDisallow {
			continue
		}
		norm := normalizePath(r.Path)
		for _, cp := range criticalPrefixes {
			if strings.HasPrefix(norm, cp) {
				sysDisallows = append(sysDisallows, r.Name+"（"+r.Path+"）")
				break
			}
		}
	}
	if len(sysDisallows) == 0 {
		add("system_path_disallow", "系统路径未被拒绝", model.CheckPass,
			"未发现对系统关键路径的 disallow 规则。", "")
	} else {
		add("system_path_disallow", "系统路径未被拒绝", model.CheckWarn,
			"存在对系统关键路径的 disallow，可能导致系统无法启动/登录。",
			strings.Join(sysDisallows, "；"))
	}

	// 汇总。
	for _, c := range report.Checks {
		switch c.Status {
		case model.CheckBlock:
			report.BlockingCount++
		case model.CheckWarn:
			report.WarningCount++
		case model.CheckPass:
			report.PassCount++
		}
	}
	report.Blocked = report.BlockingCount > 0
	if report.Blocked {
		report.Summary = "预检发现 " + strconv.Itoa(report.BlockingCount) + " 项阻断、" +
			strconv.Itoa(report.WarningCount) + " 项告警，当前草案不能应用。"
	} else if report.WarningCount > 0 {
		report.Summary = "预检无阻断项，但有 " + strconv.Itoa(report.WarningCount) +
			" 项告警需关注；仍需通过管理员环境检查和显式确认才能应用。"
	} else {
		report.Summary = "预检全部通过；仍需通过管理员环境检查和显式确认才能应用 SRP。"
	}
	return report
}

func ruleLabel(res model.SimulationResult) string {
	if res.MatchedRule == "" {
		return "（默认级别）"
	}
	return res.MatchedRule + "（" + res.MatchedPattern + "）"
}

// ExplainCoverage 「规则覆盖解释器」纯函数内核：给定一份策略与一个可执行文件路径，
// 模拟其最终判定并把命中的规则归类为 base/selected/custom/guard/other/default。
// 纯推理，不访问文件系统、不写任何东西。
func ExplainCoverage(p model.Policy, target string) model.CoverageExplanation {
	sim := SimulatePolicyDoc(p, target)
	category := categoryForRuleID(sim.MatchedRuleID, sim.DefaultUsed)
	return model.CoverageExplanation{
		Target:        sim.Path,
		Decision:      sim.Decision,
		Allowed:       sim.Decision == model.ActionAllow,
		MatchedRuleID: sim.MatchedRuleID,
		MatchedRule:   sim.MatchedRule,
		MatchedPath:   sim.MatchedPattern,
		Category:      category,
		CategoryLabel: coverageCategoryLabel(category),
		DefaultUsed:   sim.DefaultUsed,
		Reason:        sim.Reason,
	}
}

// categoryForRuleID 依据命中规则的 id 前缀归类。顺序敏感：sel-custom- 必须先于 sel-。
func categoryForRuleID(ruleID string, defaultUsed bool) string {
	if defaultUsed || ruleID == "" {
		return model.CoverageCategoryDefault
	}
	switch {
	case strings.HasPrefix(ruleID, selCustomPrefix):
		return model.CoverageCategoryCustom
	case strings.HasPrefix(ruleID, selRulePrefix):
		return model.CoverageCategorySelected
	case strings.HasPrefix(ruleID, "base-"):
		return model.CoverageCategoryBase
	case strings.HasPrefix(ruleID, "guard-"):
		return model.CoverageCategoryGuard
	default:
		return model.CoverageCategoryOther
	}
}

func coverageCategoryLabel(category string) string {
	switch category {
	case model.CoverageCategoryBase:
		return "系统基础放行"
	case model.CoverageCategorySelected:
		return "所选软件"
	case model.CoverageCategoryCustom:
		return "自定义路径"
	case model.CoverageCategoryGuard:
		return "显式拦截"
	case model.CoverageCategoryDefault:
		return "默认级别"
	default:
		return "其它规则"
	}
}

func warningDigest(res model.ValidationResult) string {
	if len(res.Warnings) == 0 {
		return ""
	}
	parts := make([]string, 0, len(res.Warnings))
	for _, w := range res.Warnings {
		parts = append(parts, w.Message)
	}
	return "校验告警：" + strings.Join(parts, "；")
}

func errorDigest(res model.ValidationResult) string {
	parts := make([]string, 0, len(res.Errors))
	for _, e := range res.Errors {
		parts = append(parts, e.Message)
	}
	return strings.Join(parts, "；")
}
