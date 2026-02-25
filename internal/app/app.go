package app

import (
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
		AppDir:  appDir,
		cfgPath: path,
		DbPath:  dbPath,
		store:   st,
	}
	a.mon = monitor.NewMonitor(st, cfg)
	return a, nil
}
