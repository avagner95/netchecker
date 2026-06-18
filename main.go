package main

import (
	"embed"
	"fmt"
	"log"
	appsvc "netchecker/internal/app"
	"netchecker/internal/helpers"
	"netchecker/internal/logging"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

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

	cmd := NCApp.IsRunning()

	_, err = logging.Init(logging.Options{
		LogDir:     NCApp.AppDir,
		Filename:   "netchecker.log",
		MaxSizeMB:  10,
		MaxBackups: 10,
		Compress:   true,
		AlsoStdout: true,
		Running:    cmd,
	})
	if err != nil {
		log.Fatal(err)
	}

	var mainWindow *application.WebviewWindow
	var app *application.App

	showAndFocus := func() {
		if mainWindow == nil || app == nil {
			return
		}

		app.Show()
		if mainWindow.IsMinimised() {
			mainWindow.Restore()
		}
		if !mainWindow.IsVisible() {
			mainWindow.Show()
		}
		mainWindow.Focus()
	}

	hideToTray := func() {
		if mainWindow == nil || app == nil {
			return
		}

		time.AfterFunc(10*time.Millisecond, func() {
			mainWindow.Hide()
			app.Hide()
		})
	}

	runCommand := func(args []string, workingDir string) {
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
			outPath := resolveCommandPath(args[2], workingDir)
			if _, err := NCApp.ExportAllToCSVGZ(outPath); err != nil {
				log.Printf("export: %v", err)
				return
			}
			log.Printf("export: saved %s", outPath)

		case "upload-alfadisk":
			filename, err := NCApp.ExportAndUploadToConfiguredAlfaDisk()
			if err != nil {
				log.Printf("%s: %v", cmd, err)
				return
			}
			log.Printf("%s: uploaded %s", cmd, filename)

		case "help", "-h", "--help":
			log.Print(commandUsage())

		default:
			showAndFocus()
		}
	}

	app = application.New(application.Options{
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
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},

		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.netchecker.app",
			OnSecondInstanceLaunch: func(d application.SecondInstanceData) {

				go func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("command panic: %v", r)
						}
					}()
					runCommand(d.Args, d.WorkingDir)
				}()
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
			HiddenOnTaskbar: false,
		},
	})

	mainWindow.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		hideToTray()
	})

	mainWindow.RegisterHook(events.Common.WindowMinimise, func(e *application.WindowEvent) {
		e.Cancel()
		hideToTray()
	})

	tray := app.SystemTray.New()
	tray.SetIcon(appIcon)

	menu := app.NewMenu()

	tray.OnClick(func() { showAndFocus() })
	tray.OnDoubleClick(func() { showAndFocus() })

	menu.Add("Open").OnClick(func(ctx *application.Context) { showAndFocus() })
	menu.Add("Hide").OnClick(func(ctx *application.Context) { hideToTray() })
	menu.Add("Quit").OnClick(func(ctx *application.Context) { app.Quit() })

	tray.SetMenu(menu)

	if len(os.Args) >= 2 {
		wd, _ := os.Getwd()
		runCommand(os.Args, wd)
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

func resolveCommandPath(path string, workingDir string) string {
	if filepath.IsAbs(path) || workingDir == "" {
		return path
	}
	return filepath.Join(workingDir, path)
}

func commandUsage() string {
	return fmt.Sprintf("usage: netchecker <command>\ncommands:\n  start\n  stop\n  export <path.csv.gz>\n  upload-alfadisk")
}
