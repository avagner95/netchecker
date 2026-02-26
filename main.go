package main

import (
	"embed"
	_ "embed"
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

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend.
// See https://pkg.go.dev/embed for more information.

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	// Register a custom event whose associated data type is string.
	// This is not required, but the binding generator will pick up registered events
	// and provide a strongly typed JS/TS API for them.
	application.RegisterEvent[string]("time")
	application.RegisterEvent[string]("app:size")
	application.RegisterEvent[bool]("app:running")
}

// main function serves as the application's entry point. It initializes the application, creates a window,
// and starts a goroutine that emits a time-based event every second. It subsequently runs the application and
// logs any error that might occur.
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
		AlsoStdout: true, // dev
	})
	if err != nil {
		log.Fatal(err)
	}

	// Create a new Wails application by providing the necessary options.
	// Variables 'Name' and 'Description' are for application metadata.
	// 'Assets' configures the asset server with the 'FS' variable pointing to the frontend files.
	// 'Bind' is a list of Go struct instances. The frontend has access to the methods of these instances.
	// 'Mac' options tailor the application when running an macOS.
	// svc, err := appsvc.NewApp(AppName)
	app := application.New(application.Options{
		Name:        AppName,
		Description: "A demo of using raw HTML & CSS",
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
	mainWindow.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		mainWindow.Hide()
		e.Cancel()
	})

	tray := app.SystemTray.New()
	// TODO change icons
	if runtime.GOOS == "darwin" {

		tray.SetTemplateIcon(icons.WailsLogoWhiteTransparent)
	} else {
		tray.SetIcon(icons.WailsLogoBlackTransparent)
	}

	menu := app.NewMenu()

	menu.Add("Open / Close").OnClick(func(ctx *application.Context) {
		if mainWindow.IsVisible() {
			mainWindow.Hide()
		} else {
			mainWindow.Show()
			mainWindow.Focus()
		}
	})

	menu.AddSeparator()

	menu.Add("Quit").OnClick(func(ctx *application.Context) {
		app.Quit()
	})

	tray.SetMenu(menu)

	tray.AttachWindow(mainWindow)
	// Create a new window with the necessary options.
	// 'Title' is the title of the window.
	// 'Mac' options tailor the window when running on macOS.
	// 'BackgroundColour' is the background colour of the window.
	// 'URL' is the URL that will be loaded into the webview.
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "NetChecker",
		Hidden: true,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(27, 38, 54),
		URL:              "/",
	})

	// Create a goroutine that emits an event containing the current time every second.
	// The frontend can listen to this event and update the UI accordingly.
	go func() {
		for {
			now := time.Now().Format(time.RFC1123)

			app.Event.Emit("time", now)
			time.Sleep(time.Second)
		}
	}()
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

	// Run the application. This blocks until the application has been exited.
	err = app.Run()

	// If an error occurred while running the application, log it and exit.
	if err != nil {
		log.Fatal(err)
	}
}
