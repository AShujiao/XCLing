using System;
using System.Collections.Generic;

namespace XCLing.Wpf.Models
{
    // 与 internal/model/apply.go 的 JSON 契约一一对应（camelCase 自动映射）。

    public sealed class ApplyStatus
    {
        public bool Available { get; set; }
        public bool CanApply { get; set; }
        public bool CanRestore { get; set; }
        public bool IsAdmin { get; set; }
        public bool DomainJoined { get; set; }
        public bool SrpKeyExists { get; set; }
        public string SrpDisposition { get; set; }
        public bool WillReplaceLegacy { get; set; }
        public bool Active { get; set; }
        public string PolicyName { get; set; }
        public string Fingerprint { get; set; }
        public string ReasonCode { get; set; }
        public string Reason { get; set; }
        public List<string> Warnings { get; set; } = new List<string>();
        public string RecoveryPath { get; set; }
        public string CheckedAt { get; set; }
        public string ProtectionState { get; set; }
        public string PolicyMode { get; set; }
        public bool CanTakeOver { get; set; }
        public bool CanUnlock { get; set; }
        public bool CanLock { get; set; }
        public int ExistingRuleCount { get; set; }
        public string BackupCreatedAt { get; set; }
        public List<ManagedRule> ManagedRules { get; set; } = new List<ManagedRule>();
    }

    public sealed class ManagedRule
    {
        public string Id { get; set; }
        public string Path { get; set; }
        public string Description { get; set; }
        public bool Removable { get; set; }
    }

    public sealed class ApplyResult
    {
        public bool Applied { get; set; }
        public string PolicyName { get; set; }
        public int RuleCount { get; set; }
        public string Fingerprint { get; set; }
        public string AppliedAt { get; set; }
        public string Message { get; set; }
    }

    public sealed class RestoreResult
    {
        public bool Restored { get; set; }
        public string PolicyName { get; set; }
        public string RestoredAt { get; set; }
        public string Message { get; set; }
    }

    public sealed class ProtectionResult
    {
        public string ProtectionState { get; set; }
        public string ChangedAt { get; set; }
        public string Message { get; set; }
    }

    public sealed class RuleChangeResult
    {
        public string ProtectionState { get; set; }
        public ManagedRule Rule { get; set; }
        public int RuleCount { get; set; }
        public string ChangedAt { get; set; }
        public string Message { get; set; }
    }

    public sealed class ProtectionEvent
    {
        public string Id { get; set; }
        public string Action { get; set; }
        public bool Success { get; set; }
        public string Message { get; set; }
        public string CreatedAt { get; set; }

        public string ActionLabel
        {
            get
            {
                switch (Action)
                {
                    case "enable": return "启用白名单模式";
                    case "enable_block_only": return "启用黑名单模式";
                    case "unlock": return "临时解锁";
                    case "lock": return "重新锁定";
                    case "restore": return "恢复原状";
                    case "rule_add": return "添加白名单规则";
                    case "rule_remove": return "删除白名单规则";
                    case "shutdown": return "定时关机";
                    default: return Action;
                }
            }
        }
    }

    public sealed class ShutdownConfig
    {
        public bool Enabled { get; set; }
        public int Hour { get; set; }
        public string CreatedAt { get; set; }
    }


    // 与 internal/model/whitelist.go 的预检报告契约对应（主控制台启用流程只需这两个）。

    public sealed class PreflightCheck
    {
        public string Id { get; set; }
        public string Title { get; set; }
        public string Status { get; set; }
        public string Message { get; set; }
        public string Detail { get; set; }
    }

    public sealed class PreflightReport
    {
        public bool Blocked { get; set; }
        public int BlockingCount { get; set; }
        public int WarningCount { get; set; }
        public int PassCount { get; set; }
        public List<PreflightCheck> Checks { get; set; } = new List<PreflightCheck>();
        public string Summary { get; set; }
        public string GeneratedAt { get; set; }
    }
}
