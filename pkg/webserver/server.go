package webserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/NURJAKS/procsentry/pkg/cleaner"
	"github.com/NURJAKS/procsentry/pkg/docker"
	"github.com/NURJAKS/procsentry/pkg/hardware"
	"github.com/NURJAKS/procsentry/pkg/ports"
	"github.com/NURJAKS/procsentry/pkg/processes"
)

type KillReq struct {
	PID   int  `json:"pid"`
	Force bool `json:"force"`
}

type FreePortReq struct {
	Port int `json:"port"`
}

type CleanCacheReq struct {
	Name string `json:"name"`
}

type ContainerActionReq struct {
	ID string `json:"id"`
}

type DockerPruneReq struct {
	Mode string `json:"mode"`
}

// StartServer starts the high-speed Go web server on the specified port
func StartServer(port int, staticDir string) error {
	mux := http.NewServeMux()

	// API Telemetry
	mux.HandleFunc("/api/telemetry", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		data := hardware.GetSystemTelemetry()
		_ = json.NewEncoder(w).Encode(data)
	})

	// API Processes
	mux.HandleFunc("/api/processes", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		data := processes.GetDevProcesses()
		_ = json.NewEncoder(w).Encode(data)
	})

	mux.HandleFunc("/api/processes/kill", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var req KillReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := processes.KillProcess(req.PID, req.Force); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "message": fmt.Sprintf("Процесс %d завершен", req.PID)})
	})

	mux.HandleFunc("/api/processes/kill_ghosts", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		count, freed, _ := processes.KillAllGhosts()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":  true,
			"killed":   count,
			"freed_mb": freed,
			"message":  fmt.Sprintf("Завершено %d забытых dev-процессов, освобождено %.1f MB", count, freed),
		})
	})

	// API Ports
	mux.HandleFunc("/api/ports", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		data := ports.GetListeningPorts()
		_ = json.NewEncoder(w).Encode(data)
	})

	mux.HandleFunc("/api/ports/free", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var req FreePortReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := ports.FreePort(req.Port); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "message": fmt.Sprintf("Порт %d успешно освобожден", req.Port)})
	})

	// API Docker
	mux.HandleFunc("/api/docker", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		data := docker.GetTelemetry()
		_ = json.NewEncoder(w).Encode(data)
	})

	mux.HandleFunc("/api/docker/logs/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		id := strings.TrimPrefix(r.URL.Path, "/api/docker/logs/")
		id = strings.TrimSpace(id)
		if id == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"detail": "ID контейнера обязателен"})
			return
		}

		tail := 100
		if tailStr := r.URL.Query().Get("tail"); tailStr != "" {
			if t, err := strconv.Atoi(tailStr); err == nil && t > 0 {
				tail = t
			}
		}

		logs, err := docker.GetContainerLogs(id, tail)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"detail": err.Error()})
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"logs":    logs,
		})
	})

	mux.HandleFunc("/api/docker/start", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			http.Error(w, `{"detail":"Method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		var req ContainerActionReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"detail": "Некорректный запрос или ID контейнера"})
			return
		}
		if err := docker.StartContainer(req.ID); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"detail": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "message": fmt.Sprintf("Контейнер %s запущен", req.ID)})
	})

	mux.HandleFunc("/api/docker/stop", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			http.Error(w, `{"detail":"Method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		var req ContainerActionReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"detail": "Некорректный запрос или ID контейнера"})
			return
		}
		if err := docker.StopContainer(req.ID); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"detail": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "message": fmt.Sprintf("Контейнер %s остановлен", req.ID)})
	})

	mux.HandleFunc("/api/docker/restart", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodPost {
			http.Error(w, `{"detail":"Method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		var req ContainerActionReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"detail": "Некорректный запрос или ID контейнера"})
			return
		}
		if err := docker.RestartContainer(req.ID); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"detail": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "message": fmt.Sprintf("Контейнер %s перезапущен", req.ID)})
	})

	mux.HandleFunc("/api/docker/prune", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var req DockerPruneReq
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Mode == "" {
			req.Mode = "stopped"
		}
		if err := docker.Prune(req.Mode); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"detail": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "message": fmt.Sprintf("Docker %s prune выполнен", req.Mode)})
	})

	// API Caches
	mux.HandleFunc("/api/caches", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		data := cleaner.ScanCaches(false)
		_ = json.NewEncoder(w).Encode(data)
	})

	mux.HandleFunc("/api/caches/clean", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var req CleanCacheReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := cleaner.CleanCache(req.Name); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "message": fmt.Sprintf("Кэш %s очищен", req.Name)})
	})

	// Static Files
	if _, err := os.Stat(staticDir); err == nil {
		fs := http.FileServer(http.Dir(staticDir))
		mux.Handle("/static/", http.StripPrefix("/static/", fs))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/" || r.URL.Path == "/index.html" {
				http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
				return
			}
			http.NotFound(w, r)
		})
	}

	addr := ":" + strconv.Itoa(port)
	return http.ListenAndServe(addr, mux)
}
