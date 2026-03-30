# opi-dockerd-watch

Tiny Go daemon for Orange Pi and similar Linux hosts. It monitors selected Docker containers, sends Telegram alerts when required containers go down or become unhealthy, tracks restart spikes, and supports a small command surface through Telegram.
## Features

- Polls selected Docker containers on a fixed interval
- Reads status via `docker inspect`
- Detects:
  - container down
  - unhealthy health check
  - restart spikes
  - recovery
- Optional auto-restart per container
- Telegram commands:
  - `/docker`
  - `/docker <name>`
  - `/docker_restart <name>`

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

## Telegram commands

- `/docker` shows all monitored containers
- `/docker <name>` shows one container
- `/docker_restart <name>` tries to restart a container if it is configured with `auto_restart=true`

## Runtime files

- Config: `/etc/opi-dockerd-watch/config.json`
- Data: `/var/lib/opi-dockerd-watch/`
- State: `/var/lib/opi-dockerd-watch/state.json`
