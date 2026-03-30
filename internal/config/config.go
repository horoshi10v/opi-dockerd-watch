package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	PollIntervalSeconds          int                `json:"poll_interval_seconds"`
	DataDir                      string             `json:"data_dir"`
	TelegramBotToken             string             `json:"telegram_bot_token"`
	TelegramChatID               string             `json:"telegram_chat_id"`
	HostAlias                    string             `json:"host_alias"`
	AlertCooldownMinute          int                `json:"alert_cooldown_minutes"`
	InspectFailureThreshold      int                `json:"inspect_failure_threshold"`
	StatusFailureThreshold       int                `json:"status_failure_threshold"`
	AutoRestartCooldownMinutes   int                `json:"auto_restart_cooldown_minutes"`
	AutoRestartMaxAttempts       int                `json:"auto_restart_max_attempts"`
	AutoRestartMaxBackoffMinutes int                `json:"auto_restart_max_backoff_minutes"`
	DailySummary                 DailySummaryConfig `json:"daily_summary"`
	Containers                   []ContainerConfig  `json:"containers"`
}

type DailySummaryConfig struct {
	Enabled bool `json:"enabled"`
	Hour    int  `json:"hour"`
	Minute  int  `json:"minute"`
}

type ContainerConfig struct {
	Name            string `json:"name"`
	DisplayName     string `json:"display_name"`
	Required        bool   `json:"required"`
	AutoRestart     bool   `json:"auto_restart"`
	MaxRestartDelta int    `json:"max_restart_delta"`
}

func Load(path string) (Config, error) {
	var cfg Config

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read %s: %w", path, err)
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("decode config: %w", err)
	}

	if cfg.PollIntervalSeconds <= 0 {
		cfg.PollIntervalSeconds = 30
	}
	if cfg.AlertCooldownMinute <= 0 {
		cfg.AlertCooldownMinute = 20
	}
	if cfg.InspectFailureThreshold <= 0 {
		cfg.InspectFailureThreshold = 3
	}
	if cfg.StatusFailureThreshold <= 0 {
		cfg.StatusFailureThreshold = 2
	}
	if cfg.AutoRestartCooldownMinutes <= 0 {
		cfg.AutoRestartCooldownMinutes = 15
	}
	if cfg.AutoRestartMaxAttempts <= 0 {
		cfg.AutoRestartMaxAttempts = 3
	}
	if cfg.AutoRestartMaxBackoffMinutes <= 0 {
		cfg.AutoRestartMaxBackoffMinutes = 120
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "/var/lib/opi-dockerd-watch"
	}
	if cfg.HostAlias == "" {
		host, _ := os.Hostname()
		cfg.HostAlias = host
	}
	if cfg.DailySummary.Hour < 0 || cfg.DailySummary.Hour > 23 {
		cfg.DailySummary.Hour = 9
	}
	if cfg.DailySummary.Minute < 0 || cfg.DailySummary.Minute > 59 {
		cfg.DailySummary.Minute = 0
	}

	for i := range cfg.Containers {
		if cfg.Containers[i].DisplayName == "" {
			cfg.Containers[i].DisplayName = cfg.Containers[i].Name
		}
		if cfg.Containers[i].MaxRestartDelta <= 0 {
			cfg.Containers[i].MaxRestartDelta = 1
		}
	}

	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return cfg, fmt.Errorf("create data dir: %w", err)
	}

	cfg.DataDir = filepath.Clean(cfg.DataDir)
	return cfg, nil
}

func (c Config) PollInterval() time.Duration {
	return time.Duration(c.PollIntervalSeconds) * time.Second
}

func (c Config) AlertCooldown() time.Duration {
	return time.Duration(c.AlertCooldownMinute) * time.Minute
}

func (c Config) AutoRestartCooldown() time.Duration {
	return time.Duration(c.AutoRestartCooldownMinutes) * time.Minute
}

func (c Config) AutoRestartMaxBackoff() time.Duration {
	return time.Duration(c.AutoRestartMaxBackoffMinutes) * time.Minute
}
