package main

import (
	"embed"
	"log"
	appsvc "netchecker/internal/app"
	"netchecker/internal/helpers"
	"netchecker/internal/logging"
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
		// небольшая задержка снижает "гонки" на macOS
		time.AfterFunc(10*time.Millisecond, func() {
			mainWindow.Hide()
		})
	}

	runCommand := func(args []string) {
		// args: ["/path/to/netchecker", "start"] etc
		if len(args) < 2 {
			showAndFocus()
			return
		}
		cmd := args[1]

		switch cmd {
		case "start":
			ok := NCApp.Start()
			if !ok {
				log.Printf("start: already running")
			}

		case "stop":
			ok := NCApp.Stop()
			if !ok {
				log.Printf("stop: not running")
			}

		case "export":
			if len(args) < 3 {
				log.Printf("export: missing path (usage: netchecker export /path/file.csv.gz)")
				return
			}
			if _, err := NCApp.ExportAllToCSVGZ(args[2]); err != nil {
				log.Printf("export: %v", err)
			}

		default:
			showAndFocus()
		}
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

		// Для tray-режима на macOS обязательно false
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},

		// Built-in single instance (без портов!)
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.netchecker.app",
			OnSecondInstanceLaunch: func(d application.SecondInstanceData) {
				// Во 2-м запуске команды приходят сюда
				runCommand(d.Args)
			},
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

	// X => hide to tray
	mainWindow.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		hideToTray()
	})

	// — => hide to tray (чтобы не оставалось minimised)
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

	tray.OnClick(func() { showAndFocus() })
	tray.OnDoubleClick(func() { showAndFocus() })

	menu.Add("Open").OnClick(func(ctx *application.Context) { showAndFocus() })
	menu.Add("Hide").OnClick(func(ctx *application.Context) { hideToTray() })
	menu.Add("Quit").OnClick(func(ctx *application.Context) { app.Quit() })

	tray.SetMenu(menu)

	// Если это ПЕРВЫЙ запуск и он был с командой — выполним сразу.
	// Это полезно, когда GUI ещё не запущен, но ты делаешь: netchecker start
	if len(os.Args) >= 2 {
		runCommand(os.Args)
	}

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
