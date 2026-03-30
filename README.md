# opi-dockerd-watch

Tiny Go daemon for Orange Pi and similar Linux hosts. It monitors selected Docker containers, sends Telegram alerts when required containers go down or become unhealthy, tracks restart spikes, sends daily summaries, and exposes a small command surface through Telegram.

## Features

- Polls selected Docker containers on a fixed interval
- Reads status via `docker inspect`
- Detects:
  - repeated inspect failures
  - confirmed container down
  - confirmed unhealthy health checks
  - restart spikes
  - recovery
- Optional auto-restart with cooldown, retry threshold, and backoff
- Daily Telegram summary with manual on-demand summary command
- Safe dry-run cleanup report for dangling images and stopped containers
- Telegram commands:
  - `/docker`
  - `/docker <name>`
  - `/docker_restart <name>`
  - `/docker_summary`
  - `/docker_cleanup_report`

## Project layout

```text
opi-dockerd-watch/
  cmd/opi-dockerd-watch/main.go
  config/config.example.json
  deploy/opi-dockerd-watch.service
  internal/
    config/
    docker/
    service/
    state/
    telegram/
```

## Local build

```bash
go build -o bin/opi-dockerd-watch ./cmd/opi-dockerd-watch
```

## Deploy on Orange Pi

```bash
sudo apt update
sudo apt install -y golang-go git docker.io

cd /opt
sudo git clone https://github.com/horoshi10v/opi-dockerd-watch.git
cd /opt/opi-dockerd-watch

go build -o opi-dockerd-watch ./cmd/opi-dockerd-watch
sudo install -Dm755 ./opi-dockerd-watch /usr/local/bin/opi-dockerd-watch

sudo mkdir -p /etc/opi-dockerd-watch /var/lib/opi-dockerd-watch
sudo cp ./config/config.example.json /etc/opi-dockerd-watch/config.json
sudo nano /etc/opi-dockerd-watch/config.json

sudo cp ./deploy/opi-dockerd-watch.service /etc/systemd/system/opi-dockerd-watch.service
sudo systemctl daemon-reload
sudo systemctl enable --now opi-dockerd-watch
```

## Config notes

- `inspect_failure_threshold` controls how many failed `docker inspect` polls are needed before alerting
- `status_failure_threshold` controls how many consecutive polls must confirm `down` or `unhealthy`
- `auto_restart_cooldown_minutes`, `auto_restart_max_attempts`, and `auto_restart_max_backoff_minutes` limit restart retries
- `daily_summary.enabled/hour/minute` controls scheduled summary delivery in host local time
- Optional containers can stay out of the required alert flow

## Telegram commands

- `/docker` shows all monitored containers
- `/docker <name>` shows one container
- `/docker_restart <name>` tries to restart a container if it is configured with `auto_restart=true`
- `/docker_summary` shows the current summary window without resetting it
- `/docker_cleanup_report` shows a dry-run cleanup report without deleting anything

## Runtime files

- Config: `/etc/opi-dockerd-watch/config.json`
- Data: `/var/lib/opi-dockerd-watch/`
- State: `/var/lib/opi-dockerd-watch/state.json`
