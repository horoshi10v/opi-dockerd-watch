package docker

import (
	"bytes"
	"fmt"
	"os/exec"
	"sort"
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

type danglingImage struct {
	Repository string
	Tag        string
	ID         string
	SizeBytes  int64
}

type stoppedContainer struct {
	Name         string
	Status       string
	WritableSize int64
}

func New() *Client {
	return &Client{}
}

func (c *Client) Inspect(name string) (ContainerStatus, error) {
	out, err := c.run(
		"inspect",
		"-f",
		"{{.Name}}|{{.State.Status}}|{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}|{{.RestartCount}}|{{.State.StartedAt}}|{{.State.ExitCode}}|{{.HostConfig.RestartPolicy.Name}}",
		name,
	)
	if err != nil {
		return ContainerStatus{}, fmt.Errorf("docker inspect %s: %w", name, err)
	}

	parts := strings.Split(strings.TrimSpace(out), "|")
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
	if _, err := c.run("restart", name); err != nil {
		return fmt.Errorf("docker restart %s: %w", name, err)
	}
	return nil
}

func (c *Client) CleanupReport() (string, error) {
	danglingImages, err := c.listDanglingImages()
	if err != nil {
		return "", err
	}
	stoppedContainers, err := c.listStoppedContainers()
	if err != nil {
		return "", err
	}
	diskUsage, err := c.run("system", "df")
	if err != nil {
		return "", err
	}

	lines := []string{
		"disk usage:",
		indentBlock(strings.TrimSpace(diskUsage)),
		"",
		fmt.Sprintf("dangling images: %d", len(danglingImages)),
	}
	if len(danglingImages) == 0 {
		lines = append(lines, "  none")
	} else {
		for _, image := range danglingImages {
			lines = append(lines, fmt.Sprintf("  - %s:%s (%s, %s)", image.Repository, image.Tag, image.ID, formatBytes(image.SizeBytes)))
		}
	}

	lines = append(lines, "", fmt.Sprintf("stopped containers: %d", len(stoppedContainers)))
	if len(stoppedContainers) == 0 {
		lines = append(lines, "  none")
	} else {
		for _, container := range stoppedContainers {
			lines = append(lines, fmt.Sprintf("  - %s (%s, writable=%s)", container.Name, container.Status, formatBytes(container.WritableSize)))
		}
	}

	lines = append(lines, "", "large cleanup candidates:")
	candidates := c.largeCandidates(danglingImages, stoppedContainers)
	if len(candidates) == 0 {
		lines = append(lines, "  none")
	} else {
		for _, candidate := range candidates {
			lines = append(lines, "  - "+candidate)
		}
	}

	return strings.Join(lines, "\n"), nil
}

func (c *Client) listDanglingImages() ([]danglingImage, error) {
	out, err := c.run("images", "--filter", "dangling=true", "--format", "{{.Repository}}|{{.Tag}}|{{.ID}}")
	if err != nil {
		return nil, err
	}

	lines := splitNonEmptyLines(out)
	images := make([]danglingImage, 0, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) != 3 {
			continue
		}

		sizeBytes, err := c.imageSizeBytes(parts[2])
		if err != nil {
			return nil, err
		}

		images = append(images, danglingImage{
			Repository: parts[0],
			Tag:        parts[1],
			ID:         parts[2],
			SizeBytes:  sizeBytes,
		})
	}

	sort.Slice(images, func(i, j int) bool {
		return images[i].SizeBytes > images[j].SizeBytes
	})

	if len(images) > 5 {
		images = images[:5]
	}
	return images, nil
}

func (c *Client) listStoppedContainers() ([]stoppedContainer, error) {
	out, err := c.run(
		"ps",
		"-a",
		"--filter", "status=created",
		"--filter", "status=exited",
		"--filter", "status=dead",
		"--format",
		"{{.Names}}|{{.Status}}",
	)
	if err != nil {
		return nil, err
	}

	lines := splitNonEmptyLines(out)
	containers := make([]stoppedContainer, 0, len(lines))
	for _, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) != 2 {
			continue
		}

		sizeBytes, err := c.containerWritableSize(parts[0])
		if err != nil {
			return nil, err
		}

		containers = append(containers, stoppedContainer{
			Name:         parts[0],
			Status:       parts[1],
			WritableSize: sizeBytes,
		})
	}

	sort.Slice(containers, func(i, j int) bool {
		return containers[i].WritableSize > containers[j].WritableSize
	})

	if len(containers) > 5 {
		containers = containers[:5]
	}
	return containers, nil
}

func (c *Client) largeCandidates(images []danglingImage, containers []stoppedContainer) []string {
	candidates := make([]string, 0, len(images)+len(containers))
	for _, image := range images {
		candidates = append(candidates, fmt.Sprintf("dangling image %s:%s (%s)", image.Repository, image.Tag, formatBytes(image.SizeBytes)))
	}
	for _, container := range containers {
		candidates = append(candidates, fmt.Sprintf("stopped container %s (%s)", container.Name, formatBytes(container.WritableSize)))
	}
	return candidates
}

func (c *Client) imageSizeBytes(id string) (int64, error) {
	out, err := c.run("image", "inspect", "-f", "{{.Size}}", id)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(out), 10, 64)
}

func (c *Client) containerWritableSize(name string) (int64, error) {
	out, err := c.run("inspect", "--size", "-f", "{{.SizeRw}}", name)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(strings.TrimSpace(out), 10, 64)
}

func (c *Client) run(args ...string) (string, error) {
	cmd := exec.Command("docker", args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func splitNonEmptyLines(s string) []string {
	raw := strings.Split(strings.TrimSpace(s), "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func indentBlock(s string) string {
	lines := splitNonEmptyLines(s)
	if len(lines) == 0 {
		return "  none"
	}

	for i := range lines {
		lines[i] = "  " + lines[i]
	}
	return strings.Join(lines, "\n")
}

func formatBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}

	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}
