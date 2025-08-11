package main

import (
	"embed"
	"log"

	"github.com/vanpelt/catnip/internal/services"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// We embed the built React frontend from the dist directory.
//
//go:embed all:../../internal/assets/dist
var assets embed.FS

func main() {
	// Initialize the existing container services
	// These will be wrapped by our Wails services
	gitService := services.NewGitService()
	claudeService := services.NewClaudeService()
	sessionService := services.NewSessionService()

	// Create the Wails application
	app := application.New(application.Options{
		Name:        "Catnip",
		Description: "Agentic Coding Environment - Desktop Edition",
		Services: []application.Service{
			// Core services that expose existing functionality
			application.NewService(&ClaudeDesktopService{claude: claudeService}),
			application.NewService(&GitDesktopService{git: gitService}),
			application.NewService(&SessionDesktopService{session: sessionService}),
			application.NewService(&SettingsDesktopService{}),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	// Create the main application window
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "Catnip - Agentic Coding Environment",
		Width:     1400,
		Height:    900,
		MinWidth:  800,
		MinHeight: 600,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(15, 23, 42), // Slate-900 from Tailwind
		URL:              "/",
	})

	// Initialize services and start any necessary background processes
	go func() {
		// Initialize any background monitoring or services
		// This could include file watching, git status monitoring, etc.
	}()

	// Run the application
	err := app.Run()
	if err != nil {
		log.Fatal(err)
	}
}
