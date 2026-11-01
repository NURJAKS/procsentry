package processes

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type ProcessInfo struct {
	PID         int     `json:"pid"`
	PPID        int     `json:"ppid"`
	Name        string  `json:"name"`
	Category    string  `json:"category"`
	Title       string  `json:"title"`
	Subtitle    string  `json:"subtitle"`
	Project     string  `json:"project"`
	Cmdline     string  `json:"cmdline"`
	MemoryMB    float64 `json:"memory_mb"`
	CPUPercent  float64 `json:"cpu_percent"`
	UptimeHours float64 `json:"uptime_hours"`
	IsGhost     bool    `json:"is_ghost"`
	IsService   bool    `json:"is_service"`
}

var devKeywords = map[string][]string{
	"frontend": {"node", "vite", "next", "nuxt", "webpack", "esbuild", "bun", "deno", "ts-node"},
	"backend":  {"python", "python3", "uvicorn", "gunicorn", "flask", "django", "celery", "tracker_bot", "scrapy"},
	"database": {"postgres", "redis-server", "mongod", "mysqld"},
	"compiler": {"cargo", "rustc", "go", "javac", "gradle"},
}

var ghostExclusions = []string{
	"tracker_bot",
	"jobradar",
	"notify.tracker_bot",
	"mcp-server",
	"mcp-server-fetch",
	"mcp-server-github",
	"mcp-server-filesystem",
}

var projectRegex = regexp.MustCompile(`/Dev/([^/\s]+)`)

func extractProject(cmdline, cwd string) string {
	if m := projectRegex.FindStringSubmatch(cmdline); len(m) > 1 {
		return "~/Dev/" + m[1]
	}
	if m := projectRegex.FindStringSubmatch(cwd); len(m) > 1 {
		return "~/Dev/" + m[1]
	}
	return ""
}

func isSystemdService(pid int) bool {
	cgroupPath := fmt.Sprintf("/proc/%d/cgroup", pid)
	data, err := os.ReadFile(cgroupPath)
	if err != nil {
		return false
	}
	str := string(data)
	return strings.Contains(str, "/system.slice") || strings.Contains(str, ".service")
}

func parseCategory(name, cmdline string) string {
	cmdLower := strings.ToLower(cmdline)
	nameLower := strings.ToLower(name)

	// Exclude IDE / system internals
	if strings.Contains(cmdLower, "antigravity-ide") || strings.Contains(cmdLower, "language_server") || strings.Contains(cmdLower, "systemd") {
		return ""
	}
	if strings.Contains(cmdLower, "gnome-terminal") || strings.Contains(cmdLower, "chrome") {
		return ""
	}

	for cat, keywords := range devKeywords {
		for _, kw := range keywords {
			if strings.Contains(nameLower, kw) || strings.Contains(" "+cmdLower+" ", " "+kw+" ") || strings.HasPrefix(cmdLower, kw+" ") {
				return cat
			}
		}
	}
	return ""
}

func parseFriendlyMeta(name, cmdline string) (title, subtitle, category, project string) {
	cmdLower := strings.ToLower(cmdline)

	if strings.Contains(cmdLower, "mcp-server-fetch") {
		return "MCP: Web Fetcher (AI)", "Инструмент Antigravity AI для парсинга веб", "ai", "Antigravity AI"
	}
	if strings.Contains(cmdLower, "mcp-server-github") {
		return "MCP: GitHub Integration", "Инструмент Antigravity AI для Git", "ai", "Antigravity AI"
	}
	if strings.Contains(cmdLower, "mcp-server-filesystem") {
		return "MCP: FileSystem Bridge", "Мост доступа к файловой системе", "ai", "Antigravity AI"
	}
	if strings.Contains(cmdLower, "jobradar") || strings.Contains(cmdLower, "tracker_bot") || strings.Contains(cmdLower, "auto-отклики") {
		return "Job Tracker Telegram Bot", "Фоновый бот отслеживания откликов", "bot", "~/Dev/auto-отклики"
	}
	if strings.Contains(cmdLower, "vite") {
		return "Vite Dev Server", "Локальный веб-сервер фронтенда (HMR)", "frontend", extractProject(cmdline, "")
	}
	if strings.Contains(cmdLower, "next") && (strings.Contains(cmdLower, "dev") || strings.Contains(cmdLower, "start")) {
		return "Next.js Server", "Сервер приложения Next.js", "frontend", extractProject(cmdline, "")
	}
	if strings.Contains(cmdLower, "uvicorn") {
		return "Uvicorn (FastAPI / ASGI)", "Высокопроизводительный Python API сервер", "backend", extractProject(cmdline, "")
	}
	if strings.Contains(cmdLower, "gunicorn") {
		return "Gunicorn WSGI Server", "Python веб-сервер приложений", "backend", extractProject(cmdline, "")
	}

	return name, cmdline, parseCategory(name, cmdline), extractProject(cmdline, "")
}

