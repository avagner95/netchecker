package confilg

import model "netchecker/internal/models"

func DefaultConfig() model.Config {
	return model.Config{
		Ping: model.PingSettings{
			IntervalMs: 1000,
			TimeoutMs:  1000,
			Payload:    56,
		},
		Gateway: model.GatewaySettings{
			Enabled: true,
		},
		Targets: []model.Target{
			{Enabled: true, TraceEnabled: true, Name: "Google DNS", Address: "8.8.8.8"},
			{Enabled: true, TraceEnabled: false, Name: "DNS", Address: "1.1.1.1"},
		},
		Trace: model.TraceTriggers{
			OnStart: true,
			Loss: model.TraceLossTrigger{
				Enabled: true,
				Percent: 10,
				LastN:   10,
			},
			HighRTT: model.TraceHighRTTTrigger{
				Enabled: true,
				RTTms:   700,
				Percent: 10,
				LastN:   10,
			},
			CooldownSec: 300,
		},
	}
}
