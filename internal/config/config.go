package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func DefaultConfig() Config {
	return Config{
		Ping: PingSettings{
			IntervalMs: 300,
			TimeoutMs:  1000,
			Payload:    64,
		},
		Gateway: GatewaySettings{
			Enabled: true,
		},
		Targets: []Target{
			//External
			{Enabled: true, TraceEnabled: true, Name: "Yandex", Address: "77.88.8.8"},
			{Enabled: true, TraceEnabled: true, Name: "VK", Address: "87.240.132.72"},
			{Enabled: true, TraceEnabled: true, Name: "2gis", Address: "91.236.51.50"},
			{Enabled: true, TraceEnabled: true, Name: "Ozon", Address: "162.159.140.11"},
			{Enabled: true, TraceEnabled: true, Name: "Rutube", Address: "178.248.233.148"},
			{Enabled: true, TraceEnabled: true, Name: "Wildberries", Address: "185.138.253.1"},
			{Enabled: true, TraceEnabled: true, Name: "GosUslugi", Address: "213.59.253.7"},
			//External Bank
			{Enabled: true, TraceEnabled: true, Name: "MYPC", Address: "217.12.96.114"},
			{Enabled: true, TraceEnabled: true, Name: "MYCC", Address: "217.12.96.106"},
			{Enabled: true, TraceEnabled: true, Name: "Telework", Address: "10.229.208.78"},
			//Internal
			{Enabled: true, TraceEnabled: true, Name: "VDI", Address: "10.211.3.182"},
			{Enabled: true, TraceEnabled: true, Name: "DNS VPN1", Address: "10.224.0.5"},
			{Enabled: true, TraceEnabled: true, Name: "DNS VPN2", Address: "10.226.0.5"},
		},
		Trace: TraceTriggers{
			OnStart: true,
			Loss: TraceLossTrigger{
				Enabled: true,
				Percent: 10,
				LastN:   50,
			},
			HighRTT: TraceHighRTTTrigger{
				Enabled: true,
				RTTms:   300,
				Percent: 10,
				LastN:   50,
			},
			CooldownSec: 600,
		},
	}
}

func Path(AppName string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("os.UserConfigDir: %w", err)
	}
	return filepath.Join(dir, AppName, FileConfig), nil
}

func EnsureDir(path string) error {
	dir := filepath.Dir(path)
	return os.MkdirAll(dir, 0o755)
}

func LoadOrCreate[T any](defaultValue T, AppName string) (T, string, error) {
	var zero T

	path, err := Path(AppName)
	if err != nil {
		return zero, "", err
	}
	if err := EnsureDir(path); err != nil {
		return zero, "", fmt.Errorf("ensure dir: %w", err)
	}

	// If doesn't exist -> create with defaults
	_, statErr := os.Stat(path)
	if statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			if err := Save(path, defaultValue); err != nil {
				return zero, path, err
			}
			return defaultValue, path, nil
		}
		return zero, path, fmt.Errorf("stat config: %w", statErr)
	}

	// Load existing
	b, err := os.ReadFile(path)
	if err != nil {
		return zero, path, fmt.Errorf("read config: %w", err)
	}

	var cfg T
	if err := json.Unmarshal(b, &cfg); err != nil {
		// Если файл битый — не молчим: вернём ошибку
		return zero, path, fmt.Errorf("unmarshal config: %w", err)
	}

	return cfg, path, nil
}

func Save[T any](path string, cfg T) error {
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return fmt.Errorf("write tmp config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename tmp config: %w", err)
	}
	return nil
}

func DataDBPath(AppName string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("os.UserConfigDir: %w", err)
	}
	return filepath.Join(dir, AppName, "data", "metrics.sqlite"), nil
}
