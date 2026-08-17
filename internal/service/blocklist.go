package service

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	pgapply "XCLing/internal/apply"
	"XCLing/internal/model"
	"XCLing/internal/platform"
	"XCLing/internal/store"
)

// BlocklistService 管理显式拦截规则（level-0 disallow）。
// 这些规则比白名单 allow 规则更具体（SRP 路径优先），因此：
//  1. 即使安装目录位于 Program Files 可信区内，拦截依然生效；
//  2. 临时解锁白名单时（DefaultLevel=Unrestricted），拦截规则**依然生效**。
type BlocklistService struct {
	mu  sync.Mutex
	now func() time.Time
}

func NewBlocklistService() *BlocklistService { return &BlocklistService{now: time.Now} }

// GetBlocklistStatus 返回拦截名单页面所需的完整状态。
func (s *BlocklistService) GetBlocklistStatus() (model.BlocklistStatus, error) {
	status := model.BlocklistStatus{
		CheckedAt: s.now().UTC().Format(time.RFC3339),
		Vendors:   model.VendorPresets(),
	}
	if runtime.GOOS != "windows" {
		status.Reason = "黑名单仅支持 Windows"
		return status, nil
	}
	status.Available = true
	status.IsAdmin = platform.IsAdmin()

	// 读取当前保护状态（需要知道 SRP 是否已激活）。
	recovery, err := store.NewRecoveryStore()
	if err != nil {
		return status, err
	}
	record, recordErr := recovery.Load()
	if recordErr == nil && activeRecoveryRecord(record) {
		mode := recordPolicyMode(record)
		status.PolicyMode = mode
		expected := planFromRecovery(record)
		actual, readErr := platform.ReadSRPPlan()
		rootSnapshot, rootErr := platform.InspectSRPRoot()
		if readErr == nil && rootErr == nil {
			status.ProtectionState = detectManagedState(record, expected, actual, rootSnapshot)
			// 白名单模式的临时解锁只翻 DefaultLevel，拦截规则仍写在注册表里继续生效；
			// 仅拦截模式的临时解锁会移除注册表里的拦截规则，此时不再生效。
			status.Enforcing = status.ProtectionState == model.ProtectionStateLocked ||
				(mode == model.PolicyModeWhitelist && status.ProtectionState == model.ProtectionStateUnlocked)
			listFrom := actual.BlockRules
			if mode == model.PolicyModeBlacklist && status.ProtectionState == model.ProtectionStateUnlocked {
				listFrom = expected.BlockRules // 解锁期间规则保留在恢复记录里
			}
			for _, rule := range listFrom {
				status.Rules = append(status.Rules, model.BlockRule{
					ID:      rule.ID,
					Pattern: rule.Path,
					Kind:    kindFromPattern(rule.Path),
					Label:   rule.Description,
					Preset:  isPresetDescription(rule.Description),
				})
			}
		}
	} else {
		status.ProtectionState = model.ProtectionStateUnmanaged
		domainJoined, domainErr := platform.IsDomainJoined()
		status.CanEnableBlockOnly = status.IsAdmin && domainErr == nil && !domainJoined
		status.Reason = "保护尚未启用；请前往主控制台启用黑名单模式"
	}
	status.RuleCount = len(status.Rules)

	// Mark vendors that are fully applied (every filename pattern is present).
	blocked := blockedPatternSet(status.Rules)
	for i, v := range status.Vendors {
		status.Vendors[i].Applied = vendorFullyApplied(v, blocked)
	}

	if status.Enforcing {
		status.Reason = fmt.Sprintf("共 %d 条黑名单规则，正在生效", status.RuleCount)
	} else if status.PolicyMode == model.PolicyModeBlacklist && status.ProtectionState == model.ProtectionStateUnlocked {
		status.Reason = "黑名单已临时解锁，规则保留在恢复记录中；重新锁定后恢复生效"
	} else if status.ProtectionState != model.ProtectionStateUnmanaged {
		status.Reason = "保护策略已配置但当前未完全生效"
	}
	return status, nil
}

