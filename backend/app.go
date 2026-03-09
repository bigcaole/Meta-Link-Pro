package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"meta-link-pro/backend/engine"
	"meta-link-pro/backend/models"
	"meta-link-pro/backend/services"
)

// App is the Wails service entrypoint.
type App struct {
	updateMu      sync.RWMutex
	updateStatus  models.UpdateStatus
	updateStarted bool
}

func NewApp() *App {
	app := &App{}
	app.startUpdateCheck()
	return app
}

func (a *App) ParseLinks(input string) models.ParseReport {
	if !a.isUpdateCheckCompleted() {
		return models.ParseReport{
			Errors: []models.ParseIssue{
				{
					Protocol: "INIT",
					Field:    "update",
					Message:  "依赖版本检查尚未完成，请稍候再解析",
				},
			},
		}
	}
	return engine.ParseInput(input)
}

func (a *App) LoadServiceTree() ([]models.ServiceTree, error) {
	return services.LoadServiceTree()
}

func (a *App) GenerateMetaYAML(req models.GenerateMetaYAMLRequest) (string, error) {
	if err := a.ensureReadyForOperations(); err != nil {
		return "", err
	}
	return engine.GenerateMetaYAML(req)
}

func (a *App) ExportToDesktop(content string) (string, error) {
	if err := a.ensureReadyForOperations(); err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("无法获取用户目录: %w", err)
	}
	desktop := filepath.Join(home, "Desktop")
	if _, err := os.Stat(desktop); err != nil {
		return "", fmt.Errorf("桌面目录不存在: %w", err)
	}

	filename := fmt.Sprintf("meta-link-pro-%s.yaml", time.Now().Format("20060102-150405"))
	path := filepath.Join(desktop, filename)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("导出失败: %w", err)
	}
	return path, nil
}

func (a *App) StartUpdateCheck() models.UpdateStatus {
	a.startUpdateCheck()
	return a.GetUpdateStatus()
}

func (a *App) GetUpdateStatus() models.UpdateStatus {
	return a.copyUpdateStatus()
}

func (a *App) ensureReadyForOperations() error {
	if a.isUpdateCheckCompleted() {
		return nil
	}
	return fmt.Errorf("依赖版本检查尚未完成，请稍候")
}
