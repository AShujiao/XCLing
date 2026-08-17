using System.Collections.Generic;

namespace XCLing.Wpf.Models
{
    // 与 internal/model/audit.go 的 JSON 契约对应（仅保留运行记录页用到的只读事件）。

    public sealed class AuditCapability
    {
        public string OS { get; set; }
        public bool IsWindows { get; set; }
        public bool Available { get; set; }
        public string Tool { get; set; }
        public List<string> Channels { get; set; } = new List<string>();
        public string Reason { get; set; }
        public List<string> Notes { get; set; } = new List<string>();
        public string ProbedAt { get; set; }
        public bool AuditEnabled { get; set; }
        public bool AuditAvailable { get; set; }
        public string AuditDetail { get; set; }
    }

    public sealed class AuditEvent
    {
        public string Id { get; set; }
        public string Timestamp { get; set; }
        public string Provider { get; set; }
        public string Channel { get; set; }
        public int EventId { get; set; }
        public string Level { get; set; }
        public string ExecutablePath { get; set; }
        public string User { get; set; }
        public string Message { get; set; }
        public string RawSummary { get; set; }
        public string Risk { get; set; }
        public List<string> RiskReasons { get; set; } = new List<string>();
    }

    public sealed class ListEventsResult
    {
        public List<AuditEvent> Events { get; set; } = new List<AuditEvent>();
        public bool Available { get; set; }
        public string Window { get; set; }
        public int Max { get; set; }
        public int Scanned { get; set; }
        public bool Truncated { get; set; }
        public string Message { get; set; }
        public List<string> Warnings { get; set; } = new List<string>();
        public string GeneratedAt { get; set; }
    }
}
