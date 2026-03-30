package docker

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Client struct{}

type ContainerStatus struct {
	Name          string
	Status        string
	Health        string
	RestartCount  int
	StartedAt     time.Time
	ExitCode      int
	RestartPolicy string
}

func New() *Client {
	return &Client{}
}

func (c *Client) Inspect(name string) (ContainerStatus, error) {
	cmd := exec.Command(
		"docker",
		"inspect",
		"-f",
		"{{.Name}}|{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}|{{.RestartCount}}|{{.State.StartedAt}}|{{.State.ExitCode}}|{{.HostConfig.RestartPolicy.Name}}",
		name,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return ContainerStatus{}, fmt.Errorf("docker inspect %s: %v: %s", name, err, strings.TrimSpace(stderr.String()))
	}

	parts := strings.Split(strings.TrimSpace(stdout.String()), "|")
	if len(parts) != 7 {
		return ContainerStatus{}, fmt.Errorf("unexpected inspect format for %s", name)
	}

	restartCount, err := strconv.Atoi(parts[3])
	if err != nil {
		return ContainerStatus{}, err
	}
	exitCode, err := strconv.Atoi(parts[5])
	if err != nil {
		return ContainerStatus{}, err
	}

	var startedAt time.Time
	if parts[4] != "" && parts[4] != "0001-01-01T00:00:00Z" {
		startedAt, _ = time.Parse(time.RFC3339Nano, parts[4])
	}

	return ContainerStatus{
		Name:          strings.TrimPrefix(parts[0], "/"),
		Status:        parts[1],
		Health:        parts[2],
		RestartCount:  restartCount,
		StartedAt:     startedAt,
		ExitCode:      exitCode,
		RestartPolicy: parts[6],
	}, nil
}

func (c *Client) Restart(name string) error {
	cmd := exec.Command("docker", "restart", name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker restart %s: %v: %s", name, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
