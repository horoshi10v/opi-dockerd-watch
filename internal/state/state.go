package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type ContainerState struct {
	LastStatus                     string    `json:"last_status"`
	LastHealth                     string    `json:"last_health"`
	LastRestartCount               int       `json:"last_restart_count"`
	ProblemActive                  bool      `json:"problem_active"`
	ProblemKind                    string    `json:"problem_kind"`
	LastAlertSentAt                time.Time `json:"last_alert_sent_at"`
	LastRecoverySentAt             time.Time `json:"last_recovery_sent_at"`
	LastManualActionAt             time.Time `json:"last_manual_action_at"`
	LastAutoRestartAt              time.Time `json:"last_auto_restart_at"`
	LastSeenAt                     time.Time `json:"last_seen_at"`
	ConsecutiveInspectFailures     int       `json:"consecutive_inspect_failures"`
	ConsecutiveDownDetections      int       `json:"consecutive_down_detections"`
	ConsecutiveUnhealthyDetections int       `json:"consecutive_unhealthy_detections"`
	AutoRestartAttemptCount        int       `json:"auto_restart_attempt_count"`
}

type DailyContainerSummary struct {
	HadDown             bool `json:"had_down"`
	HadUnhealthy        bool `json:"had_unhealthy"`
	HadInspectIssue     bool `json:"had_inspect_issue"`
	RestartSpikeCount   int  `json:"restart_spike_count"`
	AutoRestartAttempts int  `json:"auto_restart_attempts"`
}

type DailySummaryState struct {
	PeriodStartedAt     time.Time                        `json:"period_started_at"`
	LastScheduledSentAt time.Time                        `json:"last_scheduled_sent_at"`
	Containers          map[string]DailyContainerSummary `json:"containers"`
}

type State struct {
	LastUpdateID int64                     `json:"last_update_id"`
	Containers   map[string]ContainerState `json:"containers"`
	DailySummary DailySummaryState         `json:"daily_summary"`
}

type Store struct {
	dataDir   string
	statePath string
}

func New(dataDir string) *Store {
	cleanDir := filepath.Clean(dataDir)
	return &Store{
		dataDir:   cleanDir,
		statePath: filepath.Join(cleanDir, "state.json"),
	}
}

func (s *Store) Load() (State, error) {
	var st State
	data, err := os.ReadFile(s.statePath)
	if err != nil {
		if os.IsNotExist(err) {
			st.Normalize()
			return st, nil
		}
		return st, err
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return st, err
	}
	st.Normalize()
	return st, nil
}

func (s *Store) Save(st State) error {
	st.Normalize()
	if err := os.MkdirAll(s.dataDir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := s.statePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, s.statePath)
}

func (s *State) Normalize() {
	if s.Containers == nil {
		s.Containers = make(map[string]ContainerState)
	}
	if s.DailySummary.Containers == nil {
		s.DailySummary.Containers = make(map[string]DailyContainerSummary)
	}
}
