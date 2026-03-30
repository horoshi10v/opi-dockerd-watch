package service

import (
	"fmt"
	"strings"
)

func (s *Service) handleCleanupReportCommand() error {
	report, err := s.docker.CleanupReport()
	if err != nil {
		return s.telegram.Send(fmt.Sprintf("%s cleanup report failed: %v", s.cfg.HostAlias, err))
	}
	return s.telegram.Send(s.formatCleanupReport(report))
}

func (s *Service) formatCleanupReport(report string) string {
	lines := []string{
		fmt.Sprintf("%s docker cleanup report", s.cfg.HostAlias),
		"dry-run only, nothing was deleted",
		"",
		report,
	}
	return strings.Join(lines, "\n")
}
