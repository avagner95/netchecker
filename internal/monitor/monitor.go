package monitor

import (
	"netchecker/internal/config"
	"netchecker/internal/storage"
)

func NewMonitor(store *storage.SQLiteStore, cfg config.Config) *Monitor {
	return &Monitor{
		store:        store,
		cfg:          cfg,
		states:       make(map[string]*targetState),
		traceQ:       make(chan traceJob, 64),
		pendingTrace: make(map[string]struct{}),
		traceWorkers: 2,
	}
}
