package app

import (
	"context"
	"fmt"
	"log"
	"netchecker/internal/config"
	"netchecker/internal/monitor"
	"netchecker/internal/storage"
	"os"
	"path/filepath"
	"time"

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
func (a *App) SaveConfig(newCfg config.Config) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	// минимальная валидация
	if newCfg.Ping.IntervalMs <= 0 {
		newCfg.Ping.IntervalMs = a.cfg.Ping.IntervalMs
	}
	if newCfg.Ping.TimeoutMs <= 0 {
		newCfg.Ping.TimeoutMs = a.cfg.Ping.TimeoutMs
	}
	if newCfg.Ping.Payload < 0 {
		newCfg.Ping.Payload = a.cfg.Ping.Payload
	}

	if err := config.Save(a.cfgPath, newCfg); err != nil {
		return false
	}
	a.cfg = newCfg
	if a.mon != nil {
		a.mon.UpdateConfig(newCfg)
	}
	return true
}

// как у тебя уже есть
func (a *App) OnStartup(ctx context.Context) {
	log.Println("Startup called")
	a.ctx = ctx
	a.wails = application.Get() // <-- ВАЖНО: без аргументов в alpha.72
}

func (a *App) ExportAllToCSVGZWithDialog() (string, error) {
	a.mu.RLock()
	st := a.store
	app := a.wails
	a.mu.RUnlock()

	if st == nil {
		return "", fmt.Errorf("store is nil")
	}

	// если вдруг nil — попробуем получить ещё раз
	if app == nil {
		app = application.Get()
		if app == nil {
			return "", fmt.Errorf("wails app is nil (startup not completed)")
		}
		a.mu.Lock()
		a.wails = app
		a.mu.Unlock()
	}

	path, err := app.Dialog.SaveFile().
		SetFilename(fmt.Sprintf("netchecker_%s.csv.gz", time.Now().Format("20060102_150405"))).
		AddFilter("GZip CSV", "*.csv.gz").
		PromptForSingleSelection()
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil
	}

	return st.ExportMergedCSVGZ(context.Background(), path, 0, 0)
}

func (a *App) DashboardPoll(lastBucketMs int64) (*storage.DashboardResponse, error) {
	a.mu.RLock()
	st := a.store
	ctx := a.ctx
	a.mu.RUnlock()

	// 1) store must exist
	if st == nil {
		return nil, fmt.Errorf("store is nil (app not initialized)")
	}

	// 2) ctx might not be set yet if called too early
	if ctx == nil {
		ctx = context.Background()
	}

	// 3) (optional) ensure db is open
	if !st.IsReady() { // добавим метод ниже
		return nil, fmt.Errorf("db is not ready (sqlite not initialized)")
	}

	return st.DashboardPoll(ctx, lastBucketMs)
}
