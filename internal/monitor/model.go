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
type Status struct {
	TraceWorkers       int   `json:"traceWorkers"`
	ActiveTraceWorkers int64 `json:"activeTraceWorkers"`
	TraceQueueSize     int   `json:"traceQueueSize"`
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

type Target struct {
	Enabled      bool   `json:"enabled"`
	TraceEnabled bool   `json:"traceEnabled"`
	Name         string `json:"name"`
	Address      string `json:"address"` // ip or hostname
}

type TraceLossTrigger struct {
	Enabled bool `json:"enabled"`
	Percent int  `json:"percent"` // 0..100
	LastN   int  `json:"lastN"`   // window size
}

type TraceHighRTTTrigger struct {
	Enabled bool `json:"enabled"`
	RTTms   int  `json:"rttMs"`   // threshold
	Percent int  `json:"percent"` // 0..100
	LastN   int  `json:"lastN"`   // window size
}

type TraceTriggers struct {
	OnStart     bool                `json:"onStart"`
	Loss        TraceLossTrigger    `json:"loss"`
	HighRTT     TraceHighRTTTrigger `json:"highRtt"`
	CooldownSec int                 `json:"cooldownSec"` // 300 by default
}
type PingOut struct {
	OK    bool
	TTL   *int
	RTTms *int // integer ms
	Err   string
}
type TraceOut struct {
	OK   bool
	Text string
	Err  string // "timeout" | "canceled" | "exit" | "spawn"
}
type PingSettings struct {
	IntervalMs int `json:"intervalMs"`
	TimeoutMs  int `json:"timeoutMs"`
	Payload    int `json:"payload"` // best-effort for different OS
}

type GatewaySettings struct {
	Enabled bool `json:"enabled"`
}

type Config struct {
	Version int             `json:"version"`
	Ping    PingSettings    `json:"ping"`
	Gateway GatewaySettings `json:"gateway"`

	Targets []Target      `json:"targets"`
	Trace   TraceTriggers `json:"trace"`
}
