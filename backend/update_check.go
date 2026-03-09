package backend

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"meta-link-pro/backend/engine"
	"meta-link-pro/backend/models"
)

const (
	updateCheckTimeout  = 10 * time.Second
	updateProbeMaxBytes = 64 * 1024
)

type dependencyTarget struct {
	Name string
	URL  string
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
		Message:   "准备检查依赖更新...",
		StartedAt: time.Now().Format(time.RFC3339),
		Steps:     []models.UpdateStep{},
	}
	a.updateMu.Unlock()

	go a.runUpdateCheck()
}

func (a *App) runUpdateCheck() {
	targets := buildDependencyTargets()
	total := len(targets)
	failures := 0

	if total == 0 {
		a.finishUpdateCheck("未找到可检查的依赖源", 0)
		return
	}

	for idx, target := range targets {
		a.updateMu.Lock()
		a.updateStatus.Message = fmt.Sprintf("检查依赖 %d/%d: %s", idx+1, total, target.Name)
		a.updateStatus.Steps = append(a.updateStatus.Steps, models.UpdateStep{
			Name:   target.Name,
			URL:    target.URL,
			Status: "running",
			Detail: "检查中...",
		})
		stepIdx := len(a.updateStatus.Steps) - 1
		a.updateStatus.Progress = int(float64(idx) / float64(total) * 100)
		a.updateMu.Unlock()

		err := probeDependencyTarget(target.URL)
		a.updateMu.Lock()
		if err != nil {
			a.updateStatus.Steps[stepIdx].Status = "failed"
			a.updateStatus.Steps[stepIdx].Detail = err.Error()
			failures++
		} else {
			a.updateStatus.Steps[stepIdx].Status = "ok"
			a.updateStatus.Steps[stepIdx].Detail = "可用"
		}
		a.updateStatus.Progress = int(float64(idx+1) / float64(total) * 100)
		a.updateMu.Unlock()
	}

	if failures > 0 {
		a.finishUpdateCheck(fmt.Sprintf("更新检查完成，%d 个依赖源不可用", failures), failures)
		return
	}
	a.finishUpdateCheck("更新检查完成，依赖源状态正常", 0)
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
		// Check completed even with failures to avoid blocking users indefinitely.
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

func probeDependencyTarget(url string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("URL 无效")
	}
	req.Header.Set("User-Agent", "Meta-Link-Pro/1.0")
	req.Header.Set("Range", "bytes=0-1023")

	resp, err := updateHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("访问失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		if resp.StatusCode != http.StatusPartialContent {
			return fmt.Errorf("HTTP 状态异常: %d", resp.StatusCode)
		}
	}

	_, err = io.ReadAll(io.LimitReader(resp.Body, updateProbeMaxBytes))
	if err != nil {
		return fmt.Errorf("读取失败: %v", err)
	}
	return nil
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
