package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.3.16", "0.3.16", 0},
		{"0.3.17", "0.3.16", 1},
		{"0.3.16", "0.3.17", -1},
		{"0.3.10", "0.3.9", 1},    // 逐段数字比较，不是字符串比较
		{"0.3.16", "0.3", 1},      // 缺段视为 0
		{"0.3.16", "0.3.16.1", 0}, // 超过 3 段解析失败，视为相等（不误报）
		{"1.0.0", "0.99.99", 1},
		{"0.3.16-beta", "0.3.16", 0}, // 非数字后缀解析失败，视为相等
		{"", "0.3.16", 0},
		{"abc", "def", 0},
	}
	for _, c := range cases {
		got := compareVersions(c.a, c.b)
		if (got > 0) != (c.want > 0) || (got < 0) != (c.want < 0) || (got == 0) != (c.want == 0) {
			t.Errorf("compareVersions(%q, %q) = %d, want sign %d", c.a, c.b, got, c.want)
		}
	}
}

func TestParseVersion(t *testing.T) {
	for _, v := range []string{"0.3.16", "3", "1.2", "0.0.0", "  1 . 2 . 3 "} {
		if _, ok := parseVersion(v); !ok {
			t.Errorf("parseVersion(%q) unexpectedly failed", v)
		}
	}
	for _, v := range []string{"", "a.b.c", "1.2.3.4", "-1.0.0", "1.2.x", "v0.3.16"} {
		if _, ok := parseVersion(v); ok {
			t.Errorf("parseVersion(%q) unexpectedly succeeded", v)
		}
	}
}

// fakeGitHub 返回可定制的 latest release 响应，并把本地版本固定为 0.3.16。
func fakeGitHub(t *testing.T, status int, body string) (*httptest.Server, *UpdateService) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ua := r.Header.Get("User-Agent"); ua == "" {
			t.Error("request missing User-Agent header")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	svc := NewUpdateService()
	svc.endpoint = srv.URL
	svc.currentVersion = "0.3.16"
	return srv, svc
}

func TestCheckUpdateHasUpdate(t *testing.T) {
	srv, svc := fakeGitHub(t, http.StatusOK, `{
		"tag_name": "v0.3.17",
		"published_at": "2026-08-18T00:00:00Z",
		"html_url": "https://github.com/AShujiao/XCLing/releases/tag/v0.3.17",
		"body": "修复若干问题",
		"assets": [
			{"name": "README.md", "size": 100},
			{"name": "XCLing-Setup-0.3.17.exe", "size": 25165824}
		]
	}`)
	defer srv.Close()

	info, err := svc.CheckUpdate()
	if err != nil {
		t.Fatalf("CheckUpdate error: %v", err)
	}
	if !info.HasUpdate {
		t.Fatalf("expected HasUpdate=true, got false (latest=%q current=%q)", info.LatestVersion, info.CurrentVersion)
	}
	if info.LatestVersion != "0.3.17" {
		t.Errorf("LatestVersion = %q, want 0.3.17 (v 前缀应去掉)", info.LatestVersion)
	}
	if info.ReleaseURL == "" || info.AssetName != "XCLing-Setup-0.3.17.exe" || info.AssetSize != 25165824 {
		t.Errorf("unexpected payload: %+v", info)
	}
	if info.CheckedAt == "" {
		t.Error("CheckedAt must be set")
	}
}

func TestCheckUpdateAlreadyLatest(t *testing.T) {
	srv, svc := fakeGitHub(t, http.StatusOK, `{"tag_name": "0.3.16"}`)
	defer srv.Close()

	info, err := svc.CheckUpdate()
	if err != nil {
		t.Fatalf("CheckUpdate error: %v", err)
	}
	if info.HasUpdate {
		t.Fatalf("expected HasUpdate=false when versions equal")
	}
}

func TestCheckUpdateFailure(t *testing.T) {
	// 非 200：报可读错误，且带 UPDATE_CHECK_FAILED 前缀（GUI 错误码解析依赖该前缀）。
	srv, svc := fakeGitHub(t, http.StatusNotFound, `{"message": "Not Found"}`)
	defer srv.Close()

	_, err := svc.CheckUpdate()
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.HasPrefix(err.Error(), "UPDATE_CHECK_FAILED:") {
		t.Errorf("error must carry UPDATE_CHECK_FAILED prefix, got: %v", err)
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention status code, got: %v", err)
	}
}

func TestCheckUpdateNetworkError(t *testing.T) {
	// 连接被拒（端口必然空闲）：网络层错误同样以 UPDATE_CHECK_FAILED 返回。
	svc := NewUpdateService()
	svc.endpoint = "http://127.0.0.1:1/latest"
	_, err := svc.CheckUpdate()
	if err == nil {
		t.Fatal("expected network error")
	}
	if !strings.HasPrefix(err.Error(), "UPDATE_CHECK_FAILED:") {
		t.Errorf("network error must carry UPDATE_CHECK_FAILED prefix, got: %v", err)
	}
}

func TestCheckUpdateMalformedJSON(t *testing.T) {
	srv, svc := fakeGitHub(t, http.StatusOK, `{"tag_name": `) // 截断的 JSON
	defer srv.Close()
	if _, err := svc.CheckUpdate(); err == nil {
		t.Fatal("expected parse error")
	}
}

// 确保错误消息形状与 GUI 错误码解析一致（"CODE: 说明"）。
func TestUpdateErrorShape(t *testing.T) {
	srv, svc := fakeGitHub(t, http.StatusOK, `not json`)
	defer srv.Close()
	_, err := svc.CheckUpdate()
	if err == nil {
		t.Fatal("expected error")
	}
	if code, _, ok := strings.Cut(err.Error(), ": "); !ok || code != "UPDATE_CHECK_FAILED" {
		t.Errorf("expected 'CODE: message' shape, got: %v", err)
	}
}
