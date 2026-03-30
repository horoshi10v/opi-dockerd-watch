package service

import "github.com/horoshi10v/opi-dockerd-watch/internal/config"

func (s *Service) findContainer(name string) (config.ContainerConfig, bool) {
	for _, item := range s.cfg.Containers {
		if item.Name == name || item.DisplayName == name {
			return item, true
		}
	}
	return config.ContainerConfig{}, false
}

func (s *Service) pruneState() {
	allowed := make(map[string]struct{}, len(s.cfg.Containers))
	for _, item := range s.cfg.Containers {
		allowed[item.Name] = struct{}{}
	}

	for name := range s.state.Containers {
		if _, ok := allowed[name]; !ok {
			delete(s.state.Containers, name)
		}
	}
	for name := range s.state.DailySummary.Containers {
		if _, ok := allowed[name]; !ok {
			delete(s.state.DailySummary.Containers, name)
		}
	}
}