// GetVendorPresets returns the built-in vendor presets. No write.
func (s *BlocklistService) GetVendorPresets() ([]model.VendorPreset, error) {
	return model.VendorPresets(), nil
}

// ApplyVendorPreset 应用一个预设厂商包的全部裸文件名规则。
// 不依赖保护状态：规则存进恢复记录；下次 WriteSRPPlan 时自动写入注册表。
// 若保护已启用，则即时写入注册表并更新恢复记录。
func (s *BlocklistService) ApplyVendorPreset(vendorID string) (model.BlocklistResult, error) {
	return s.applyVendorPreset(vendorID)
}

// RemoveVendorPreset 移除一个预设厂商包的全部规则。
func (s *BlocklistService) RemoveVendorPreset(vendorID string) (model.BlocklistResult, error) {
	return s.removeVendorRules(vendorID)
}

// AddBlockRule 手工添加一条拦截规则（裸文件名 / 目录 / 精确文件）。
func (s *BlocklistService) AddBlockRule(pattern, kind, label string) (model.BlocklistResult, error) {
	return s.addBlockRules([]rawBlockInput{{pattern: pattern, kind: kind, label: label, deriveNames: true}})
}

// RemoveBlockRule 按 ID 删除一条拦截规则。
func (s *BlocklistService) RemoveBlockRule(id string) (model.BlocklistResult, error) {
	return s.removeBlockRulesByID([]string{id})
}

