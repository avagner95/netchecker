package config

const FileConfig = "config.json"

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

type PingSettings struct {
	IntervalMs int `json:"intervalMs"`
	TimeoutMs  int `json:"timeoutMs"`
	Payload    int `json:"payload"` // best-effort for different OS
}

type GatewaySettings struct {
	Enabled bool `json:"enabled"`
}

type Config struct {
	Ping    PingSettings    `json:"ping"`
	Gateway GatewaySettings `json:"gateway"`

	Targets []Target      `json:"targets"`
	Trace   TraceTriggers `json:"trace"`
}
