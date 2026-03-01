package main

import (
	"embed"
	"log"
	appsvc "netchecker/internal/app"
	"netchecker/internal/helpers"
	"netchecker/internal/logging"
	"netchecker/internal/singleinstance"
	"os"
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

	// ---- SINGLE INSTANCE (до запуска Wails) ----
	// Уникальный и стабильный ID для mutex/lock + focus-file.
	const AppID = "netchecker.singleinstance.v1"

	release, err := singleinstance.Claim(AppID)
	if err == singleinstance.ErrAlreadyRunning {
		// попросим первый инстанс поднять окно и выйдем
		_ = singleinstance.RequestFocus(AppID)
		os.Exit(0)
	}
	if err != nil {
		log.Fatalf("singleinstance: %v", err)
	}
	defer func() { _ = release() }()

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
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	mainWindow := app.Window.NewWithOptions(application.WebviewWindowOptions{
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
		mainWindow.Hide()
		e.Cancel()
	})

	// ---- Focus server (слушает на 127.0.0.1:0 и пишет порт в cache-файл) ----
	stopFocus, err := singleinstance.StartFocusServer(AppID, func() {
		// Поднять/показать/сфокусировать окно
		if mainWindow.IsMinimised() {
			mainWindow.Restore()
		}
		if !mainWindow.IsVisible() {
			mainWindow.Show()
		}
		mainWindow.Focus()
	})
	if err != nil {
		// focus - это бонус, не критично
		log.Printf("focus server disabled: %v", err)
	} else {
		defer func() { _ = stopFocus() }()
	}

	// --- Tray ---
	tray := app.SystemTray.New()
	if runtime.GOOS == "darwin" {
		tray.SetTemplateIcon(icons.WailsLogoWhiteTransparent)
	} else {
		tray.SetIcon(icons.WailsLogoBlackTransparent)
	}

	menu := app.NewMenu()

	// Robust toggle:
	// - Minimized => Restore+Focus
	// - Visible (normal) => Hide
	// - Hidden => Show+Focus
	menu.Add("Open / Close").OnClick(func(ctx *application.Context) {

		// IMPORTANT: minimized windows are still "visible" on Windows
		if mainWindow.IsMinimised() {
			mainWindow.Restore()
			mainWindow.Focus()
			return
		}

		if mainWindow.IsVisible() {
			mainWindow.Hide()
			return
		}

		mainWindow.Show()
		mainWindow.Focus()
	})

	menu.AddSeparator()

	menu.Add("Quit").OnClick(func(ctx *application.Context) {
		app.Quit()
	})

	tray.SetMenu(menu)
	tray.AttachWindow(mainWindow)

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
