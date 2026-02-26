package app

import (
	"context"
	"netchecker/internal/config"
	"netchecker/internal/monitor"
	"netchecker/internal/storage"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type App struct {
	mu      sync.RWMutex
	ctx     context.Context
	wails   *application.App
	version string
	running bool
	AppDir  string
	cfg     config.Config
	cfgPath string
	DbPath  string
	store   *storage.SQLiteStore
	mon     *monitor.Monitor
	cancel  context.CancelFunc
}
