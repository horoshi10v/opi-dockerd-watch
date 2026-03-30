package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type ContainerState struct {
	LastStatus         string    `json:"last_status"`
	LastHealth         string    `json:"last_health"`
	LastRestartCount   int       `json:"last_restart_count"`
	ProblemActive      bool      `json:"problem_active"`
	ProblemKind        string    `json:"problem_kind"`
	LastAlertSentAt    time.Time `json:"last_alert_sent_at"`
	LastRecoverySentAt time.Time `json:"last_recovery_sent_at"`
	LastManualActionAt time.Time `json:"last_manual_action_at"`
}

type State struct {
	LastUpdateID int64                     `json:"last_update_id"`
	Containers   map[string]ContainerState `json:"containers"`
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
			st.Containers = make(map[string]ContainerState)
			return st, nil
		}
		return st, err
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return st, err
	}
	if st.Containers == nil {
		st.Containers = make(map[string]ContainerState)
	}
	return st, nil
}

func (s *Store) Save(st State) error {
	if st.Containers == nil {
		st.Containers = make(map[string]ContainerState)
	}
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
