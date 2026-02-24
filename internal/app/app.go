package app

import (
	"netchecker/internal/config"
	"netchecker/internal/monitor"
	"netchecker/internal/storage"
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

	a := &App{
		cfg:     cfg,
		cfgPath: path,
		dbPath:  dbPath,
		store:   st,
		running: false,
	}
	a.mon = monitor.NewMonitor(st, cfg)
	return a, nil
}
