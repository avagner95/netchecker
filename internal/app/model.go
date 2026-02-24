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
	cfg     config.Config
	cfgPath string
	dbPath  string
	running bool
	store   *storage.SQLiteStore
	mon     *monitor.Monitor
	cancel  context.CancelFunc
}
