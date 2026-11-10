package docker

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type ContainerStats struct {
	CPU string `json:"cpu"`
	Mem string `json:"mem"`
}

type ContainerInfo struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Image     string         `json:"image"`
	Status    string         `json:"status"`
	IsRunning bool           `json:"is_running"`
	Ports     string         `json:"ports"`
	Stats     ContainerStats `json:"stats"`
}

type DiskDFInfo struct {
	ImagesReclaimable     string `json:"images_reclaimable"`
	ContainersReclaimable string `json:"containers_reclaimable"`
	VolumesReclaimable    string `json:"volumes_reclaimable"`
	BuildCacheReclaimable string `json:"build_cache_reclaimable"`
	TotalReclaimable      string `json:"total_reclaimable"`
}

type DockerTelemetry struct {
	Available    bool            `json:"available"`
	Containers   []ContainerInfo `json:"containers"`
	RunningCount int             `json:"running_count"`
	StoppedCount int             `json:"stopped_count"`
	Disk         DiskDFInfo      `json:"disk"`
}

const socketPath = "/var/run/docker.sock"

func getUnixClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return net.DialTimeout("unix", socketPath, 1*time.Second)
			},
		},
		Timeout: 3 * time.Second,
	}
}

// IsAvailable checks if the Docker daemon unix socket responds
func IsAvailable() bool {
	client := getUnixClient()
	resp, err := client.Get("http://unix/_ping")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// GetTelemetry fetches container list and reclaimable space via unix socket in ~2ms
func GetTelemetry() DockerTelemetry {
	res := DockerTelemetry{
		Available:  false,
		Containers: []ContainerInfo{},
		Disk: DiskDFInfo{
			ImagesReclaimable:     "0 B",
			ContainersReclaimable: "0 B",
			VolumesReclaimable:    "0 B",
			BuildCacheReclaimable: "0 B",
		},
	}

	if !IsAvailable() {
		return res
	}
	res.Available = true

	client := getUnixClient()

	// 1. Get Containers
	resp, err := client.Get("http://unix/containers/json?all=1")
	if err == nil {
		defer resp.Body.Close()
		var rawContainers []struct {
			ID     string   `json:"Id"`
			Names  []string `json:"Names"`
			Image  string   `json:"Image"`
			State  string   `json:"State"`
			Status string   `json:"Status"`
			Ports  []struct {
				PrivatePort int    `json:"PrivatePort"`
				PublicPort  int    `json:"PublicPort"`
				Type        string `json:"Type"`
			} `json:"Ports"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&rawContainers); err == nil {
			for _, rc := range rawContainers {
				name := "unknown"
				if len(rc.Names) > 0 {
					name = strings.TrimPrefix(rc.Names[0], "/")
				}
				cid := rc.ID
				if len(cid) > 12 {
					cid = cid[:12]
				}

				isRunning := strings.ToLower(rc.State) == "running" || strings.Contains(rc.Status, "Up")
				if isRunning {
					res.RunningCount++
				} else {
					res.StoppedCount++
				}

				var portList []string
				for _, p := range rc.Ports {
					if p.PublicPort > 0 {
						portList = append(portList, fmt.Sprintf("%d->%d/%s", p.PublicPort, p.PrivatePort, p.Type))
					} else {
						portList = append(portList, fmt.Sprintf("%d/%s", p.PrivatePort, p.Type))
					}
				}

				res.Containers = append(res.Containers, ContainerInfo{
					ID:        cid,
					Name:      name,
					Image:     rc.Image,
					Status:    rc.Status,
					IsRunning: isRunning,
					Ports:     strings.Join(portList, ", "),
					Stats: ContainerStats{
						CPU: "--",
						Mem: "--",
					},
				})
			}
		}
	}

	// 2. System DF
	dfResp, err := client.Get("http://unix/system/df")
	if err == nil {
		defer dfResp.Body.Close()
		var rawDF struct {
			Images []struct {
				Size   int64 `json:"Size"`
				Shared int64 `json:"SharedSize"`
			} `json:"Images"`
			Containers []struct {
				SizeRw int64 `json:"SizeRw"`
			} `json:"Containers"`
			BuildCache []struct {
				Size int64 `json:"Size"`
			} `json:"BuildCache"`
		}
		if err := json.NewDecoder(dfResp.Body).Decode(&rawDF); err == nil {
			var imgRec, cRec, bRec int64
			for _, img := range rawDF.Images {
				imgRec += img.Size
			}
			for _, c := range rawDF.Containers {
				cRec += c.SizeRw
			}
			for _, b := range rawDF.BuildCache {
				bRec += b.Size
			}

			formatBytes := func(b int64) string {
				if b >= 1024*1024*1024 {
					return fmt.Sprintf("%.2f GB", float64(b)/(1024*1024*1024))
				}
				return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
			}

			res.Disk.ImagesReclaimable = formatBytes(imgRec)
			res.Disk.ContainersReclaimable = formatBytes(cRec)
			res.Disk.BuildCacheReclaimable = formatBytes(bRec)
		}
	}

	return res
}

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

func demuxDockerSocket(raw []byte) string {
	var buf strings.Builder
	idx := 0
	n := len(raw)

	for idx+8 <= n {
		streamType := raw[idx]
		// Docker multiplexed frame header: streamType in [0, 1, 2] and padding bytes == 0
		if streamType <= 2 && raw[idx+1] == 0 && raw[idx+2] == 0 && raw[idx+3] == 0 {
			payloadSize := int(binary.BigEndian.Uint32(raw[idx+4 : idx+8]))
			idx += 8
			if idx+payloadSize <= n {
				buf.Write(raw[idx : idx+payloadSize])
				idx += payloadSize
				continue
			} else {
				buf.Write(raw[idx:])
				break
			}
		} else {
			// Not a demux header (e.g. raw TTY), write the remainder
			buf.Write(raw[idx:])
			break
		}
	}

	if buf.Len() == 0 && n > 0 {
		return string(raw)
	}
	return buf.String()
}

// GetContainerLogs returns recent logs for the specified container.
func GetContainerLogs(containerID string, tail int) (string, error) {
	if tail <= 0 {
		tail = 100
	}

	client := getUnixClient()
	url := fmt.Sprintf("http://unix/containers/%s/logs?stdout=1&stderr=1&tail=%d&timestamps=0", containerID, tail)
	resp, err := client.Get(url)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			raw, err := io.ReadAll(resp.Body)
			if err == nil {
				cleaned := strings.TrimSpace(stripANSI(demuxDockerSocket(raw)))
				if cleaned == "" {
					return "Логи пусты", nil
				}
				return cleaned, nil
			}
		} else if resp.StatusCode == http.StatusNotFound {
			return "", fmt.Errorf("контейнер %s не найден", containerID)
		}
	}

	// Fallback to CLI command if unix socket returned error or is unavailable
	cmd := exec.Command("docker", "logs", "--tail", fmt.Sprintf("%d", tail), containerID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("не удалось получить логи контейнера: %v", err)
	}
	cleaned := strings.TrimSpace(stripANSI(string(out)))
	if cleaned == "" {
		return "Логи пусты", nil
	}
	return cleaned, nil
}

// StartContainer starts a Docker container
func StartContainer(containerID string) error {
	client := getUnixClient()
	resp, err := client.Post(fmt.Sprintf("http://unix/containers/%s/start", containerID), "application/json", nil)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotModified {
			return nil
		}
		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("контейнер %s не найден", containerID)
		}
	}

	cmd := exec.Command("docker", "start", containerID)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ошибка запуска: %s (%v)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// StopContainer stops a Docker container
func StopContainer(containerID string) error {
	client := getUnixClient()
	resp, err := client.Post(fmt.Sprintf("http://unix/containers/%s/stop?t=10", containerID), "application/json", nil)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusNotModified {
			return nil
		}
		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("контейнер %s не найден", containerID)
		}
	}

	cmd := exec.Command("docker", "stop", containerID)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ошибка остановки: %s (%v)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// RestartContainer restarts a Docker container
func RestartContainer(containerID string) error {
	client := getUnixClient()
	resp, err := client.Post(fmt.Sprintf("http://unix/containers/%s/restart?t=10", containerID), "application/json", nil)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNoContent {
			return nil
		}
		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("контейнер %s не найден", containerID)
		}
	}

	cmd := exec.Command("docker", "restart", containerID)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ошибка перезапуска: %s (%v)", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Prune prunes containers, images, or build cache depending on mode
func Prune(mode string) error {
	client := getUnixClient()
	switch mode {
	case "images":
		resp, err := client.Post("http://unix/images/prune?filters=%7B%22dangling%22%3A%5B%22true%22%5D%7D", "application/json", nil)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		cmd := exec.Command("docker", "image", "prune", "-f")
		return cmd.Run()
	case "system":
		if r, err := client.Post("http://unix/containers/prune", "application/json", nil); err == nil {
			_ = r.Body.Close()
		}
		if r, err := client.Post("http://unix/images/prune", "application/json", nil); err == nil {
			_ = r.Body.Close()
		}
		if r, err := client.Post("http://unix/buildcache/prune", "application/json", nil); err == nil {
			_ = r.Body.Close()
		}
		cmd := exec.Command("docker", "system", "prune", "-f")
		return cmd.Run()
	default: // "stopped"
		resp, err := client.Post("http://unix/containers/prune", "application/json", nil)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		cmd := exec.Command("docker", "container", "prune", "-f")
		return cmd.Run()
	}
}

// PruneContainers removes stopped containers via Docker API (backwards compatibility)
func PruneContainers() error {
	return Prune("stopped")
}
