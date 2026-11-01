package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
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

// PruneContainers removes stopped containers via Docker API
func PruneContainers() error {
	client := getUnixClient()
	resp, err := client.Post("http://unix/containers/prune", "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}
