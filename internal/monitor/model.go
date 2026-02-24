package monitor

import (
	"netchecker/internal/config"
	"netchecker/internal/storage"
	"sync"
	"time"
)

type mode int

type traceJob struct {
	name    string
	address string
	reason  string // "start" | "loss" | "high_rtt"
}
type targetState struct {
	mode          mode
	cooldownUntil time.Time

	buf   []ringSample
	next  int
	count int
}

type ringSample struct {
	ok    bool
	rttMs int
}

type Monitor struct {
	store *storage.SQLiteStore

	mu  sync.RWMutex
	cfg config.Config

	// runtime per-target state (keyed by address, plus "gateway")
	stMu   sync.Mutex
	states map[string]*targetState

	// traceroute queue
	traceQ chan traceJob

	// trace dedup
	ptMu         sync.Mutex
	pendingTrace map[string]struct{}

	// trace workers status
	traceWorkers int
	activeTrace  int64

	// gateway runtime
	gwIP         string
	gwFailStreak int

	// throttled logs
	logMu        sync.Mutex
	lastPingLog  time.Time
	lastTraceLog time.Time
	lastWarnLog  time.Time
}
