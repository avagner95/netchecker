package main

import (
	"embed"
	"log"
	appsvc "netchecker/internal/app"
	"netchecker/internal/helpers"
	"netchecker/internal/logging"
	"runtime"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/icons"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	application.RegisterEvent[string]("app:size")
	application.RegisterEvent[bool]("app:running")
}

func main() {
	AppName := "netchecker"

	NCApp, err := appsvc.NewApp(AppName)
	if err != nil {
		log.Fatal(err)
	}

	_, err = logging.Init(logging.Options{
		LogDir:     NCApp.AppDir,
		Filename:   "netchecker.log",
		MaxSizeMB:  10,
		MaxBackups: 10,
		Compress:   true,
		AlsoStdout: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	var mainWindow *application.WebviewWindow

	// Helpers for consistent window behavior (Mac + Windows)
	showAndFocus := func() {
		if mainWindow == nil {
			return
		}
		if mainWindow.IsMinimised() {
			mainWindow.Restore()
		}
		if !mainWindow.IsVisible() {
			mainWindow.Show()
		}
		mainWindow.Focus()
	}

	hideToTray := func() {
		if mainWindow == nil {
			return
		}
		// Delay avoids races with OS/window manager (especially on macOS)
		time.AfterFunc(10*time.Millisecond, func() {
			mainWindow.Hide()
		})
	}

	app := application.New(application.Options{
		Name:        AppName,
		Description: "NetChecker",
		Services: []application.Service{
			application.NewService(&GreetService{}),
			application.NewService(NCApp),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},

		// IMPORTANT for tray apps on macOS: don't terminate when last window closes
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},

		// Built-in single instance (Wails v3alpha)
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.netchecker.app", // сделай уникальным (лучше reverse-domain)
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				// When second instance starts: just bring window to front
				showAndFocus()
			},
			// (Optional) EncryptionKey / AdditionalData — если понадобится
		},
	})

	mainWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "NetChecker",
		Name:   "main",
		URL:    "/",
		Hidden: true,

		BackgroundColour: application.NewRGB(27, 38, 54),

		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},

		Windows: application.WindowsWindow{
			HiddenOnTaskbar: true,
		},
	})

	// X (close) => hide to tray (not quit)
	mainWindow.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		hideToTray()
	})

	// Minimise (—) => hide to tray (not minimise)
	mainWindow.RegisterHook(events.Common.WindowMinimise, func(e *application.WindowEvent) {
		e.Cancel()
		hideToTray()
	})

	// --- Tray ---
	tray := app.SystemTray.New()
	if runtime.GOOS == "darwin" {
		tray.SetTemplateIcon(icons.WailsLogoWhiteTransparent)
	} else {
		tray.SetIcon(icons.WailsLogoBlackTransparent)
	}

	menu := app.NewMenu()

	// Click/double click on tray icon => show window
	tray.OnClick(func() { showAndFocus() })
	tray.OnDoubleClick(func() { showAndFocus() })

	menu.Add("Open").OnClick(func(ctx *application.Context) {
		showAndFocus()
	})
	menu.Add("Hide").OnClick(func(ctx *application.Context) {
		hideToTray()
	})

	menu.AddSeparator()

	menu.Add("Quit").OnClick(func(ctx *application.Context) {
		app.Quit()
	})

	tray.SetMenu(menu)
	// NOTE: intentionally NOT using tray.AttachWindow(mainWindow)

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		check := func() {
			size, err := helpers.FolderSize(NCApp.AppDir)
			if err != nil {
				return
			}
			app.Event.Emit("app:size", helpers.HumanBytes(size))
		}

		check()
		for range ticker.C {
			check()
		}
	}()

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
