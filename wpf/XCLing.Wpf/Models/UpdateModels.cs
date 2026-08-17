namespace XCLing.Wpf.Models
{
    // 与 internal/model/update.go 的 JSON 契约对应（检查更新结果视图）。

    public sealed class UpdateInfo
    {
        public string CurrentVersion { get; set; }
        public string LatestVersion { get; set; }
        public bool HasUpdate { get; set; }
        public string PublishedAt { get; set; }
        public string ReleaseUrl { get; set; }
        public string ReleaseNotes { get; set; }
        public string AssetName { get; set; }
        public long AssetSize { get; set; }
        public string CheckedAt { get; set; }
    }
}
