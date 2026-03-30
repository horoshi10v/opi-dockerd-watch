package service

import (
	"fmt"
	"time"

	"github.com/horoshi10v/opi-dockerd-watch/internal/config"
	"github.com/horoshi10v/opi-dockerd-watch/internal/docker"
	"github.com/horoshi10v/opi-dockerd-watch/internal/state"
)

func (s *Service) checkContainers(now time.Time) error {
	for _, item := range s.cfg.Containers {
		containerState := s.state.Containers[item.Name]
		s.ensureSummaryContainer(item.Name)

		status, err := s.docker.Inspect(item.Name)
		if err != nil {
			containerState.ConsecutiveInspectFailures++
			if item.Required && containerState.ConsecutiveInspectFailures >= s.cfg.InspectFailureThreshold {
				s.markSummaryInspectIssue(item.Name)
				if sendErr := s.raiseProblem(
					&containerState,
					item,
					"inspect_error",
					fmt.Sprintf(
						"%s: inspect failed %d/%d times: %v",
						item.DisplayName,
						containerState.ConsecutiveInspectFailures,
						s.cfg.InspectFailureThreshold,
						err,
					),
					now,
				); sendErr != nil {
					s.state.Containers[item.Name] = containerState
					return sendErr
				}
			}
			s.state.Containers[item.Name] = containerState
			continue
		}

		summary := s.state.DailySummary.Containers[item.Name]
		containerState.ConsecutiveInspectFailures = 0
		containerState.LastSeenAt = now

		problemKind, problemMessage := s.evaluate(item, status, &containerState, &summary)
		s.state.DailySummary.Containers[item.Name] = summary

		if problemKind != "" {
			if err := s.raiseProblem(&containerState, item, problemKind, problemMessage, now); err != nil {
				s.state.Containers[item.Name] = containerState
				return err
			}
			if problemKind == "down" {
				if err := s.tryAutoRestart(item, &containerState, now); err != nil {
					s.state.Containers[item.Name] = containerState
					return err
				}
			}
		} else if containerState.ProblemActive {
			msg := fmt.Sprintf(
				"%s: recovered, status=%s health=%s restarts=%d",
				item.DisplayName,
				status.Status,
				status.Health,
				status.RestartCount,
			)
			if err := s.telegram.Send(msg); err != nil {
				s.state.Containers[item.Name] = containerState
				return err
			}
			containerState.ProblemActive = false
			containerState.ProblemKind = ""
			containerState.LastRecoverySentAt = now
		}

		if status.Status == "running" && status.Health != "unhealthy" {
			containerState.AutoRestartAttemptCount = 0
		}

		containerState.LastStatus = status.Status
		containerState.LastHealth = status.Health
		containerState.LastRestartCount = status.RestartCount
		s.state.Containers[item.Name] = containerState
	}

	return nil
}

func (s *Service) evaluate(
	item config.ContainerConfig,
	status docker.ContainerStatus,
	st *state.ContainerState,
	summary *state.DailyContainerSummary,
) (kind, message string) {
	if item.Required && status.Status != "running" {
		st.ConsecutiveDownDetections++
	} else {
		st.ConsecutiveDownDetections = 0
	}

	if item.Required && status.Status == "running" && status.Health == "unhealthy" {
		st.ConsecutiveUnhealthyDetections++
	} else {
		st.ConsecutiveUnhealthyDetections = 0
	}

	if item.Required && st.ConsecutiveDownDetections >= s.cfg.StatusFailureThreshold {
		summary.HadDown = true
		return "down", fmt.Sprintf(
			"%s: down confirmed %d/%d polls, status=%s exit_code=%d restart_policy=%s",
			item.DisplayName,
			st.ConsecutiveDownDetections,
			s.cfg.StatusFailureThreshold,
			status.Status,
			status.ExitCode,
			status.RestartPolicy,
		)
	}

	if item.Required && st.ConsecutiveUnhealthyDetections >= s.cfg.StatusFailureThreshold {
		summary.HadUnhealthy = true
		return "unhealthy", fmt.Sprintf(
			"%s: unhealthy confirmed %d/%d polls, restarts=%d",
			item.DisplayName,
			st.ConsecutiveUnhealthyDetections,
			s.cfg.StatusFailureThreshold,
			status.RestartCount,
		)
	}

	if delta := status.RestartCount - st.LastRestartCount; st.LastRestartCount > 0 && delta > item.MaxRestartDelta {
		summary.RestartSpikeCount++
		return "restart_spike", fmt.Sprintf(
			"%s: restart spike detected, +%d restarts (total=%d)",
			item.DisplayName,
			delta,
			status.RestartCount,
		)
	}

	return "", ""
}

func (s *Service) raiseProblem(st *state.ContainerState, item config.ContainerConfig, kind, message string, now time.Time) error {
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
	return nil
}

func (s *Service) tryAutoRestart(item config.ContainerConfig, st *state.ContainerState, now time.Time) error {
	if !item.AutoRestart {
		return nil
	}
	if st.AutoRestartAttemptCount >= s.cfg.AutoRestartMaxAttempts {
		return nil
	}

	baseCooldown := s.cfg.AutoRestartCooldown()
	if !st.LastManualActionAt.IsZero() && now.Sub(st.LastManualActionAt) < baseCooldown {
		return nil
	}
	if !st.LastAutoRestartAt.IsZero() && now.Sub(st.LastAutoRestartAt) < s.autoRestartBackoff(*st) {
		return nil
	}

	if err := s.docker.Restart(item.Name); err != nil {
		return s.telegram.Send(fmt.Sprintf("%s: auto-restart failed: %v", item.DisplayName, err))
	}

	st.LastAutoRestartAt = now
	st.AutoRestartAttemptCount++
	s.recordAutoRestartAttempt(item.Name)

	return s.telegram.Send(fmt.Sprintf(
		"%s: auto-restart attempt %d/%d requested",
		item.DisplayName,
		st.AutoRestartAttemptCount,
		s.cfg.AutoRestartMaxAttempts,
	))
}

func (s *Service) autoRestartBackoff(st state.ContainerState) time.Duration {
	if st.AutoRestartAttemptCount <= 0 {
		return 0
	}

	backoff := s.cfg.AutoRestartCooldown()
	for attempt := 1; attempt < st.AutoRestartAttemptCount; attempt++ {
		backoff *= 2
		if backoff >= s.cfg.AutoRestartMaxBackoff() {
			return s.cfg.AutoRestartMaxBackoff()
		}
	}

	if backoff > s.cfg.AutoRestartMaxBackoff() {
		return s.cfg.AutoRestartMaxBackoff()
	}
	return backoff
}
