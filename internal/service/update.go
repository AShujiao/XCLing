package service

// 检查更新（UpdateService）：
//   - 仅当用户主动点击「检查更新」时访问 GitHub Releases API（无启动联网、无遥测）；
//   - 网络逻辑放在 Go 侧：Go 的 crypto/tls 不依赖系统 Schannel，Win7（Go 1.20.14 变体）
//     无需任何 TLS 注册表补丁即可访问 GitHub（GitHub 要求 TLS 1.2+）；
//   - 超时、失败一律返回可读错误，绝不阻塞或影响其他服务。
//
// 配套发布约定：GitHub Release 的 tag_name 与仓库根 VERSION 对齐（可带 v 前缀，
// 如 v0.3.16）；安装包资产由 build-installer.ps1 产出后随 release 上传。

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"XCLing/internal/model"
)

const (
	updateRepo    = "AShujiao/XCLing"
	updateTimeout = 8 * time.Second
	updateMaxBody = 4 << 20 // 4MB，防畸形响应撑爆内存
)

// updateEndpoint 为最新 release 查询端点（const 拼接，便于测试覆写字段时对比）。
const updateEndpoint = "https://api.github.com/repos/" + updateRepo + "/releases/latest"

// githubRelease 是 GitHub Releases API 响应的最小只读视图。
type githubRelease struct {
	TagName     string `json:"tag_name"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
	HTMLURL     string `json:"html_url"`
	Assets      []struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

// UpdateService 提供基于 GitHub Releases 的检查更新能力。
type UpdateService struct {
	client         *http.Client
	endpoint       string
	now            func() time.Time
	currentVersion string // 本地版本，默认 model.AppVersion；测试可覆写
}

// NewUpdateService 构造检查更新服务。
func NewUpdateService() *UpdateService {
	return &UpdateService{
		client:         &http.Client{Timeout: updateTimeout},
		endpoint:       updateEndpoint,
		now:            time.Now,
		currentVersion: model.AppVersion,
	}
}

// CheckUpdate 查询最新 release 并与本地版本比较，返回只读结果。
// 任何网络/解析/状态码失败都以 "UPDATE_CHECK_FAILED: ..." 返回可读错误。
func (s *UpdateService) CheckUpdate() (model.UpdateInfo, error) {
	current := s.currentVersion
	info := model.UpdateInfo{
		CurrentVersion: current,
		CheckedAt:      s.now().Local().Format(time.RFC3339),
	}

	req, err := http.NewRequest(http.MethodGet, s.endpoint, nil)
	if err != nil {
		return info, fmt.Errorf("UPDATE_CHECK_FAILED: 构造请求失败: %v", err)
	}
	req.Header.Set("User-Agent", "XCLing/"+model.AppVersion+" (update-check)") // GitHub 强制要求 UA
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := s.client.Do(req)
	if err != nil {
		return info, fmt.Errorf("UPDATE_CHECK_FAILED: 网络请求失败: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, updateMaxBody))
	if err != nil {
		return info, fmt.Errorf("UPDATE_CHECK_FAILED: 读取响应失败: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		// 常见原因：仓库不存在/私有（404），或匿名限流（403）。
		return info, fmt.Errorf("UPDATE_CHECK_FAILED: GitHub 返回 %s（请确认仓库为公开）", resp.Status)
	}

	var release githubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return info, fmt.Errorf("UPDATE_CHECK_FAILED: 解析响应失败: %v", err)
	}

	latest := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
	info.LatestVersion = latest
	info.PublishedAt = release.PublishedAt
	info.ReleaseNotes = release.Body
	info.ReleaseURL = release.HTMLURL
	if len(release.Assets) > 0 {
		best := release.Assets[0]
		for _, a := range release.Assets[1:] {
			if a.Size > best.Size {
				best = a
			}
		}
		info.AssetName = best.Name
		info.AssetSize = best.Size
	}

	info.HasUpdate = compareVersions(latest, current) > 0
	return info, nil
}

// compareVersions 三段式比较：a>b 返回正数，a<b 返回负数，相等返回 0。
// 任一版本解析失败视为相等（宁可不提示，也不误报更新）。
func compareVersions(a, b string) int {
	pa, oka := parseVersion(a)
	pb, okb := parseVersion(b)
	if !oka || !okb {
		return 0
	}
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			if pa[i] > pb[i] {
				return 1
			}
			return -1
		}
	}
	return 0
}

// parseVersion 解析 "x.y.z"（允许 1~3 段，纯数字），失败返回 false。
func parseVersion(v string) ([3]int, bool) {
	parts := strings.Split(v, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return [3]int{}, false
	}
	var out [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 0 {
			return [3]int{}, false
		}
		out[i] = n
	}
	return out, true
}
