package app

import (
	"context"
	"fmt"
	"netchecker/internal/config"
	"netchecker/internal/monitor"
	"netchecker/internal/storage"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func NewApp(AppName string) (*App, error) {
	cfg, path, err := config.LoadOrCreate(config.DefaultConfig(), AppName)
	if err != nil {
		return nil, err
	}
	dbPath, err := config.DataDBPath(AppName)
	if err != nil {
		return nil, err
	}
	st, err := storage.OpenSQLite(dbPath)
	if err != nil {
		return nil, err
	}
	configDir, err := os.UserConfigDir()
	appDir := filepath.Join(configDir, AppName)
	a := &App{
		cfg:     cfg,
		version: Version,
		running: false,
		AppDir:  appDir,
		cfgPath: path,
		DbPath:  dbPath,
		store:   st,
	}
	a.mon = monitor.NewMonitor(st, cfg)
	return a, nil
}

func (a *App) IsRunning() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.running
}

func (a *App) Start() bool {

	a.mu.Lock()

	defer a.mu.Unlock()
	if a.running {
		return false
	}
	a.running = true
	app := application.Get()
	app.Event.Emit("app:running", a.running)

	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel

	if a.mon != nil {
		a.mon.UpdateConfig(a.cfg)
		go a.mon.Run(ctx)
	}
	return true
}

func (a *App) Stop() bool {
	a.mu.Lock()
	fmt.Println("stopping")
	defer a.mu.Unlock()
	if !a.running {
		return false
	}
	a.running = false
	app := application.Get()
	app.Event.Emit("app:running", a.running)
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	return true
}

func (a *App) GetConfig() config.Config {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.cfg
}
