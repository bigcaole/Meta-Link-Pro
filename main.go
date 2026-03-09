package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"

	"meta-link-pro/backend"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	appService := backend.NewApp()

	app := application.New(application.Options{
		Name:        "Meta-Link Pro",
		Description: "Proxy link to OpenClash Meta (Mihomo) YAML desktop converter",
		Services: []application.Service{
			application.NewService(appService),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Meta-Link Pro",
		Width:            1320,
		Height:           860,
		MinWidth:         1100,
		MinHeight:        720,
		BackgroundColour: application.NewRGB(15, 23, 42),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
