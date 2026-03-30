package service

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/horoshi10v/opi-dockerd-watch/internal/state"
)

func (s *Service) ensureSummaryWindow(now time.Time) {
	if s.state.DailySummary.PeriodStartedAt.IsZero() {
		s.state.DailySummary.PeriodStartedAt = now
	}
	if s.state.DailySummary.Containers == nil {
		s.state.DailySummary.Containers = make(map[string]state.DailyContainerSummary)
	}
	for _, item := range s.cfg.Containers {
		s.ensureSummaryContainer(item.Name)
	}
}

func (s *Service) ensureSummaryContainer(name string) {
	if s.state.DailySummary.Containers == nil {
		s.state.DailySummary.Containers = make(map[string]state.DailyContainerSummary)
	}
	if _, ok := s.state.DailySummary.Containers[name]; !ok {
		s.state.DailySummary.Containers[name] = state.DailyContainerSummary{}
	}
}

func (s *Service) markSummaryInspectIssue(name string) {
	s.ensureSummaryContainer(name)
	summary := s.state.DailySummary.Containers[name]
	summary.HadInspectIssue = true
	s.state.DailySummary.Containers[name] = summary
}

func (s *Service) recordAutoRestartAttempt(name string) {
	s.ensureSummaryContainer(name)
	summary := s.state.DailySummary.Containers[name]
	summary.AutoRestartAttempts++
	s.state.DailySummary.Containers[name] = summary
}

func (s *Service) sendScheduledSummary(now time.Time) error {
	if !s.cfg.DailySummary.Enabled {
		return nil
	}
	if s.state.DailySummary.PeriodStartedAt.IsZero() {
		return nil
	}

	scheduledAt := s.scheduledSummaryTime(now)
	if now.Before(scheduledAt) {
		return nil
	}
	if !s.state.DailySummary.LastScheduledSentAt.IsZero() && !s.state.DailySummary.LastScheduledSentAt.Before(scheduledAt) {
		return nil
	}

	if err := s.telegram.Send(s.formatDailySummary(now, true)); err != nil {
		return err
	}

	s.state.DailySummary.LastScheduledSentAt = now
	s.resetSummaryWindow(now)
	return nil
}

func (s *Service) scheduledSummaryTime(now time.Time) time.Time {
	return time.Date(
		now.Year(),
		now.Month(),
		now.Day(),
		s.cfg.DailySummary.Hour,
		s.cfg.DailySummary.Minute,
		0,
		0,
		now.Location(),
	)
}

func (s *Service) resetSummaryWindow(now time.Time) {
	s.state.DailySummary.PeriodStartedAt = now
	s.state.DailySummary.Containers = make(map[string]state.DailyContainerSummary, len(s.cfg.Containers))
	for _, item := range s.cfg.Containers {
		s.state.DailySummary.Containers[item.Name] = state.DailyContainerSummary{}
	}
}

func (s *Service) formatDailySummary(now time.Time, scheduled bool) string {
	start := s.state.DailySummary.PeriodStartedAt
	if start.IsZero() {
		start = now
	}

	stable := make([]string, 0)
	down := make([]string, 0)
	unhealthy := make([]string, 0)
	inspectIssue := make([]string, 0)
	restartLines := make([]string, 0)
	autoRestartLines := make([]string, 0)

	hadProblems := false

	for _, item := range s.cfg.Containers {
		summary := s.state.DailySummary.Containers[item.Name]

		if summary.HadDown {
			down = append(down, item.DisplayName)
			hadProblems = true
		}
		if summary.HadUnhealthy {
			unhealthy = append(unhealthy, item.DisplayName)
			hadProblems = true
		}
		if summary.HadInspectIssue {
			inspectIssue = append(inspectIssue, item.DisplayName)
			hadProblems = true
		}
		if summary.RestartSpikeCount > 0 {
			restartLines = append(restartLines, fmt.Sprintf("%s: %d", item.DisplayName, summary.RestartSpikeCount))
			hadProblems = true
		}
		if summary.AutoRestartAttempts > 0 {
			autoRestartLines = append(autoRestartLines, fmt.Sprintf("%s: %d", item.DisplayName, summary.AutoRestartAttempts))
			hadProblems = true
		}
		if !summary.HadDown && !summary.HadUnhealthy && !summary.HadInspectIssue && summary.RestartSpikeCount == 0 && summary.AutoRestartAttempts == 0 {
			stable = append(stable, item.DisplayName)
		}
	}

	sort.Strings(stable)
	sort.Strings(down)
	sort.Strings(unhealthy)
	sort.Strings(inspectIssue)
	sort.Strings(restartLines)
	sort.Strings(autoRestartLines)

	label := "docker summary"
	if scheduled {
		label = "daily docker summary"
	}

	if !hadProblems {
		return fmt.Sprintf(
			"%s %s\nperiod: %s - %s\nall monitored containers were stable",
			s.cfg.HostAlias,
			label,
			start.Format("2006-01-02 15:04"),
			now.Format("2006-01-02 15:04"),
		)
	}

	lines := []string{
		fmt.Sprintf("%s %s", s.cfg.HostAlias, label),
		fmt.Sprintf("period: %s - %s", start.Format("2006-01-02 15:04"), now.Format("2006-01-02 15:04")),
	}

	lines = append(lines, s.summarySection("stable", stable)...)
	lines = append(lines, s.summarySection("down", down)...)
	lines = append(lines, s.summarySection("unhealthy", unhealthy)...)
	lines = append(lines, s.summarySection("inspect issues", inspectIssue)...)
	lines = append(lines, s.summarySection("restart spikes", restartLines)...)
	lines = append(lines, s.summarySection("auto-restart attempts", autoRestartLines)...)

	return strings.Join(lines, "\n")
}

func (s *Service) summarySection(title string, values []string) []string {
	if len(values) == 0 {
		return nil
	}

	return []string{
		"",
		fmt.Sprintf("%s:", title),
		"- " + strings.Join(values, "\n- "),
	}
}
