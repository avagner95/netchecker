package app

import (
	"context"
	"netchecker/internal/config"
	"netchecker/internal/monitor"
	"netchecker/internal/storage"
	"os"
	"path/filepath"
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
	defer a.mu.Unlock()
	if !a.running {
		return false
	}
	a.running = false
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	return true
}
