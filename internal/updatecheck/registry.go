package updatecheck

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	fetchTimeout = 5 * time.Second
	maxBody      = 256 << 10 // 256 KB
)

// registryURL 是 npm dist-tag latest 端点。测试中可覆盖。
var registryURL = "https://registry.npmjs.org/shoplazza-cli/latest"

// DefaultClient 覆盖 HTTP client(测试用)。nil → 带超时的默认 client。
var DefaultClient *http.Client

func httpClient() *http.Client {
	if DefaultClient != nil {
		return DefaultClient
	}
	return &http.Client{Timeout: fetchTimeout}
}

type npmLatestResponse struct {
	Version string `json:"version"`
}

// fetchLatest 请求 npm registry latest 端点,返回版本字符串。
func fetchLatest() (string, error) {
	resp, err := httpClient().Get(registryURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("npm registry: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return "", err
	}
	var r npmLatestResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return "", err
	}
	if r.Version == "" {
		return "", fmt.Errorf("npm registry: empty version")
	}
	return r.Version, nil
}
