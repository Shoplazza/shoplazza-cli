package updatecheck

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	cacheTTL  = 24 * time.Hour
	stateFile = "update-check.json"
)

// osUserConfigDir 在测试中可覆盖。
var osUserConfigDir = os.UserConfigDir

// Info 描述一次可用更新。
type Info struct {
	Current string
	Latest  string
}

// Message 返回单行 stderr 提示文案。
func (i *Info) Message() string {
	return fmt.Sprintf("⚡ shoplazza-cli %s is available (current %s) — run 'shoplazza update'", i.Latest, i.Current)
}

type state struct {
	LatestVersion string `json:"latest_version"`
	CheckedAt     int64  `json:"checked_at"`
}

func statePath() (string, error) {
	dir, err := osUserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "shoplazza-cli", stateFile), nil
}

func loadState() (*state, error) {
	path, err := statePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s state
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func saveState(s *state) error {
	path, err := statePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// CheckCached 只读本地缓存(零网络)。当不跳过、缓存有 latest 且 latest 比 current 新时返回 Info。
func CheckCached(currentVersion string) *Info {
	if shouldSkip(currentVersion) {
		return nil
	}
	s, err := loadState()
	if err != nil || s == nil || s.LatestVersion == "" {
		return nil
	}
	if !IsNewer(s.LatestVersion, currentVersion) {
		return nil
	}
	return &Info{Current: currentVersion, Latest: s.LatestVersion}
}

// RefreshCache 在缓存过期(>24h)时联网抓取最新版并写回;新鲜或跳过时 no-op。
// 任何错误静默。可从 goroutine 调用。
func RefreshCache(currentVersion string) {
	if shouldSkip(currentVersion) {
		return
	}
	if s, err := loadState(); err == nil && s != nil && time.Since(time.Unix(s.CheckedAt, 0)) < cacheTTL {
		return // 新鲜
	}
	latest, err := fetchLatest()
	if err != nil {
		return
	}
	_ = saveState(&state{LatestVersion: latest, CheckedAt: time.Now().Unix()})
}

func shouldSkip(version string) bool {
	if os.Getenv("SHOPLAZZA_CLI_NO_UPDATE_CHECK") != "" {
		return true
	}
	if isCIEnv() {
		return true
	}
	if version == "" || version == "dev" || version == "DEV" {
		return true
	}
	if !isReleaseVersion(version) {
		return true
	}
	return false
}

func isCIEnv() bool {
	for _, k := range []string{"CI", "BUILD_NUMBER", "RUN_ID"} {
		if os.Getenv(k) != "" {
			return true
		}
	}
	return false
}
