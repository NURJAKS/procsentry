package ports

import (
	"bufio"
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

type PortInfo struct {
	Port     int     `json:"port"`
	IP       string  `json:"ip"`
	PID      int     `json:"pid"`
	Name     string  `json:"name"`
	Cmdline  string  `json:"cmdline"`
	MemoryMB float64 `json:"memory_mb"`
	Hint     string  `json:"hint"`
	CanKill  bool    `json:"can_kill"`
}

var commonDevPorts = map[int]string{
	1433:  "Microsoft SQL Server (MSSQL)",
	3000:  "React / Next.js / Node",
	3001:  "Dev Server / Alternate",
	4000:  "GraphQL / Hexo",
	4200:  "Angular CLI",
	5000:  "Flask / .NET / Python",
	5173:  "Vite / Svelte / Vue",
	8000:  "FastAPI / Django / Uvicorn",
	8080:  "Spring Boot / Tomcat / Web",
	8443:  "HTTPS Dev Server",
	9000:  "PHP-FPM / MinIO",
	9222:  "Chrome Remote Debugger (CDP)",
	5432:  "PostgreSQL",
	6379:  "Redis",
	27017: "MongoDB",
	3306:  "MySQL / MariaDB",
	11434: "Ollama Local LLM API",
	48888: "Dev Laptop Sentinel (Self)",
}

func parseHexIP(hexIP string) string {
	if len(hexIP) == 8 {
		// IPv4
		b0, _ := strconv.ParseInt(hexIP[6:8], 16, 64)
		b1, _ := strconv.ParseInt(hexIP[4:6], 16, 64)
		b2, _ := strconv.ParseInt(hexIP[2:4], 16, 64)
		b3, _ := strconv.ParseInt(hexIP[0:2], 16, 64)
		return fmt.Sprintf("%d.%d.%d.%d", b0, b1, b2, b3)
	}
	return "0.0.0.0"
}

// buildInodePIDMap maps socket inodes to PID in ~0.5ms
func buildInodePIDMap() map[string]int {
	inodeMap := make(map[string]int)
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return inodeMap
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		fdDir := filepath.Join("/proc", entry.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}

		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil {
				continue
			}
			if strings.HasPrefix(link, "socket:[") && strings.HasSuffix(link, "]") {
				inode := link[8 : len(link)-1]
				inodeMap[inode] = pid
			}
		}
	}
	return inodeMap
}

// GetListeningPorts retrieves all listening TCP ports and owning processes in ~1ms
func GetListeningPorts() []PortInfo {
	inodeMap := buildInodePIDMap()
	currentUID := os.Getuid()
	var results []PortInfo
	seen := make(map[int]bool)

	parseProcNet := func(filePath string) {
		f, err := os.Open(filePath)
		if err != nil {
			return
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		isHeader := true
		for scanner.Scan() {
			if isHeader {
				isHeader = false
				continue
			}
			fields := strings.Fields(scanner.Text())
			if len(fields) < 10 {
				continue
			}

			state := fields[3]
			if state != "0A" { // 0A = TCP_LISTEN
				continue
			}

			localAddr := fields[1]
			addrParts := strings.Split(localAddr, ":")
			if len(addrParts) < 2 {
				continue
			}

			portHex := addrParts[1]
			port64, err := strconv.ParseInt(portHex, 16, 64)
			if err != nil {
				continue
			}
			port := int(port64)
			if seen[port] {
				continue
			}
			seen[port] = true

			ip := parseHexIP(addrParts[0])
			inode := fields[9]

			pid := inodeMap[inode]
			procName := "System / Other"
			cmdline := ""
			rssMB := 0.0
			canKill := false

			if pid > 0 {
				procDir := fmt.Sprintf("/proc/%d", pid)
				if commBytes, err := os.ReadFile(filepath.Join(procDir, "comm")); err == nil {
					procName = strings.TrimSpace(string(commBytes))
				}
				if cmdBytes, err := os.ReadFile(filepath.Join(procDir, "cmdline")); err == nil {
					cmdline = strings.TrimSpace(string(bytes.ReplaceAll(cmdBytes, []byte{0}, []byte{' '})))
				}
				if statusBytes, err := os.ReadFile(filepath.Join(procDir, "status")); err == nil {
					for _, line := range strings.Split(string(statusBytes), "\n") {
						if strings.HasPrefix(line, "Uid:") {
							fields := strings.Fields(line)
							if len(fields) > 1 && fields[1] == strconv.Itoa(currentUID) {
								canKill = true
							}
						} else if strings.HasPrefix(line, "VmRSS:") {
							fields := strings.Fields(line)
							if len(fields) > 1 {
								f, _ := strconv.ParseFloat(fields[1], 64)
								rssMB = math.Round((f/1024.0)*10) / 10
							}
						}
					}
				}
			}

			hint := "Custom Dev Service"
			if h, ok := commonDevPorts[port]; ok {
				hint = h
			} else if strings.Contains(strings.ToLower(cmdline), "chrome") {
				hint = "Chrome Browser Debugger"
			} else if port < 1024 {
				hint = "System Port"
			}

			results = append(results, PortInfo{
				Port:     port,
				IP:       ip,
				PID:      pid,
				Name:     procName,
				Cmdline:  cmdline,
				MemoryMB: rssMB,
				Hint:     hint,
				CanKill:  canKill,
			})
		}
	}

	parseProcNet("/proc/net/tcp")
	parseProcNet("/proc/net/tcp6")

	// Sort ports ascending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Port < results[j].Port
	})

	return results
}

// FreePort kills the process occupying the specified port
func FreePort(port int) error {
	ports := GetListeningPorts()
	for _, p := range ports {
		if p.Port == port {
			if p.PID <= 0 {
				return fmt.Errorf("процесс на порту %d не определен", port)
			}
			return syscall.Kill(p.PID, syscall.SIGKILL)
		}
	}
	return fmt.Errorf("порт %d не найден среди активных", port)
}
