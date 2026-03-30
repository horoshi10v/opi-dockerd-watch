package service

import (
	"context"
	"log"
	"time"
)

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
	s.state.Normalize()
	s.pruneState()
	s.ensureSummaryWindow(now)

	if err := s.checkContainers(now); err != nil {
		log.Printf("container checks failed: %v", err)
	}
	if err := s.sendScheduledSummary(now); err != nil {
		log.Printf("scheduled summary failed: %v", err)
	}
	if err := s.handleCommands(now); err != nil {
		log.Printf("commands failed: %v", err)
	}

	return s.store.Save(s.state)
}