// ScanVendorTargets 扫描本机已安装软件，返回命中已知垃圾软件厂商的候选列表。
func (s *BlocklistService) ScanVendorTargets() ([]model.BlockedVendorScan, error) {
	if runtime.GOOS != "windows" {
		return []model.BlockedVendorScan{}, nil
	}
	apps := platform.DiscoverInstalledApps()
	presets := model.VendorPresets()

	// 读取当前拦截规则，用于标记 AlreadyBlocked。
	var blocked map[string]bool
	if recovery, err := store.NewRecoveryStore(); err == nil {
		if record, rErr := recovery.Load(); rErr == nil && activeRecoveryRecord(record) {
			if actual, rErr2 := platform.ReadSRPPlan(); rErr2 == nil {
				blocked = pgapply.BlockedPatterns(actual)
			}
		}
	}
	if blocked == nil {
		blocked = map[string]bool{}
	}

	seen := map[string]bool{}
	var results []model.BlockedVendorScan
	for _, app := range apps {
		if app.InstallLocation == "" || !app.Selectable {
			continue
		}
		dir := strings.TrimRight(app.InstallLocation, `\`)
		key := strings.ToLower(dir)
		if seen[key] {
			continue
		}
		seen[key] = true

		matchedVendor, suggested := matchVendorByPublisher(app.Publisher, presets)
		item := model.BlockedVendorScan{
			ID:             "scan-" + app.ID,
			DisplayName:    app.DisplayName,
			Publisher:      app.Publisher,
			InstallPath:    dir,
			MatchedVendor:  matchedVendor,
			Suggested:      suggested,
			AlreadyBlocked: blocked[strings.ToLower(dir+`\*`)] || blocked[strings.ToLower(dir)],
		}
		results = append(results, item)
	}
	return results, nil
}

// ApplyScanResult 把扫描结果里用户选中的路径批量加为目录拦截规则。
func (s *BlocklistService) ApplyScanResult(installPaths []string) (model.BlocklistResult, error) {
	inputs := make([]rawBlockInput, 0, len(installPaths))
	for _, p := range installPaths {
		p = strings.TrimSpace(p)
		if p != "" {
			inputs = append(inputs, rawBlockInput{pattern: p, kind: model.BlockKindDirectory, label: "", deriveNames: true})
		}
	}
	return s.addBlockRules(inputs)
}

// ─── internals ────────────────────────────────────────────────────────────────

type rawBlockInput struct {
	pattern string
	kind    string
	label   string
	// deriveNames 为 true 且该输入是目录时，额外扫描目录下的 .exe 生成文件名规则，
	// 使这些程序无论从哪个位置启动都被拦（应对服务副本 / 看门狗从目录外复活）。
	// 厂商预设已自带精选文件名，无需派生。
	deriveNames bool
}

func (s *BlocklistService) applyVendorPreset(vendorID string) (model.BlocklistResult, error) {
	var preset *model.VendorPreset
	for _, v := range model.VendorPresets() {
		if v.ID == vendorID {
			cp := v
			preset = &cp
			break
		}
	}
	if preset == nil {
		return model.BlocklistResult{}, fmt.Errorf("%s: %q", model.BlocklistErrUnknownVendor, vendorID)
	}
	inputs := make([]rawBlockInput, 0, len(preset.FileNames)+len(preset.Directories))
	for _, fn := range preset.FileNames {
		inputs = append(inputs, rawBlockInput{pattern: fn, kind: model.BlockKindFileName, label: preset.Name + " · " + fn})
	}
	for _, dir := range preset.Directories {
		inputs = append(inputs, rawBlockInput{pattern: dir, kind: model.BlockKindDirectory, label: preset.Name + " · 安装目录"})
	}
	return s.addBlockRules(inputs)
}

func (s *BlocklistService) removeVendorRules(vendorID string) (model.BlocklistResult, error) {
	var preset *model.VendorPreset
	for _, v := range model.VendorPresets() {
		if v.ID == vendorID {
			cp := v
			preset = &cp
			break
		}
	}
	if preset == nil {
		return model.BlocklistResult{}, fmt.Errorf("%s: %q", model.BlocklistErrUnknownVendor, vendorID)
	}
	// Collect expected patterns for this vendor (both filenames and dirs).
	vendorPatterns := map[string]bool{}
	selfExe, _ := os.Executable()
	for _, fn := range preset.FileNames {
		if p, _, err := pgapply.NormalizeBlockPattern(fn, model.BlockKindFileName, selfExe); err == nil {
			vendorPatterns[strings.ToLower(p)] = true
		}
	}
	for _, dir := range preset.Directories {
		if p, _, err := pgapply.NormalizeBlockPattern(dir, model.BlockKindDirectory, selfExe); err == nil {
			vendorPatterns[strings.ToLower(p)] = true
		}
	}
	// Find IDs of matching rules in the live plan.
	return s.mutateBlockRules(func(plan pgapply.Plan) (pgapply.Plan, int, int, error) {
		var ids []string
		for _, rule := range plan.BlockRules {
			if vendorPatterns[strings.ToLower(rule.Path)] {
				ids = append(ids, rule.ID)
			}
		}
		if len(ids) == 0 {
			return plan, 0, 0, nil
		}
		updated, removed := pgapply.RemoveBlockRules(plan, ids)
		return updated, 0, removed, nil
	})
}

func (s *BlocklistService) addBlockRules(inputs []rawBlockInput) (model.BlocklistResult, error) {
	selfExe, _ := os.Executable()
	return s.mutateBlockRules(func(plan pgapply.Plan) (pgapply.Plan, int, int, error) {
		patterns := make([]string, 0, len(inputs))
		descriptions := make(map[string]string, len(inputs))
		addPattern := func(pattern, label string) {
			if _, exists := descriptions[pattern]; exists {
				return
			}
			patterns = append(patterns, pattern)
			descriptions[pattern] = label
		}
		for _, inp := range inputs {
			p, resolvedKind, err := pgapply.NormalizeBlockPattern(inp.pattern, inp.kind, selfExe)
			if err != nil {
				return pgapply.Plan{}, 0, 0, err
			}
			label := strings.TrimSpace(inp.label)
			if label == "" {
				label = "拦截：" + p
			}
			addPattern(p, label)

			// 拦目录时一并按文件名拦其中程序：目录规则只覆盖该目录树，而流氓软件
			// 常把常驻组件（服务副本 / 看门狗 / 托盘监控）装在目录外，文件名规则
			// 不限位置，可一并挡住这些"漏网"进程。派生失败（如系统关键名）静默跳过。
			if inp.deriveNames && resolvedKind == model.BlockKindDirectory {
				dir := strings.TrimSuffix(p, `\*`)
				for _, name := range platform.ListExecutableNames(dir) {
					fnPattern, _, fnErr := pgapply.NormalizeBlockPattern(name, model.BlockKindFileName, selfExe)
					if fnErr != nil {
						continue
					}
					addPattern(fnPattern, label+" · "+fnPattern)
				}
			}
		}
		updated, added := pgapply.AddBlockRules(plan, patterns, descriptions)
		return updated, added, 0, nil
	})
}

func (s *BlocklistService) removeBlockRulesByID(ids []string) (model.BlocklistResult, error) {
	return s.mutateBlockRules(func(plan pgapply.Plan) (pgapply.Plan, int, int, error) {
		updated, removed := pgapply.RemoveBlockRules(plan, ids)
		if removed == 0 {
			return pgapply.Plan{}, 0, 0, fmt.Errorf("%s: id not found", model.BlocklistErrNotFound)
		}
		return updated, 0, removed, nil
	})
}

type blockMutation func(pgapply.Plan) (pgapply.Plan, int, int, error)

func (s *BlocklistService) mutateBlockRules(mutate blockMutation) (model.BlocklistResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := requireWindowsAdmin(); err != nil {
		return model.BlocklistResult{}, err
	}

	recovery, err := store.NewRecoveryStore()
	if err != nil {
		return model.BlocklistResult{}, err
	}
	record, err := recovery.Load()
	if err != nil || !activeRecoveryRecord(record) {
		// Protection is not yet enabled: update the record for use when it is.
		return model.BlocklistResult{}, applyError(model.ApplyErrInvalidState, "保护尚未启用，请先在主控制台启用黑名单模式后再管理规则")
	}

	mode := recordPolicyMode(record)
	expected := planFromRecovery(record)
	actual, err := platform.ReadSRPPlan()
	if err != nil {
		return model.BlocklistResult{}, err
	}
	rootSnapshot, err := platform.InspectSRPRoot()
	if err != nil {
		return model.BlocklistResult{}, err
	}
	protectionState := detectManagedState(record, expected, actual, rootSnapshot)
	if protectionState == model.ProtectionStateAttention {
		return model.BlocklistResult{}, applyError(model.ApplyErrPolicyDrifted, "策略已被外部修改，请先处理注意状态")
	}
	if mode == model.PolicyModeBlacklist && protectionState == model.ProtectionStateUnlocked {
		// 解锁期间注册表里没有拦截规则，直接改会造成记录与注册表双向漂移。
		return model.BlocklistResult{}, applyError(model.ApplyErrInvalidState, "拦截已临时解锁，请先重新锁定再修改拦截规则")
	}

	updatedPlan, added, removed, err := mutate(expected)
	if err != nil {
		return model.BlocklistResult{}, err
	}
	if added == 0 && removed == 0 {
		return model.BlocklistResult{Changed: false, RuleCount: len(expected.BlockRules), Message: "无变更"}, nil
	}

	// Immediately write to registry (same pattern as updateManagedPlan).
	release, err := platform.AcquireApplyLock()
	if err != nil {
		return model.BlocklistResult{}, applyError(model.ApplyErrOperationProgress, err.Error())
	}
	defer release()

	currentTree, err := platform.SnapshotSRPRegistry()
	if err != nil {
		return model.BlocklistResult{}, err
	}
	if err := platform.WriteSRPPlanReplacing(updatedPlan, currentTree); err != nil {
		return model.BlocklistResult{}, err
	}
	if mode == model.PolicyModeWhitelist && protectionState == model.ProtectionStateUnlocked {
		if err := platform.SetSRPProtectionState(updatedPlan, false); err != nil {
			_ = platform.RestoreSRPRegistrySnapshot(currentTree)
			return model.BlocklistResult{}, err
		}
	}
	changedAt := s.now().UTC().Format(time.RFC3339Nano)
	updatedRecord := recoveryRecordWithPlan(record, updatedPlan, protectionState, changedAt)
	if err := recovery.Save(updatedRecord); err != nil {
		_ = platform.RestoreSRPRegistrySnapshot(currentTree)
		return model.BlocklistResult{}, applyError(model.ApplyErrRecoveryFailed, "拦截规则已写入但恢复记录保存失败: "+err.Error())
	}
	msg := buildBlocklistMessage(added, removed)
	if added > 0 {
		// SRP 只拦新进程创建：新增规则后主动结束命中的存量进程（托盘/后台常驻），
		// 否则用户看到"已拦截但托盘图标还在"。best-effort，失败不影响规则生效。
		if killed := terminateBlockedProcesses(addedBlockPatterns(expected.BlockRules, updatedPlan.BlockRules)); killed > 0 {
			msg += fmt.Sprintf("，已结束 %d 个正在运行的相关进程", killed)
		}
	}
	return model.BlocklistResult{
		Changed:      true,
		Applied:      true,
		RuleCount:    len(updatedPlan.BlockRules),
		AddedCount:   added,
		RemovedCount: removed,
		ChangedAt:    changedAt,
		Message:      msg,
	}, nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// addedBlockPatterns 返回 updated 相比 previous 新增的拦截模式。
func addedBlockPatterns(previous, updated []pgapply.Rule) []string {
	seen := make(map[string]bool, len(previous))
	for _, rule := range previous {
		seen[strings.ToLower(rule.Path)] = true
	}
	var added []string
	for _, rule := range updated {
		if !seen[strings.ToLower(rule.Path)] {
			added = append(added, rule.Path)
		}
	}
	return added
}

// terminateBlockedProcesses 结束镜像路径命中任一拦截模式的存量进程。
func terminateBlockedProcesses(patterns []string) int {
	if len(patterns) == 0 {
		return 0
	}
	return platform.TerminateProcessesMatching(func(imagePath string) bool {
		return pgapply.MatchesAnyBlockPattern(imagePath, patterns)
	})
}

func kindFromPattern(pattern string) string {
	if strings.ContainsAny(pattern, `\/`) {
		if strings.HasSuffix(pattern, `\*`) {
			return model.BlockKindDirectory
		}
		return model.BlockKindFile
	}
	return model.BlockKindFileName
}

func isPresetDescription(desc string) bool {
	for _, v := range model.VendorPresets() {
		if strings.Contains(desc, v.Name) {
			return true
		}
	}
	return false
}

func blockedPatternSet(rules []model.BlockRule) map[string]bool {
	m := make(map[string]bool, len(rules))
	for _, r := range rules {
		m[strings.ToLower(r.Pattern)] = true
	}
	return m
}

func vendorFullyApplied(v model.VendorPreset, blocked map[string]bool) bool {
	if len(v.FileNames) == 0 {
		return false
	}
	for _, fn := range v.FileNames {
		if !blocked[strings.ToLower(fn)] {
			return false
		}
	}
	return true
}

func matchVendorByPublisher(publisher string, presets []model.VendorPreset) (string, bool) {
	lower := strings.ToLower(publisher)
	for _, v := range presets {
		if v.Publisher != "" && strings.Contains(lower, strings.ToLower(v.Publisher)) {
			return v.ID, true
		}
	}
	return "", false
}

func buildBlocklistMessage(added, removed int) string {
	if added > 0 && removed > 0 {
		return fmt.Sprintf("已新增 %d 条、移除 %d 条拦截规则，即时生效", added, removed)
	}
	if added > 0 {
		return fmt.Sprintf("已新增 %d 条拦截规则，即时生效", added)
	}
	return fmt.Sprintf("已移除 %d 条拦截规则，即时生效", removed)
}
