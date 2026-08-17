using System.Collections.Generic;

namespace XCLing.Wpf.Models
{
    // 与 internal/model/blocklist.go 的 JSON 契约一一对应。

    public sealed class BlockRule
    {
        public string Id { get; set; }
        public string Pattern { get; set; }
        public string Kind { get; set; }
        public string Label { get; set; }
        public string VendorId { get; set; }
        public bool Preset { get; set; }
    }

    public sealed class VendorPreset
    {
        public string Id { get; set; }
        public string Name { get; set; }
        public string Publisher { get; set; }
        public string Description { get; set; }
        public List<string> FileNames { get; set; } = new List<string>();
        public List<string> Directories { get; set; } = new List<string>();
        public bool Applied { get; set; }
    }

    public sealed class BlocklistStatus
    {
        public bool Available { get; set; }
        public bool IsAdmin { get; set; }
        public string ProtectionState { get; set; }
        public string PolicyMode { get; set; }
        public bool Enforcing { get; set; }
        public bool CanEnableBlockOnly { get; set; }
        public List<BlockRule> Rules { get; set; } = new List<BlockRule>();
        public List<VendorPreset> Vendors { get; set; } = new List<VendorPreset>();
        public int RuleCount { get; set; }
        public string Reason { get; set; }
        public string CheckedAt { get; set; }
    }

    public sealed class BlocklistResult
    {
        public bool Changed { get; set; }
        public bool Applied { get; set; }
        public int RuleCount { get; set; }
        public int AddedCount { get; set; }
        public int RemovedCount { get; set; }
        public string ChangedAt { get; set; }
        public string Message { get; set; }
    }

    public sealed class BlockedVendorScan
    {
        public string Id { get; set; }
        public string DisplayName { get; set; }
        public string Publisher { get; set; }
        public string InstallPath { get; set; }
        public string MatchedVendor { get; set; }
        public bool Suggested { get; set; }
        public bool AlreadyBlocked { get; set; }
    }
}
