package service

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"strings"
	"time"

	"github.com/horoshi10v/opi-dockerd-watch/internal/config"
	"github.com/horoshi10v/opi-dockerd-watch/internal/docker"
	"github.com/horoshi10v/opi-dockerd-watch/internal/state"
	"github.com/horoshi10v/opi-dockerd-watch/internal/telegram"
)

type Service struct {
	cfg      config.Config
	docker   *docker.Client
	telegram *telegram.Client
	store    *state.Store
	state    state.State
}

func New(cfg config.Config) (*Service, error) {
	store := state.New(cfg.DataDir)
	st, err := store.Load()
	if err != nil {
		return nil, err
	}

	return &Service{
		cfg:      cfg,
		docker:   docker.New(),
		telegram: telegram.New(cfg.TelegramBotToken, cfg.TelegramChatID),
		store:    store,
		state:    st,
	}, nil
}

func (s *Service) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.PollInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := s.tick(); err != nil {
				log.Printf("tick failed: %v", err)
			}
		}
	}
}

func (s *Service) tick() error {
	now := time.Now()
	if err := s.checkContainers(now); err != nil {
		log.Printf("container checks failed: %v", err)
	}
	if err := s.handleCommands(now); err != nil {
		log.Printf("commands failed: %v", err)
	}
	return s.store.Save(s.state)
}

func (s *Service) checkContainers(now time.Time) error {
	for _, item := range s.cfg.Containers {
		status, err := s.docker.Inspect(item.Name)
		if err != nil {
			if item.Required {
				if sendErr := s.raiseProblem(item, "inspect_error", fmt.Sprintf("%s: inspect failed: %v", item.DisplayName, err), now); sendErr != nil {
					return sendErr
				}
			}
			continue
		}

		containerState := s.state.Containers[item.Name]
		problemKind, problemMessage := s.evaluate(item, status, containerState)
		if problemKind != "" {
			if err := s.raiseProblem(item, problemKind, problemMessage, now); err != nil {
				return err
			}
			if problemKind == "down" && item.AutoRestart && now.Sub(containerState.LastManualActionAt) >= 15*time.Minute {
				if err := s.docker.Restart(item.Name); err == nil {
					containerState.LastManualActionAt = now
				}
			}
		} else if containerState.ProblemActive {
			msg := fmt.Sprintf("%s: recovered, status=%s health=%s restarts=%d", item.DisplayName, status.Status, status.Health, status.RestartCount)
			if err := s.telegram.Send(msg); err != nil {
				return err
			}
			containerState.ProblemActive = false
			containerState.ProblemKind = ""
			containerState.LastRecoverySentAt = now
		}

		containerState.LastStatus = status.Status
		containerState.LastHealth = status.Health
		containerState.LastRestartCount = status.RestartCount
		s.state.Containers[item.Name] = containerState
	}

	return nil
}

func (s *Service) evaluate(item config.ContainerConfig, status docker.ContainerStatus, st state.ContainerState) (kind, message string) {
	if item.Required && status.Status != "running" {
		return "down", fmt.Sprintf("%s: down, status=%s exit_code=%d restart_policy=%s", item.DisplayName, status.Status, status.ExitCode, status.RestartPolicy)
	}

	if item.Required && status.Status == "running" && status.Health == "unhealthy" {
		return "unhealthy", fmt.Sprintf("%s: unhealthy, restarts=%d", item.DisplayName, status.RestartCount)
	}

	if delta := status.RestartCount - st.LastRestartCount; st.LastRestartCount > 0 && delta > item.MaxRestartDelta {
		return "restart_spike", fmt.Sprintf("%s: restart spike detected, +%d restarts (total=%d)", item.DisplayName, delta, status.RestartCount)
	}

	return "", ""
}

