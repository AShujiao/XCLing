package model

// UpdateInfo 是「检查更新」的只读结果视图，数据来自 GitHub Releases API。
//
// 隐私边界：
//   - 仅当用户主动点击「检查更新」时才发起网络请求（无启动联网、无遥测）；
//   - 请求只带 User-Agent 与 Accept 头，不携带任何本机信息；
//   - 所有字段均来自 GitHub 公开响应，失败时 GUI 展示可读错误，不影响其他功能。
type UpdateInfo struct {
	CurrentVersion string `json:"currentVersion"` // 本地版本（model.AppVersion）
	LatestVersion  string `json:"latestVersion"`  // 远端最新版本（tag_name 去掉 v 前缀）
	HasUpdate      bool   `json:"hasUpdate"`      // 远端版本是否更新于本地
	PublishedAt    string `json:"publishedAt"`    // 发布时间 RFC3339，可空
	ReleaseURL     string `json:"releaseUrl"`     // Release 页面地址（浏览器打开下载）
	ReleaseNotes   string `json:"releaseNotes"`   // 更新说明（body），可为空
	AssetName      string `json:"assetName"`      // 安装包资产名，可为空
	AssetSize      int64  `json:"assetSize"`      // 安装包大小（字节），0 表示未知
	CheckedAt      string `json:"checkedAt"`      // 检查时间 RFC3339
}
