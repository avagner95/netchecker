package model

import (
	"sync"
)

type App struct {
	mu      sync.RWMutex
	version string
	cfg     Config
	cfgPath string
	dbPath  string
}
