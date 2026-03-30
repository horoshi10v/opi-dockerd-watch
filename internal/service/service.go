package service

import (
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

	st.Normalize()

	return &Service{
		cfg:      cfg,
		docker:   docker.New(),
		telegram: telegram.New(cfg.TelegramBotToken, cfg.TelegramChatID),
		store:    store,
		state:    st,
	}, nil
}
