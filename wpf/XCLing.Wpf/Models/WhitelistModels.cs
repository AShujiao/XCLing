using System.Collections.Generic;

namespace XCLing.Wpf.Models
{
    // 与 internal/model/whitelist.go 的 JSON 契约对应。
    // WhitelistDraft 本身不建模——作为不透明 JObject 在 GoApi 透传。

    public sealed class CustomPathEntry
    {
        public string Id { get; set; }
        public string Path { get; set; }
        public string Kind { get; set; }  // directory | file
        public string Label { get; set; }

        public string KindLabel => Kind == "file" ? "程序" : "目录";
    }

    public sealed class DiscoveredApp
    {
        public string Id { get; set; }
        public string DisplayName { get; set; }
        public string Publisher { get; set; }
        public string DisplayVersion { get; set; }
        public string InstallLocation { get; set; }
        public string DisplayIcon { get; set; }
        public string UninstallString { get; set; }
        public string Source { get; set; }
        public string ExecutablePath { get; set; }
        public string CandidatePath { get; set; }
        public bool CandidateIsDir { get; set; }
        public string Confidence { get; set; }
        public bool Selectable { get; set; }
        public List<string> Warnings { get; set; } = new List<string>();

        public string SourceLabel
        {
            get
            {
                switch (Source)
                {
                    case "HKLM64": return "HKLM·64位";
                    case "HKLM32": return "HKLM·32位";
                    case "HKCU": return "当前用户";
                    case "userCustom": return "手工添加";
                    case "mockDemo": return "演示数据";
                    default: return Source;
                }
            }
        }

        public string ConfidenceLabel
        {
            get
            {
                switch (Confidence)
                {
                    case "high": return "高";
                    case "medium": return "中";
                    default: return "低";
                }
            }
        }

        public string WarningText => Warnings != null && Warnings.Count > 0 ? string.Join("；", Warnings) : "";

        /// <summary>勾选后将新增的具体允许规则路径（目录\* 或精确 exe）。</summary>
        public string WillAllowRule
        {
            get
            {
                var path = (CandidatePath ?? "").TrimEnd('\\');
                return CandidateIsDir ? path + "\\*" : path;
            }
        }
    }
}
