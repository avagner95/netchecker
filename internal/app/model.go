package app

import (
	"context"
	"netchecker/internal/config"
	"netchecker/internal/monitor"
	"netchecker/internal/storage"
	"sync"
)

type App struct {
	mu      sync.RWMutex
	version string
	AppDir  string
	cfg     config.Config
	cfgPath string
	DbPath  string
	store   *storage.SQLiteStore
	mon     *monitor.Monitor
	cancel  context.CancelFunc
}
