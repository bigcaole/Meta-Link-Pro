package backend

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"meta-link-pro/backend/engine"
	"meta-link-pro/backend/models"
)

const (
	updateCheckTimeout   = 10 * time.Second
	updateProbeMaxBytes  = 64 * 1024
	versionCacheDirName  = ".meta-link-pro"
	versionCacheFileName = "dependency_versions.json"
)

type dependencyTarget struct {
	Name string
	URL  string
}

type dependencyVersionCache struct {
	UpdatedAt string            `json:"updatedAt"`
	Markers   map[string]string `json:"markers"`
}

var updateHTTPClient = &http.Client{
	Timeout: updateCheckTimeout,
}

func (a *App) startUpdateCheck() {
	a.updateMu.Lock()
	if a.updateStatus.Running || a.updateStarted {
		a.updateMu.Unlock()
		return
	}
	a.updateStarted = true
	a.updateStatus = models.UpdateStatus{
		Running:   true,
		Completed: false,
		Progress:  0,
		Message:   "准备检查依赖版本...",
		StartedAt: time.Now().Format(time.RFC3339),
		Steps:     []models.UpdateStep{},
	}
	a.updateMu.Unlock()

	go a.runUpdateCheck()
}

func (a *App) runUpdateCheck() {
	targets := buildDependencyTargets()
	total := len(targets)
	if total == 0 {
		a.finishUpdateCheck("未找到可检查的依赖源", 0)
		return
	}

	cache, cachePath, cacheErr := loadDependencyVersionCache()
	if cacheErr != nil {
		cache = dependencyVersionCache{Markers: map[string]string{}}
	}

	latestCount := 0
	updatedCount := 0
	failures := 0

	for idx, target := range targets {
		a.updateMu.Lock()
		a.updateStatus.Message = fmt.Sprintf("检查版本 %d/%d: %s", idx+1, total, target.Name)
		a.updateStatus.Steps = append(a.updateStatus.Steps, models.UpdateStep{
			Name:   target.Name,
			URL:    target.URL,
			Status: "running",
			Detail: "检查中...",
		})
		stepIdx := len(a.updateStatus.Steps) - 1
		a.updateStatus.Progress = int(float64(idx) / float64(total) * 100)
		a.updateMu.Unlock()

		marker, err := fetchLatestVersionMarker(target.URL)
		a.updateMu.Lock()
		if err != nil {
			a.updateStatus.Steps[stepIdx].Status = "failed"
			a.updateStatus.Steps[stepIdx].Detail = err.Error()
			failures++
		} else {
			old := strings.TrimSpace(cache.Markers[target.URL])
			if old != "" && old == marker {
				a.updateStatus.Steps[stepIdx].Status = "latest"
				a.updateStatus.Steps[stepIdx].Detail = "已是最新版本"
				latestCount++
			} else {
				cache.Markers[target.URL] = marker
				a.updateStatus.Steps[stepIdx].Status = "updated"
				if old == "" {
					a.updateStatus.Steps[stepIdx].Detail = "已记录当前最新版本"
				} else {
					a.updateStatus.Steps[stepIdx].Detail = "检测到新版本，已更新本地版本记录"
				}
				updatedCount++
			}
		}
		a.updateStatus.Progress = int(float64(idx+1) / float64(total) * 100)
		a.updateMu.Unlock()
	}

	if cachePath != "" {
		cache.UpdatedAt = time.Now().Format(time.RFC3339)
		if err := saveDependencyVersionCache(cachePath, cache); err != nil {
			failures++
		}
	}

	message := fmt.Sprintf("版本检查完成：最新 %d，更新 %d，失败 %d", latestCount, updatedCount, failures)
	a.finishUpdateCheck(message, failures)
}

func (a *App) finishUpdateCheck(message string, failures int) {
	a.updateMu.Lock()
	defer a.updateMu.Unlock()
	a.updateStatus.Running = false
	a.updateStatus.Completed = true
	a.updateStatus.Progress = 100
	a.updateStatus.Message = message
	a.updateStatus.FinishedAt = time.Now().Format(time.RFC3339)
	if failures > 0 {
		a.updateStatus.Message = message + "（可继续使用）"
	}
}

func (a *App) isUpdateCheckCompleted() bool {
	a.updateMu.RLock()
	defer a.updateMu.RUnlock()
	return a.updateStatus.Completed
}

func (a *App) copyUpdateStatus() models.UpdateStatus {
	a.updateMu.RLock()
	defer a.updateMu.RUnlock()
	out := a.updateStatus
	out.Steps = append([]models.UpdateStep(nil), a.updateStatus.Steps...)
	return out
}

