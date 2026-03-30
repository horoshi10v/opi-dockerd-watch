package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"github.com/horoshi10v/opi-dockerd-watch/internal/config"
	"github.com/horoshi10v/opi-dockerd-watch/internal/service"
)

func main() {
	configPath := flag.String("config", "/etc/opi-dockerd-watch/config.json", "Path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	svc, err := service.New(cfg)
	if err != nil {
		log.Fatalf("init service: %v", err)
	}

	if err := svc.Run(ctx); err != nil {
		log.Fatalf("run service: %v", err)
	}
}
