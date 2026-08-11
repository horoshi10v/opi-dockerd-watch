package service

import (
	"fmt"
	"runtime"
	"strings"
	"time"
)

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
		case text == "/docker_summary":
			if err := s.telegram.Send(s.formatDailySummary(now, false)); err != nil {
				return err
			}
		case text == "/docker_cleanup_report":
			if err := s.handleCleanupReportCommand(); err != nil {
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
		default:
			if config, ok := apacheVHostForSite(text); ok {
				if err := s.telegram.SendPreformatted(config); err != nil {
					return err
				}
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
	st.AutoRestartAttemptCount = 0
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

		containerState := s.state.Containers[item.Name]
		lines = append(
			lines,
			fmt.Sprintf(
				"%s: %s, %s, restarts=%d, inspect_failures=%d, down_checks=%d, unhealthy_checks=%d",
				item.DisplayName,
				status.Status,
				status.Health,
				status.RestartCount,
				containerState.ConsecutiveInspectFailures,
				containerState.ConsecutiveDownDetections,
				containerState.ConsecutiveUnhealthyDetections,
			),
		)
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

	st := s.state.Containers[item.Name]
	return fmt.Sprintf(
		"%s\nstatus: %s\nhealth: %s\nrestarts: %d\nstarted: %s\nexit_code: %d\nrestart_policy: %s\ninspect_failures: %d\ndown_checks: %d\nunhealthy_checks: %d\nauto_restart_attempts: %d",
		item.DisplayName,
		status.Status,
		status.Health,
		status.RestartCount,
		status.StartedAt.Format(time.RFC3339),
		status.ExitCode,
		status.RestartPolicy,
		st.ConsecutiveInspectFailures,
		st.ConsecutiveDownDetections,
		st.ConsecutiveUnhealthyDetections,
		st.AutoRestartAttemptCount,
	)
}