func buildDependencyTargets() []dependencyTarget {
	targets := []dependencyTarget{
		{
			Name: "GeoSite 数据",
			URL:  "https://github.com/MetaCubeX/meta-rules-dat/releases/latest/download/geosite.dat",
		},
		{
			Name: "GeoIP 数据",
			URL:  "https://github.com/MetaCubeX/meta-rules-dat/releases/latest/download/geoip.dat",
		},
		{
			Name: "MMDB 数据",
			URL:  "https://github.com/MetaCubeX/meta-rules-dat/releases/latest/download/country.mmdb",
		},
	}

	for _, ruleURL := range representativeRuleSources(engine.RuleProviderURLs()) {
		name := "规则集: " + sourceNameFromURL(ruleURL)
		targets = append(targets, dependencyTarget{Name: name, URL: ruleURL})
	}
	return targets
}

func sourceNameFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "unknown"
	}
	parts := strings.Split(raw, "/")
	if len(parts) < 2 {
		return raw
	}
	return parts[len(parts)-2] + "/" + parts[len(parts)-1]
}

func fetchLatestVersionMarker(rawURL string) (string, error) {
	marker, err := requestVersionMarker(rawURL, http.MethodHead)
	if err == nil && marker != "" {
		return marker, nil
	}
	marker, getErr := requestVersionMarker(rawURL, http.MethodGet)
	if getErr != nil {
		if err != nil {
			return "", err
		}
		return "", getErr
	}
	if marker == "" {
		return "", fmt.Errorf("无法获取版本标识")
	}
	return marker, nil
}

func requestVersionMarker(rawURL string, method string) (string, error) {
	req, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("URL 无效")
	}
	req.Header.Set("User-Agent", "Meta-Link-Pro/1.0")
	if method == http.MethodGet {
		req.Header.Set("Range", "bytes=0-1023")
	}

	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("访问失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		if resp.StatusCode != http.StatusPartialContent {
			return "", fmt.Errorf("HTTP 状态异常: %d", resp.StatusCode)
		}
	}

	body := []byte(nil)
	if method == http.MethodGet {
		limited, readErr := io.ReadAll(io.LimitReader(resp.Body, updateProbeMaxBytes))
		if readErr != nil {
			return "", fmt.Errorf("读取失败: %v", readErr)
		}
		body = limited
	}

	marker := extractVersionMarker(resp, body)
	if marker == "" {
		return "", fmt.Errorf("无法读取版本标识")
	}
	return marker, nil
}

func extractVersionMarker(resp *http.Response, body []byte) string {
	candidates := []string{
		resp.Header.Get("ETag"),
		resp.Header.Get("Last-Modified"),
		resp.Header.Get("X-Linked-ETag"),
		resp.Header.Get("Content-Disposition"),
	}
	for _, item := range candidates {
		item = strings.TrimSpace(strings.Trim(item, "\""))
		if item != "" {
			return item
		}
	}
	if len(body) > 0 {
		h := fnv.New64a()
		_, _ = h.Write(body)
		return fmt.Sprintf("body:%x", h.Sum64())
	}
	if resp.ContentLength > 0 {
		return fmt.Sprintf("len:%d", resp.ContentLength)
	}
	return ""
}

func loadDependencyVersionCache() (dependencyVersionCache, string, error) {
	path, err := dependencyVersionCachePath()
	if err != nil {
		return dependencyVersionCache{}, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return dependencyVersionCache{Markers: map[string]string{}}, path, nil
		}
		return dependencyVersionCache{}, path, err
	}
	var cache dependencyVersionCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return dependencyVersionCache{Markers: map[string]string{}}, path, nil
	}
	if cache.Markers == nil {
		cache.Markers = map[string]string{}
	}
	return cache, path, nil
}

func saveDependencyVersionCache(path string, cache dependencyVersionCache) error {
	payload, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

func dependencyVersionCachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, versionCacheDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, versionCacheFileName), nil
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func representativeRuleSources(urls []string) []string {
	out := make([]string, 0)
	seenRepo := make(map[string]struct{})
	for _, raw := range uniqueStrings(urls) {
		parsed, err := url.Parse(raw)
		if err != nil {
			continue
		}
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		repoKey := parsed.Host
		if len(parts) >= 2 {
			repoKey = parsed.Host + "/" + parts[0] + "/" + parts[1]
		}
		if _, ok := seenRepo[repoKey]; ok {
			continue
		}
		seenRepo[repoKey] = struct{}{}
		out = append(out, raw)
	}
	sort.Strings(out)
	return out
}
