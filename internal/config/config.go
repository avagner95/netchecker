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
			IntervalMs: 1000,
			TimeoutMs:  1000,
			Payload:    56,
		},
		Gateway: GatewaySettings{
			Enabled: true,
		},
		Targets: []Target{
			{Enabled: true, TraceEnabled: true, Name: "Google DNS", Address: "8.8.8.8"},
			{Enabled: true, TraceEnabled: false, Name: "DNS", Address: "1.1.1.1"},
		},
		Trace: TraceTriggers{
			OnStart: true,
			Loss: TraceLossTrigger{
				Enabled: true,
				Percent: 10,
				LastN:   10,
			},
			HighRTT: TraceHighRTTTrigger{
				Enabled: true,
				RTTms:   700,
				Percent: 10,
				LastN:   10,
			},
			CooldownSec: 300,
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