// GetDevProcesses scans /proc and extracts active developer processes in ~1-2ms
func GetDevProcesses() []ProcessInfo {
	currentUID := os.Getuid()
	selfPID := os.Getpid()
	var results []ProcessInfo

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return results
	}

	bootTime := int64(0)
	if b, err := os.ReadFile("/proc/stat"); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "btime ") {
				fields := strings.Fields(line)
				if len(fields) > 1 {
					bootTime, _ = strconv.ParseInt(fields[1], 10, 64)
				}
				break
			}
		}
	}
	now := time.Now().Unix()

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == selfPID {
			continue
		}

		procDir := filepath.Join("/proc", entry.Name())

		// Read UID
		statusBytes, err := os.ReadFile(filepath.Join(procDir, "status"))
		if err != nil {
			continue
		}
		var realUID int
		var vmRSSKB float64
		for _, line := range strings.Split(string(statusBytes), "\n") {
			if strings.HasPrefix(line, "Uid:") {
				fields := strings.Fields(line)
				if len(fields) > 1 {
					realUID, _ = strconv.Atoi(fields[1])
				}
			} else if strings.HasPrefix(line, "VmRSS:") {
				fields := strings.Fields(line)
				if len(fields) > 1 {
					vmRSSKB, _ = strconv.ParseFloat(fields[1], 64)
				}
			}
		}
		if realUID != currentUID {
			continue
		}

		// Read Cmdline
		cmdlineBytes, _ := os.ReadFile(filepath.Join(procDir, "cmdline"))
		cmdline := string(bytes.ReplaceAll(cmdlineBytes, []byte{0}, []byte{' '}))
		cmdline = strings.TrimSpace(cmdline)

		// Read Comm
		commBytes, _ := os.ReadFile(filepath.Join(procDir, "comm"))
		name := strings.TrimSpace(string(commBytes))

		category := parseCategory(name, cmdline)
		if category == "" {
			continue
		}

		// Read Stat (for PPID and start time)
		statBytes, _ := os.ReadFile(filepath.Join(procDir, "stat"))
		statFields := strings.Fields(string(statBytes))
		ppid := 0
		var uptimeHours float64
		if len(statFields) > 21 {
			ppid, _ = strconv.Atoi(statFields[3])
			startTimeTicks, _ := strconv.ParseInt(statFields[21], 10, 64)
			// Clock ticks usually 100 Hz
			procStartSec := bootTime + (startTimeTicks / 100)
			uptimeHours = math.Round((float64(now-procStartSec)/3600.0)*10) / 10
		}

		cwdLink, _ := os.Readlink(filepath.Join(procDir, "cwd"))

		// Ghost detection
		isService := isSystemdService(pid)
		cmdLower := strings.ToLower(cmdline)
		isWhitelisted := false
		for _, ex := range ghostExclusions {
			if strings.Contains(cmdLower, ex) {
				isWhitelisted = true
				break
			}
		}

		isGhost := false
		if !isWhitelisted && !isService {
			isGhost = (ppid == 1) || (uptimeHours > 3.0)
		}

		title, subtitle, parsedCat, proj := parseFriendlyMeta(name, cmdline)
		if proj == "" {
			proj = extractProject(cmdline, cwdLink)
		}
		if parsedCat == "" {
			parsedCat = category
		}

		rssMB := math.Round((vmRSSKB/1024.0)*10) / 10

		results = append(results, ProcessInfo{
			PID:         pid,
			PPID:        ppid,
			Name:        name,
			Category:    parsedCat,
			Title:       title,
			Subtitle:    subtitle,
			Project:     proj,
			Cmdline:     cmdline,
			MemoryMB:    rssMB,
			CPUPercent:  0.0,
			UptimeHours: uptimeHours,
			IsGhost:     isGhost,
			IsService:   isService || isWhitelisted,
		})
	}

	// Sort: ghosts first, then by memory descending
	sort.Slice(results, func(i, j int) bool {
		if results[i].IsGhost != results[j].IsGhost {
			return results[i].IsGhost
		}
		return results[i].MemoryMB > results[j].MemoryMB
	})

	return results
}

// KillProcess terminates a process cleanly, falling back to SIGKILL
func KillProcess(pid int, force bool) error {
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}

	if err := syscall.Kill(pid, sig); err != nil {
		return err
	}

	if !force {
		// Wait 150ms and check if still alive
		time.Sleep(150 * time.Millisecond)
		if err := syscall.Kill(pid, syscall.Signal(0)); err == nil {
			// Still running, send SIGKILL
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
	return nil
}

// KillAllGhosts terminates all detected ghost processes in ~5ms
func KillAllGhosts() (killedCount int, freedMB float64, err error) {
	procs := GetDevProcesses()
	for _, p := range procs {
		if p.IsGhost {
			if kErr := KillProcess(p.PID, true); kErr == nil {
				killedCount++
				freedMB += p.MemoryMB
			}
		}
	}
	freedMB = math.Round(freedMB*10) / 10
	return killedCount, freedMB, nil
}
