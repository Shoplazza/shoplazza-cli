package updatecheck

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// setup 把配置目录重定向到临时目录,并清空会触发跳过的环境变量。
func setup(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	orig := osUserConfigDir
	osUserConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { osUserConfigDir = orig })
	t.Setenv("SHOPLAZZA_CLI_NO_UPDATE_CHECK", "")
	t.Setenv("CI", "")
	t.Setenv("BUILD_NUMBER", "")
	t.Setenv("RUN_ID", "")
	return dir
}

func writeCache(t *testing.T, dir string, s state) {
	t.Helper()
	p := filepath.Join(dir, "shoplazza-cli", "update-check.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(s)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCheckCached_NewerAvailable(t *testing.T) {
	dir := setup(t)
	writeCache(t, dir, state{LatestVersion: "2.5.0", CheckedAt: time.Now().Unix()})
	info := CheckCached("2.0.0")
	if info == nil || info.Latest != "2.5.0" || info.Current != "2.0.0" {
		t.Fatalf("got %+v want update to 2.5.0", info)
	}
}

func TestCheckCached_UpToDate(t *testing.T) {
	dir := setup(t)
	writeCache(t, dir, state{LatestVersion: "2.0.0", CheckedAt: time.Now().Unix()})
	if info := CheckCached("2.0.0"); info != nil {
		t.Fatalf("got %+v want nil (up to date)", info)
	}
}

func TestCheckCached_NoCache(t *testing.T) {
	setup(t)
	if info := CheckCached("2.0.0"); info != nil {
		t.Fatalf("got %+v want nil (no cache)", info)
	}
}

func TestCheckCached_SkipsOnOptOut(t *testing.T) {
	dir := setup(t)
	writeCache(t, dir, state{LatestVersion: "2.5.0", CheckedAt: time.Now().Unix()})
	t.Setenv("SHOPLAZZA_CLI_NO_UPDATE_CHECK", "1")
	if info := CheckCached("2.0.0"); info != nil {
		t.Fatalf("got %+v want nil (opt-out)", info)
	}
}

func TestCheckCached_SkipsDevVersion(t *testing.T) {
	dir := setup(t)
	writeCache(t, dir, state{LatestVersion: "2.5.0", CheckedAt: time.Now().Unix()})
	if info := CheckCached("dev"); info != nil {
		t.Fatalf("got %+v want nil (dev version)", info)
	}
}

func TestRefreshCache_FreshIsNoop(t *testing.T) {
	dir := setup(t)
	writeCache(t, dir, state{LatestVersion: "1.0.0", CheckedAt: time.Now().Unix()}) // 新鲜
	hit := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		_, _ = w.Write([]byte(`{"version":"9.9.9"}`))
	}))
	defer ts.Close()
	origURL, origClient := registryURL, DefaultClient
	registryURL, DefaultClient = ts.URL, ts.Client()
	defer func() { registryURL, DefaultClient = origURL, origClient }()

	RefreshCache("2.0.0")
	if hit {
		t.Error("RefreshCache 在缓存新鲜时仍联网")
	}
}

func TestRefreshCache_StaleFetches(t *testing.T) {
	dir := setup(t)
	writeCache(t, dir, state{LatestVersion: "1.0.0", CheckedAt: time.Now().Add(-48 * time.Hour).Unix()}) // 过期
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"version":"9.9.9"}`))
	}))
	defer ts.Close()
	origURL, origClient := registryURL, DefaultClient
	registryURL, DefaultClient = ts.URL, ts.Client()
	defer func() { registryURL, DefaultClient = origURL, origClient }()

	RefreshCache("2.0.0")

	s, err := loadState()
	if err != nil || s == nil || s.LatestVersion != "9.9.9" {
		t.Fatalf("缓存未更新: %+v err=%v", s, err)
	}
}
