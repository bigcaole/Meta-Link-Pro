package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"meta-link-pro/backend/engine"
	"meta-link-pro/backend/models"
	"meta-link-pro/backend/services"
)

// App is the Wails service entrypoint.
type App struct{}

func NewApp() *App {
	return &App{}
}

func (a *App) ParseLinks(input string) models.ParseReport {
	return engine.ParseInput(input)
}

func (a *App) LoadServiceTree() ([]models.ServiceTree, error) {
	return services.LoadServiceTree()
}

func (a *App) GenerateMetaYAML(req models.GenerateMetaYAMLRequest) (string, error) {
	return engine.GenerateMetaYAML(req)
}

func (a *App) ExportToDesktop(content string) (string, error) {
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
