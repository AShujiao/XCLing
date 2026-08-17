package service

import (
	"encoding/binary"
	"encoding/json"
	"errors"
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

// ApplyService owns every user-triggered SRP lifecycle transition.
type ApplyService struct {
	mu  sync.Mutex
	now func() time.Time
}

// NewApplyService creates the SRP lifecycle service.
func NewApplyService() *ApplyService { return &ApplyService{now: time.Now} }

func applyError(code, message string) error { return fmt.Errorf("%s: %s", code, message) }

// requireWindowsAdmin 是所有 SRP 写操作的统一前置检查：非 Windows 直接不支持，
// 未提权进程在写注册表前就返回 ADMIN_REQUIRED（而不是等 HKLM 拒绝访问后
// 被包装成误导性的 WRITE_FAILED_ROLLED_BACK）。
func requireWindowsAdmin() error {
	if runtime.GOOS != "windows" {
		return applyError(model.ApplyErrUnsupported, "仅支持 Windows")
	}
	if !platform.IsAdmin() {
		return applyError(model.ApplyErrAdminRequired, "当前进程未以管理员身份运行")
	}
	return nil
}

func (s *ApplyService) beginOperation() (func(), error) {
	if !s.mu.TryLock() {
		return nil, applyError(model.ApplyErrOperationProgress, "另一个操作尚未完成")
	}
	release, err := platform.AcquireApplyLock()
	if err != nil {
		s.mu.Unlock()
		return nil, applyError(model.ApplyErrOperationProgress, err.Error())
	}
	return func() {
		release()
		s.mu.Unlock()
	}, nil
}

func activeRecoveryRecord(record model.RecoveryRecord) bool {
	return record.State == model.RecoveryStatePrepared || record.State == model.RecoveryStateApplied || record.State == model.RecoveryStateFailed
}

// GetApplyStatus returns the complete protection lifecycle state for the console.
func (s *ApplyService) GetApplyStatus() (model.ApplyStatus, error) {
	status := model.ApplyStatus{
		Warnings:        []string{},
		CheckedAt:       s.now().UTC().Format(time.RFC3339),
		ProtectionState: model.ProtectionStateUnmanaged,
	}
	if runtime.GOOS != "windows" {
		status.ReasonCode, status.Reason = model.ApplyErrUnsupported, "SRP 写入仅支持 Windows"
		return status, nil
	}
	status.Available = true
	status.IsAdmin = platform.IsAdmin()
	domainJoined, err := platform.IsDomainJoined()
	if err != nil {
		return status, err
	}
	status.DomainJoined = domainJoined
	rootSnapshot, err := platform.InspectSRPRoot()
	if err != nil {
		return status, err
	}
	status.SrpKeyExists = rootSnapshot.Exists
	status.SrpDisposition = platform.ClassifySRPRoot(rootSnapshot)
	status.WillReplaceLegacy = status.SrpDisposition == platform.SRPDispositionInertUnrestricted

	recovery, err := store.NewRecoveryStore()
	if err != nil {
		return status, err
	}
	status.RecoveryPath = recovery.Path()
	record, recordErr := recovery.Load()
	if recordErr != nil && !errors.Is(recordErr, os.ErrNotExist) {
		return status, recordErr
	}
	if recordErr == nil && activeRecoveryRecord(record) {
		mode := recordPolicyMode(record)
		status.Active = true
		status.PolicyMode = mode
		status.PolicyName = record.PolicyName
		status.Fingerprint = record.Fingerprint
		status.BackupCreatedAt = record.CreatedAt
		status.CanRestore = status.IsAdmin
		status.ManagedRules = managedRulesFromRecord(record)
		expected := planFromRecovery(record)
		actual, readErr := platform.ReadSRPPlan()
		if readErr != nil {
			status.ProtectionState = model.ProtectionStateAttention
			status.ReasonCode, status.Reason = model.ApplyErrPolicyDrifted, "活动策略无法完整读取，可查看变化或从备份恢复"
			return status, nil
		}
		status.ExistingRuleCount = len(actual.Rules)
		if mode == model.PolicyModeBlacklist {
			status.ExistingRuleCount = len(actual.BlockRules)
		}
		state := detectManagedState(record, expected, actual, rootSnapshot)
		status.ProtectionState = state
		switch state {
		case model.ProtectionStateLocked:
			status.CanUnlock = status.IsAdmin
			status.Reason = "白名单模式已启用"
			if mode == model.PolicyModeBlacklist {
				status.Reason = "黑名单模式已启用"
			}
		case model.ProtectionStateUnlocked:
			status.CanLock = status.IsAdmin
			status.Reason = "当前已临时解锁，规则仍保留；请手动重新锁定"
			if mode == model.PolicyModeBlacklist {
				status.Reason = "黑名单已临时解锁，规则保留在恢复记录中；请手动重新锁定"
			}
		default:
			status.ReasonCode, status.Reason = model.ApplyErrPolicyDrifted, "策略已被外部修改，可查看变化或从备份恢复"
		}
		return status, nil
	}

	if domainJoined {
		status.ReasonCode, status.Reason = model.ApplyErrDomainManaged, "本机已加入域，不能接管可能由 GPO 管理的 SRP"
		return status, nil
	}
	status.CanApply = true
	status.CanTakeOver = rootSnapshot.Exists
	if rootSnapshot.Exists {
		if actual, readErr := platform.ReadSRPPlan(); readErr == nil {
			status.ExistingRuleCount = len(actual.Rules)
		}
		status.Reason = "检测到本地 SRP，可完整备份后接管"
	} else {
		status.Reason = "尚未启用保护"
	}
	return status, nil
}

// EnableProtection applies a draft without requiring typed confirmation.
func (s *ApplyService) EnableProtection(draftJSON string) (model.ApplyResult, error) {
	result, err := s.applyWhitelistDraft(draftJSON, "", false)
	s.recordProtectionEvent("enable", result.Message, err)
	return result, err
}

// ApplyWhitelistDraft remains for compatibility with the previous UI.
func (s *ApplyService) ApplyWhitelistDraft(draftJSON, confirmation string) (model.ApplyResult, error) {
	result, err := s.applyWhitelistDraft(draftJSON, confirmation, true)
	s.recordProtectionEvent("enable", result.Message, err)
	return result, err
}

func (s *ApplyService) applyWhitelistDraft(draftJSON, confirmation string, requireConfirmation bool) (model.ApplyResult, error) {
	done, err := s.beginOperation()
	if err != nil {
		return model.ApplyResult{}, err
	}
	defer done()
	if err := requireWindowsAdmin(); err != nil {
		return model.ApplyResult{}, err
	}
	domain, err := platform.IsDomainJoined()
	if err != nil {
		return model.ApplyResult{}, err
	}
	if domain {
		return model.ApplyResult{}, applyError(model.ApplyErrDomainManaged, "域成员计算机禁止接管")
	}
	var draft model.WhitelistDraft
	if err := json.Unmarshal([]byte(strings.TrimSpace(draftJSON)), &draft); err != nil {
		return model.ApplyResult{}, err
	}
	mainExe, err := os.Executable()
	if err != nil {
		return model.ApplyResult{}, err
	}
	preflight := PreflightDraft(draft, mainExe)
	if preflight.Blocked {
		return model.ApplyResult{}, applyError(model.ApplyErrPreflightBlocked, preflight.Summary)
	}
	plan, err := pgapply.BuildPlan(draft, mainExe)
	if err != nil {
		if strings.Contains(err.Error(), model.ApplyErrSelfNotAllowed) {
			return model.ApplyResult{}, applyError(model.ApplyErrSelfNotAllowed, err.Error())
		}
		return model.ApplyResult{}, err
	}
	if requireConfirmation && confirmation != plan.PolicyName {
		return model.ApplyResult{}, errors.New("确认文本与策略名称不一致")
	}
	return s.applyPlanTakeover(plan, model.PolicyModeWhitelist)
}

// EnableBlockOnlyProtection 启用"仅拦截模式"：DefaultLevel 维持 Unrestricted
// （默认全部放行），只有 level-0 拦截名单生效。与白名单保护互斥——已有活动
// 策略时需先恢复原状。启用后拦截规则在拦截页增删，即时生效。
func (s *ApplyService) EnableBlockOnlyProtection() (model.ApplyResult, error) {
	result, err := s.enableBlockOnly()
	s.recordProtectionEvent("enable_block_only", result.Message, err)
	return result, err
}

func (s *ApplyService) enableBlockOnly() (model.ApplyResult, error) {
	done, err := s.beginOperation()
	if err != nil {
		return model.ApplyResult{}, err
	}
	defer done()
	if err := requireWindowsAdmin(); err != nil {
		return model.ApplyResult{}, err
	}
	domain, err := platform.IsDomainJoined()
	if err != nil {
		return model.ApplyResult{}, err
	}
	if domain {
		return model.ApplyResult{}, applyError(model.ApplyErrDomainManaged, "域成员计算机禁止接管")
	}
	plan := pgapply.Plan{
		PolicyName:         model.AppName + " 仅拦截策略",
		DefaultLevel:       model.SrpLevelUnrestrictedRaw,
		PolicyScope:        pgapply.PolicyScopeAllUsers,
		TransparentEnabled: pgapply.TransparentEnabledEXE,
	}
	return s.applyPlanTakeover(plan, model.PolicyModeBlacklist)
}

// applyPlanTakeover 是两种模式共用的接管内核：完整备份既有 SRP 树、落恢复
// 记录、原子替换写入、写后校验，任一步失败回滚到备份。调用方负责操作锁、
// 管理员与域检查。
func (s *ApplyService) applyPlanTakeover(plan pgapply.Plan, mode string) (model.ApplyResult, error) {
	beforeTree, err := platform.SnapshotSRPRegistry()
	if err != nil {
		return model.ApplyResult{}, err
	}
	beforeRoot, err := platform.InspectSRPRoot()
	if err != nil {
		return model.ApplyResult{}, err
	}
	recovery, err := store.NewRecoveryStore()
	if err != nil {
		return model.ApplyResult{}, err
	}
	if existing, loadErr := recovery.Load(); loadErr == nil && activeRecoveryRecord(existing) {
		return model.ApplyResult{}, applyError(model.ApplyErrAlreadyApplied, "已有活动策略")
	} else if loadErr != nil && !errors.Is(loadErr, os.ErrNotExist) {
		return model.ApplyResult{}, loadErr
	}
	now := s.now().UTC()
	recoveryRules := make([]model.RecoveryRule, 0, len(plan.Rules))
	for _, rule := range plan.Rules {
		recoveryRules = append(recoveryRules, model.RecoveryRule{ID: rule.ID, Path: rule.Path, Description: rule.Description, Level: rule.Level})
	}
	recoveryBlockRules := make([]model.RecoveryRule, 0, len(plan.BlockRules))
	for _, rule := range plan.BlockRules {
		recoveryBlockRules = append(recoveryBlockRules, model.RecoveryRule{ID: rule.ID, Path: rule.Path, Description: rule.Description, Level: rule.Level})
	}
	beforeState := platform.ClassifySRPRoot(beforeRoot)
	record := model.RecoveryRecord{
		SchemaVersion:   "2",
		AppVersion:      model.AppVersion,
		ID:              now.Format("20060102T150405.000000000Z"),
		PolicyName:      plan.PolicyName,
		RuleCount:       len(plan.Rules),
		RulesDigest:     pgapply.RulesDigest(plan),
		DefaultLevel:    plan.DefaultLevel,
		PolicyScope:     plan.PolicyScope,
		Transparent:     plan.TransparentEnabled,
		Rules:           recoveryRules,
		BlockRules:      recoveryBlockRules,
		PolicyMode:      mode,
		Fingerprint:     pgapply.Fingerprint(plan),
		BeforeState:     beforeState,
		BeforeSnapshot:  beforeTree,
		ProtectionState: model.ProtectionStateLocked,
		State:           model.RecoveryStatePrepared,
		CreatedAt:       now.Local().Format(time.RFC3339Nano),
	}
	if beforeState == platform.SRPDispositionInertUnrestricted {
		record.BeforeDefaultLevel = model.SrpLevelUnrestrictedRaw
	}
	if err := recovery.Save(record); err != nil {
		return model.ApplyResult{}, err
	}
	confirmedBefore, err := platform.SnapshotSRPRegistry()
	if err != nil {
		return model.ApplyResult{}, err
	}
	if !beforeTree.Equal(confirmedBefore) {
		record.State, record.LastErrorCode, record.LastDiagnostic = model.RecoveryStateRestored, model.ApplyErrLegacySRPDrifted, "SRP tree changed after backup"
		_ = recovery.Save(record)
		return model.ApplyResult{}, applyError(model.ApplyErrLegacySRPDrifted, "接管前策略发生变化，未执行写入")
	}
	if err := platform.WriteSRPPlanReplacing(plan, beforeTree); err != nil {
		return model.ApplyResult{}, s.finishFailedApply(recovery, record, plan, err)
	}
	actual, readErr := platform.ReadSRPPlan()
	rootAfter, rootErr := platform.InspectSRPRoot()
	// 两种模式的启用校验相同：注册表回读与计划指纹完全一致，且根形态归本程序所有。
	if readErr != nil || rootErr != nil || pgapply.Fingerprint(actual) != pgapply.Fingerprint(plan) || !platform.OwnedRootMatches(rootAfter, uint64(plan.DefaultLevel), uint64(plan.PolicyScope), len(plan.BlockRules) > 0) {
		verifyErr := readErr
		if verifyErr == nil {
			verifyErr = rootErr
		}
		if verifyErr == nil {
			verifyErr = errors.New("应用后策略校验不一致")
		}
		return model.ApplyResult{}, s.finishFailedApply(recovery, record, plan, verifyErr)
	}
	record.State, record.AppliedAt = model.RecoveryStateApplied, s.now().UTC().Format(time.RFC3339Nano)
	record.LastStateChangedAt = record.AppliedAt
	if err := recovery.Save(record); err != nil {
		return model.ApplyResult{}, applyError(model.ApplyErrRecoveryFailed, "策略已启用但恢复记录提交失败: "+err.Error())
	}
	message := "白名单模式已启用"
	if beforeTree.Exists {
		message = "既有 SRP 已备份，白名单模式已启用"
	}
	if mode == model.PolicyModeBlacklist {
		message = "黑名单模式已启用，可在「黑名单」页添加拦截规则"
		if beforeTree.Exists {
			message = "既有 SRP 已备份，黑名单模式已启用"
		}
	}
	return model.ApplyResult{Applied: true, PolicyName: plan.PolicyName, RuleCount: len(plan.Rules), Fingerprint: record.Fingerprint, AppliedAt: record.AppliedAt, Message: message}, nil
}

func (s *ApplyService) finishFailedApply(recovery *store.RecoveryStore, record model.RecoveryRecord, _ pgapply.Plan, cause error) error {
	restoreErr := platform.RestoreSRPRegistrySnapshot(record.BeforeSnapshot)
	final, checkErr := platform.SnapshotSRPRegistry()
	if restoreErr == nil && checkErr == nil && final.Equal(record.BeforeSnapshot) {
		record.State, record.LastErrorCode, record.LastDiagnostic = model.RecoveryStateRestored, model.ApplyErrWriteRolledBack, cause.Error()
		_ = recovery.Save(record)
		return applyError(model.ApplyErrWriteRolledBack, cause.Error())
	}
	record.State, record.LastErrorCode, record.LastDiagnostic = model.RecoveryStateFailed, model.ApplyErrRecoveryFailed, cause.Error()
	_ = recovery.Save(record)
	return applyError(model.ApplyErrRecoveryFailed, "写入失败且回滚未完成")
}

// UnlockProtection temporarily allows software execution while retaining all rules.
func (s *ApplyService) UnlockProtection() (model.ProtectionResult, error) {
	result, err := s.changeProtection(false)
	s.recordProtectionEvent("unlock", result.Message, err)
	return result, err
}

// LockProtection re-enables the retained XCLing rules.
func (s *ApplyService) LockProtection() (model.ProtectionResult, error) {
	result, err := s.changeProtection(true)
	s.recordProtectionEvent("lock", result.Message, err)
	return result, err
}

func (s *ApplyService) changeProtection(locked bool) (model.ProtectionResult, error) {
	done, err := s.beginOperation()
	if err != nil {
		return model.ProtectionResult{}, err
	}
	defer done()
	if err := requireWindowsAdmin(); err != nil {
		return model.ProtectionResult{}, err
	}
	recovery, err := store.NewRecoveryStore()
	if err != nil {
		return model.ProtectionResult{}, err
	}
	record, err := recovery.Load()
	if err != nil || record.State != model.RecoveryStateApplied {
		return model.ProtectionResult{}, applyError(model.ApplyErrInvalidState, "没有可切换的活动策略")
	}
	expected := planFromRecovery(record)
	if recordPolicyMode(record) == model.PolicyModeBlacklist {
		return s.changeBlockOnlyProtection(recovery, record, expected, locked)
	}
	actual, err := platform.ReadSRPPlan()
	if err != nil {
		return model.ProtectionResult{}, err
	}
	previousState := pgapply.DetectProtectionState(actual, expected)
	if previousState == model.ProtectionStateAttention {
		return model.ProtectionResult{}, applyError(model.ApplyErrPolicyDrifted, "当前策略已发生变化")
	}
	if err := platform.SetSRPProtectionState(expected, locked); err != nil {
		return model.ProtectionResult{}, err
	}
	state, message := model.ProtectionStateUnlocked, "已临时解锁；规则仍保留，需手动重新锁定"
	if locked {
		state, message = model.ProtectionStateLocked, "软件保护已重新锁定"
	}
	changedAt := s.now().UTC().Format(time.RFC3339Nano)
	record.ProtectionState, record.LastStateChangedAt = state, changedAt
	if err := recovery.Save(record); err != nil {
		_ = platform.SetSRPProtectionState(expected, previousState == model.ProtectionStateLocked)
		return model.ProtectionResult{}, applyError(model.ApplyErrRecoveryFailed, "状态已回滚，恢复记录保存失败: "+err.Error())
	}
	return model.ProtectionResult{ProtectionState: state, ChangedAt: changedAt, Message: message}, nil
}

// changeBlockOnlyProtection 切换仅拦截模式的锁定状态。该模式下 DefaultLevel
// 恒为 Unrestricted，没有可翻转的级别开关，因此：
//
//	解锁   = 原子重写为"去掉全部拦截规则"的同一策略（规则保留在恢复记录里）；
//	重新锁定 = 按恢复记录原子重写回完整拦截计划。
func (s *ApplyService) changeBlockOnlyProtection(recovery *store.RecoveryStore, record model.RecoveryRecord, expected pgapply.Plan, locked bool) (model.ProtectionResult, error) {
	if !locked && len(expected.BlockRules) == 0 {
		return model.ProtectionResult{}, applyError(model.ApplyErrInvalidState, "当前没有拦截规则，无需临时解锁")
	}
	actual, err := platform.ReadSRPPlan()
	if err != nil {
		return model.ProtectionResult{}, err
	}
	rootSnapshot, err := platform.InspectSRPRoot()
	if err != nil {
		return model.ProtectionResult{}, err
	}
	previousState := detectManagedState(record, expected, actual, rootSnapshot)
	if previousState == model.ProtectionStateAttention {
		return model.ProtectionResult{}, applyError(model.ApplyErrPolicyDrifted, "当前策略已发生变化")
	}
	desiredState, message := model.ProtectionStateUnlocked, "拦截已临时解锁；规则仍保留，需手动重新锁定"
	if locked {
		desiredState, message = model.ProtectionStateLocked, "拦截已重新锁定"
	}
	currentTree, err := platform.SnapshotSRPRegistry()
	if err != nil {
		return model.ProtectionResult{}, err
	}
	if previousState != desiredState {
		target := expected
		if !locked {
			target.BlockRules = nil
		}
		if err := platform.WriteSRPPlanReplacing(target, currentTree); err != nil {
			return model.ProtectionResult{}, err
		}
	}
	changedAt := s.now().UTC().Format(time.RFC3339Nano)
	record.ProtectionState, record.LastStateChangedAt = desiredState, changedAt
	if err := recovery.Save(record); err != nil {
		if previousState != desiredState {
			_ = platform.RestoreSRPRegistrySnapshot(currentTree)
		}
		return model.ProtectionResult{}, applyError(model.ApplyErrRecoveryFailed, "状态已回滚，恢复记录保存失败: "+err.Error())
	}
	return model.ProtectionResult{ProtectionState: desiredState, ChangedAt: changedAt, Message: message}, nil
}

// AddTrustedPath immediately adds a directory or exact executable to an active policy.
func (s *ApplyService) AddTrustedPath(path, kind, label string) (model.RuleChangeResult, error) {
	directory := kind != model.CustomKindFile
	pattern := strings.TrimRight(strings.TrimSpace(path), `\`)
	if directory {
		pattern += `\*`
	}
	if directory && isDangerousUserAllow(pattern) {
		err := applyError(model.ApplyErrPreflightBlocked, "不能放行整个用户可写目录")
		s.recordProtectionEvent("rule_add", "", err)
		return model.RuleChangeResult{}, err
	}
	description := strings.TrimSpace(label)
	if description == "" {
		description = "额外可信路径"
	}
	result, err := s.updateManagedPlan(func(plan pgapply.Plan) (pgapply.Plan, pgapply.Rule, error) {
		return pgapply.AddTrustedRule(plan, path, directory, description)
	})
	if err == nil {
		result.Message = "可信路径已添加并立即生效"
	}
	s.recordProtectionEvent("rule_add", result.Message, err)
	return result, err
}

// RemoveTrustedRule immediately removes a removable rule from an active policy.
func (s *ApplyService) RemoveTrustedRule(id string) (model.RuleChangeResult, error) {
	result, err := s.updateManagedPlan(func(plan pgapply.Plan) (pgapply.Plan, pgapply.Rule, error) {
		for _, rule := range plan.Rules {
			if !strings.EqualFold(rule.ID, strings.TrimSpace(id)) {
				continue
			}
			if !managedRuleRemovable(rule) {
				return pgapply.Plan{}, pgapply.Rule{}, applyError(model.ApplyErrRuleNotRemovable, "系统基础规则、兼容规则和 "+model.AppName+" 自身规则不能删除")
			}
			updated, removeErr := pgapply.RemoveTrustedRule(plan, id)
			return updated, rule, removeErr
		}
		return pgapply.Plan{}, pgapply.Rule{}, errors.New("未找到要删除的可信规则")
	})
	if err == nil {
		result.Message = "可信路径已删除并立即生效"
	}
	s.recordProtectionEvent("rule_remove", result.Message, err)
	return result, err
}

type planMutation func(pgapply.Plan) (pgapply.Plan, pgapply.Rule, error)

func (s *ApplyService) updateManagedPlan(mutate planMutation) (model.RuleChangeResult, error) {
	done, err := s.beginOperation()
	if err != nil {
		return model.RuleChangeResult{}, err
	}
	defer done()
	if err := requireWindowsAdmin(); err != nil {
		return model.RuleChangeResult{}, err
	}
	recovery, err := store.NewRecoveryStore()
	if err != nil {
		return model.RuleChangeResult{}, err
	}
	record, err := recovery.Load()
	if err != nil || record.State != model.RecoveryStateApplied {
		return model.RuleChangeResult{}, applyError(model.ApplyErrInvalidState, "保护尚未启用，规则将在下次启用时生效")
	}
	if recordPolicyMode(record) == model.PolicyModeBlacklist {
		return model.RuleChangeResult{}, applyError(model.ApplyErrInvalidState, "黑名单模式默认放行全部程序，不维护白名单规则")
	}
	expected := planFromRecovery(record)
	actual, err := platform.ReadSRPPlan()
	if err != nil {
		return model.RuleChangeResult{}, err
	}
	previousState := pgapply.DetectProtectionState(actual, expected)
	rootSnapshot, err := platform.InspectSRPRoot()
	if err != nil {
		return model.RuleChangeResult{}, err
	}
	if previousState == model.ProtectionStateAttention || !platform.OwnedRootMatchesLegacyCompatible(rootSnapshot, uint64(actual.DefaultLevel), uint64(expected.PolicyScope), len(expected.BlockRules) > 0) {
		return model.RuleChangeResult{}, applyError(model.ApplyErrPolicyDrifted, "当前策略已发生外部变化")
	}
	updatedPlan, changedRule, err := mutate(expected)
	if err != nil {
		return model.RuleChangeResult{}, err
	}
	currentTree, err := platform.SnapshotSRPRegistry()
	if err != nil {
		return model.RuleChangeResult{}, err
	}
	rollback := func(cause error) error {
		if restoreErr := platform.RestoreSRPRegistrySnapshot(currentTree); restoreErr != nil {
			return applyError(model.ApplyErrRecoveryFailed, cause.Error()+"；更新前策略恢复失败："+restoreErr.Error())
		}
		return applyError(model.ApplyErrWriteRolledBack, cause.Error())
	}
	if err := platform.WriteSRPPlanReplacing(updatedPlan, currentTree); err != nil {
		return model.RuleChangeResult{}, err
	}
	if previousState == model.ProtectionStateUnlocked {
		if err := platform.SetSRPProtectionState(updatedPlan, false); err != nil {
			return model.RuleChangeResult{}, rollback(err)
		}
	}
	verified, readErr := platform.ReadSRPPlan()
	verifiedRoot, rootErr := platform.InspectSRPRoot()
	desiredLevel := uint64(updatedPlan.DefaultLevel)
	if previousState == model.ProtectionStateUnlocked {
		desiredLevel = model.SrpLevelUnrestrictedRaw
	}
	if readErr != nil || rootErr != nil || pgapply.DetectProtectionState(verified, updatedPlan) != previousState || !platform.OwnedRootMatches(verifiedRoot, desiredLevel, uint64(updatedPlan.PolicyScope), len(updatedPlan.BlockRules) > 0) {
		verifyErr := readErr
		if verifyErr == nil {
			verifyErr = rootErr
		}
		if verifyErr == nil {
			verifyErr = errors.New("规则更新后校验不一致")
		}
		return model.RuleChangeResult{}, rollback(verifyErr)
	}
	changedAt := s.now().UTC().Format(time.RFC3339Nano)
	updatedRecord := recoveryRecordWithPlan(record, updatedPlan, previousState, changedAt)
	if err := recovery.Save(updatedRecord); err != nil {
		return model.RuleChangeResult{}, rollback(errors.New("恢复记录保存失败: " + err.Error()))
	}
	managed := model.ManagedRule{ID: changedRule.ID, Path: changedRule.Path, Description: changedRule.Description, Removable: managedRuleRemovable(changedRule)}
	return model.RuleChangeResult{ProtectionState: previousState, Rule: managed, RuleCount: len(updatedPlan.Rules), ChangedAt: changedAt}, nil
}

func recoveryRecordWithPlan(record model.RecoveryRecord, plan pgapply.Plan, protectionState, changedAt string) model.RecoveryRecord {
	updated := record
	updated.SchemaVersion = "2"
	if record.SchemaVersion == "1" {
		updated.BeforeSnapshot = legacyBeforeSnapshot(record)
	}
	updated.PolicyName = plan.PolicyName
	updated.RuleCount = len(plan.Rules)
	updated.RulesDigest = pgapply.RulesDigest(plan)
	updated.DefaultLevel = plan.DefaultLevel
	updated.PolicyScope = plan.PolicyScope
	updated.Transparent = plan.TransparentEnabled
	updated.Fingerprint = pgapply.Fingerprint(plan)
	updated.ProtectionState = protectionState
	updated.LastStateChangedAt = changedAt
	updated.Rules = make([]model.RecoveryRule, 0, len(plan.Rules))
	for _, rule := range plan.Rules {
		updated.Rules = append(updated.Rules, model.RecoveryRule{ID: rule.ID, Path: rule.Path, Description: rule.Description, Level: rule.Level})
	}
	updated.BlockRules = make([]model.RecoveryRule, 0, len(plan.BlockRules))
	for _, rule := range plan.BlockRules {
		updated.BlockRules = append(updated.BlockRules, model.RecoveryRule{ID: rule.ID, Path: rule.Path, Description: rule.Description, Level: rule.Level})
	}
	return updated
}

func legacyBeforeSnapshot(record model.RecoveryRecord) model.RegistryTreeSnapshot {
	if record.BeforeState == model.BeforeStateAbsent {
		return model.RegistryTreeSnapshot{}
	}
	data := make([]byte, 4)
	binary.LittleEndian.PutUint32(data, uint32(record.BeforeDefaultLevel))
	return model.RegistryTreeSnapshot{Exists: true, Root: model.RegistryKeySnapshot{Values: []model.RegistryValueSnapshot{{Name: "DefaultLevel", Type: 4, Data: data}}, Children: []model.RegistryNamedKeySnapshot{}}}
}

func managedRulesFromRecord(record model.RecoveryRecord) []model.ManagedRule {
	rules := make([]model.ManagedRule, 0, len(record.Rules))
	for _, rule := range record.Rules {
		planRule := pgapply.Rule{ID: rule.ID, Path: rule.Path, Description: rule.Description, Level: rule.Level}
		rules = append(rules, model.ManagedRule{ID: rule.ID, Path: rule.Path, Description: rule.Description, Removable: managedRuleRemovable(planRule)})
	}
	return rules
}

func managedRuleRemovable(rule pgapply.Rule) bool {
	path := strings.ToLower(strings.TrimRight(strings.ReplaceAll(strings.TrimSpace(rule.Path), "/", `\`), `\*`))
	mandatory := []string{
		`c:\windows`,
		`c:\program files`,
		`c:\program files (x86)`,
		`c:\program files\windowsapps`,
		`c:\programdata\microsoft\windows defender\platform`,
		`c:\programdata\microsoft\windows defender\definition updates`,
	}
	for _, fixed := range mandatory {
		if path == fixed {
			return false
		}
	}
	if executable, err := os.Executable(); err == nil && strings.EqualFold(strings.TrimSpace(rule.Path), executable) {
		return false
	}
	return true
}

// RestoreAppliedPolicy remains for compatibility with the previous UI.
func (s *ApplyService) RestoreAppliedPolicy(confirmation string) (model.RestoreResult, error) {
	result, err := s.restoreOriginalPolicy(&confirmation, false)
	s.recordProtectionEvent("restore", result.Message, err)
	return result, err
}

// RestoreOriginalPolicy restores the complete pre-takeover snapshot. Force is
// reserved for the explicit recovery action shown after external drift.
func (s *ApplyService) RestoreOriginalPolicy(force bool) (model.RestoreResult, error) {
	result, err := s.restoreOriginalPolicy(nil, force)
	s.recordProtectionEvent("restore", result.Message, err)
	return result, err
}

// ListProtectionEvents returns the newest lifecycle operations first.
func (s *ApplyService) ListProtectionEvents() ([]model.ProtectionEvent, error) {
	if runtime.GOOS != "windows" {
		return []model.ProtectionEvent{}, nil
	}
	events, err := store.NewProtectionEventStore()
	if err != nil {
		return nil, err
	}
	return events.List()
}

func (s *ApplyService) recordProtectionEvent(action, message string, operationErr error) {
	if runtime.GOOS != "windows" {
		return
	}
	events, err := store.NewProtectionEventStore()
	if err != nil {
		return
	}
	if operationErr != nil {
		message = operationErr.Error()
	}
	createdAt := s.now().Local().Format(time.RFC3339Nano)
	_ = events.Append(model.ProtectionEvent{
		ID:        createdAt,
		Action:    action,
		Success:   operationErr == nil,
		Message:   message,
		CreatedAt: createdAt,
	})
}

func (s *ApplyService) restoreOriginalPolicy(confirmation *string, force bool) (model.RestoreResult, error) {
	done, err := s.beginOperation()
	if err != nil {
		return model.RestoreResult{}, err
	}
	defer done()
	if err := requireWindowsAdmin(); err != nil {
		return model.RestoreResult{}, err
	}
	recovery, err := store.NewRecoveryStore()
	if err != nil {
		return model.RestoreResult{}, err
	}
	record, err := recovery.Load()
	if errors.Is(err, os.ErrNotExist) || (err == nil && !activeRecoveryRecord(record)) {
		return model.RestoreResult{}, applyError(model.ApplyErrNoRecoveryRecord, "没有活动恢复记录")
	}
	if err != nil {
		return model.RestoreResult{}, err
	}
	if confirmation != nil && *confirmation != record.PolicyName {
		return model.RestoreResult{}, errors.New("确认文本与策略名称不一致")
	}
	expected := planFromRecovery(record)
	if !force {
		actual, readErr := platform.ReadSRPPlan()
		rootSnapshot, rootErr := platform.InspectSRPRoot()
		if readErr != nil || rootErr != nil || detectManagedState(record, expected, actual, rootSnapshot) == model.ProtectionStateAttention {
			return model.RestoreResult{}, applyError(model.ApplyErrPolicyDrifted, "策略已被外部修改，请查看变化后使用从备份恢复")
		}
	}
	if record.SchemaVersion == "2" {
		if err := platform.RestoreSRPRegistrySnapshot(record.BeforeSnapshot); err != nil {
			return model.RestoreResult{}, applyError(model.ApplyErrRecoveryFailed, err.Error())
		}
		final, verifyErr := platform.SnapshotSRPRegistry()
		if verifyErr != nil || !final.Equal(record.BeforeSnapshot) {
			return model.RestoreResult{}, applyError(model.ApplyErrRecoveryFailed, "恢复后的 SRP 与接管前备份不一致")
		}
	} else {
		if err := platform.RestoreSRPBeforeState(expected, record.BeforeState); err != nil {
			return model.RestoreResult{}, applyError(model.ApplyErrRecoveryFailed, err.Error())
		}
	}
	record.State, record.RestoredAt = model.RecoveryStateRestored, s.now().UTC().Format(time.RFC3339Nano)
	record.ProtectionState, record.LastStateChangedAt = model.ProtectionStateUnmanaged, record.RestoredAt
	if err := recovery.Save(record); err != nil {
		return model.RestoreResult{}, err
	}
	return model.RestoreResult{Restored: true, PolicyName: record.PolicyName, RestoredAt: record.RestoredAt, Message: "已恢复接管前的完整 SRP 状态"}, nil
}

func planFromRecovery(record model.RecoveryRecord) pgapply.Plan {
	plan := pgapply.Plan{PolicyName: record.PolicyName, DefaultLevel: record.DefaultLevel, PolicyScope: record.PolicyScope, TransparentEnabled: record.Transparent, Rules: make([]pgapply.Rule, 0, len(record.Rules))}
	for _, rule := range record.Rules {
		plan.Rules = append(plan.Rules, pgapply.Rule{ID: rule.ID, Path: rule.Path, Description: rule.Description, Level: rule.Level})
	}
	for _, rule := range record.BlockRules {
		plan.BlockRules = append(plan.BlockRules, pgapply.Rule{ID: rule.ID, Path: rule.Path, Description: rule.Description, Level: rule.Level})
	}
	return plan
}

// recordPolicyMode 返回恢复记录的策略模式；旧记录没有该字段，按白名单处理。
func recordPolicyMode(record model.RecoveryRecord) string {
	if record.PolicyMode == model.PolicyModeBlacklist {
		return model.PolicyModeBlacklist
	}
	return model.PolicyModeWhitelist
}

// detectManagedState 按记录模式判定活动策略的保护状态，并同时校验 SRP 根形态
// 是否仍归本程序所有；任一不符即归为 attention（外部修改）。
func detectManagedState(record model.RecoveryRecord, expected, actual pgapply.Plan, root platform.SRPRootSnapshot) string {
	if recordPolicyMode(record) == model.PolicyModeBlacklist {
		state := pgapply.DetectBlockOnlyProtectionState(actual, expected)
		if state == model.ProtectionStateAttention {
			return state
		}
		// 解锁状态下注册表里没有拦截规则，根下不应存在 level-0 键。
		hasBlock := state == model.ProtectionStateLocked && len(expected.BlockRules) > 0
		if !platform.OwnedRootMatches(root, uint64(actual.DefaultLevel), uint64(expected.PolicyScope), hasBlock) {
			return model.ProtectionStateAttention
		}
		return state
	}
	state := pgapply.DetectProtectionState(actual, expected)
	if state != model.ProtectionStateAttention && !platform.OwnedRootMatchesLegacyCompatible(root, uint64(actual.DefaultLevel), uint64(expected.PolicyScope), len(expected.BlockRules) > 0) {
		return model.ProtectionStateAttention
	}
	return state
}