func (s *Service) raiseProblem(item config.ContainerConfig, kind, message string, now time.Time) error {
	st := s.state.Containers[item.Name]
	shouldSend := !st.ProblemActive || st.ProblemKind != kind || now.Sub(st.LastAlertSentAt) >= s.cfg.AlertCooldown()
	if !shouldSend {
		return nil
	}

	if err := s.telegram.Send(message); err != nil {
		return err
	}

	st.ProblemActive = true
	st.ProblemKind = kind
	st.LastAlertSentAt = now
	s.state.Containers[item.Name] = st
	return nil
}

func (s *Service) handleCommands(now time.Time) error {
	updates, err := s.telegram.GetUpdates(s.state.LastUpdateID + 1)
	if err != nil {
		return err
	}

	for _, update := range updates {
		if update.UpdateID > s.state.LastUpdateID {
			s.state.LastUpdateID = update.UpdateID
		}
		if update.Message.Text == "" {
			continue
		}
		if fmt.Sprintf("%d", update.Message.Chat.ID) != s.cfg.TelegramChatID {
			continue
		}

		text := strings.TrimSpace(update.Message.Text)
		switch {
		case text == "/docker":
			if err := s.telegram.Send(s.formatAllStatuses()); err != nil {
				return err
			}
		case strings.HasPrefix(text, "/docker "):
			name := strings.TrimSpace(strings.TrimPrefix(text, "/docker "))
			if err := s.telegram.Send(s.formatOneStatus(name)); err != nil {
				return err
			}
		case strings.HasPrefix(text, "/docker_restart "):
			name := strings.TrimSpace(strings.TrimPrefix(text, "/docker_restart "))
			if err := s.handleRestartCommand(name, now); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *Service) handleRestartCommand(name string, now time.Time) error {
	item, ok := s.findContainer(name)
	if !ok {
		return s.telegram.Send(fmt.Sprintf("container %q is not monitored", name))
	}
	if !item.AutoRestart {
		return s.telegram.Send(fmt.Sprintf("%s: manual restart denied, auto_restart=false", item.DisplayName))
	}
	if err := s.docker.Restart(item.Name); err != nil {
		return s.telegram.Send(fmt.Sprintf("%s: restart failed: %v", item.DisplayName, err))
	}
	st := s.state.Containers[item.Name]
	st.LastManualActionAt = now
	s.state.Containers[item.Name] = st
	return s.telegram.Send(fmt.Sprintf("%s: restart requested", item.DisplayName))
}

func (s *Service) formatAllStatuses() string {
	lines := []string{fmt.Sprintf("%s docker status", s.cfg.HostAlias), ""}
	for _, item := range s.cfg.Containers {
		status, err := s.docker.Inspect(item.Name)
		if err != nil {
			lines = append(lines, fmt.Sprintf("%s: inspect error", item.DisplayName))
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %s, %s, restarts=%d", item.DisplayName, status.Status, status.Health, status.RestartCount))
	}
	lines = append(lines, "", fmt.Sprintf("host cores: %d", runtime.NumCPU()))
	return strings.Join(lines, "\n")
}

func (s *Service) formatOneStatus(name string) string {
	item, ok := s.findContainer(name)
	if !ok {
		return fmt.Sprintf("container %q is not monitored", name)
	}

	status, err := s.docker.Inspect(item.Name)
	if err != nil {
		return fmt.Sprintf("%s: inspect error: %v", item.DisplayName, err)
	}

	return fmt.Sprintf(
		"%s\nstatus: %s\nhealth: %s\nrestarts: %d\nstarted: %s\nexit_code: %d\nrestart_policy: %s",
		item.DisplayName,
		status.Status,
		status.Health,
		status.RestartCount,
		status.StartedAt.Format(time.RFC3339),
		status.ExitCode,
		status.RestartPolicy,
	)
}

func (s *Service) findContainer(name string) (config.ContainerConfig, bool) {
	for _, item := range s.cfg.Containers {
		if item.Name == name || item.DisplayName == name {
			return item, true
		}
	}
	return config.ContainerConfig{}, false
}
