package tui

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/NURJAKS/procsentry/pkg/docker"
	"github.com/NURJAKS/procsentry/pkg/hardware"
	"github.com/NURJAKS/procsentry/pkg/ports"
	"github.com/NURJAKS/procsentry/pkg/processes"
)

// ANSI Color codes
const (
	reset     = "\033[0m"
	bold      = "\033[1m"
	dim       = "\033[2m"
	red       = "\033[31m"
	green     = "\033[32m"
	yellow    = "\033[33m"
	blue      = "\033[34m"
	magenta   = "\033[35m"
	cyan      = "\033[36m"
	white     = "\033[37m"
	clearHome = "\033[H\033[2J"
)

// RunTUI launches the emergency terminal dashboard
func RunTUI() {
	message := "System ready. Standby mode active."
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()

	inputChan := make(chan string)
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			inputChan <- strings.TrimSpace(line)
		}
	}()

	render := func() {
		telem := hardware.GetSystemTelemetry()
		procs := processes.GetDevProcesses()
		activePorts := ports.GetListeningPorts()

		ghostCount := 0
		for _, p := range procs {
			if p.IsGhost {
				ghostCount++
			}
		}

		fmt.Print(clearHome)
		fmt.Printf("%s%s╔═══════════════════════════════════════════════════════════════════════════════╗%s\n", bold, cyan, reset)
		fmt.Printf("%s%s║   PROCSENTRY — SYSTEM RESCUE HUD (Linux x86_64)                               ║%s\n", bold, cyan, reset)
		fmt.Printf("%s%s╚═══════════════════════════════════════════════════════════════════════════════╝%s\n", bold, cyan, reset)

		// 1. Hardware Status Row
		cpuColor := green
		if telem.CPU.IsCritical {
			cpuColor = red
		} else if telem.CPU.IsHot {
			cpuColor = yellow
		}

		ramColor := green
		if telem.RAM.Pressure == "CRITICAL" {
			ramColor = red
		} else if telem.RAM.Pressure == "WARNING" {
			ramColor = yellow
		}

		fmt.Printf(" %sCPU:%s %s%.1f°C%s (%.1f%%) | %sRAM:%s %s%.1f/%.1f GB%s [%s%s%s] | %sBAT:%s %.0f%% (%.1fW)\n",
			bold, reset, cpuColor, telem.CPU.PackageTemp, reset, telem.CPU.LoadPercent,
			bold, reset, ramColor, telem.RAM.UsedGB, telem.RAM.TotalGB, reset, ramColor, telem.RAM.Pressure, reset,
			bold, reset, telem.Battery.Percent, telem.Battery.Watts)

		fmt.Printf(" %sNVMe:%s %.1f°C | %sGPU:%s %s | %sUptime:%s %s\n",
			bold, reset, telem.NVMeTemp,
			bold, reset, telem.GPU.Status,
			bold, reset, time.Duration(time.Now().Unix()-telem.UptimeSec)*time.Second)

		fmt.Printf("%s───────────────────────────────────────────────────────────────────────────────%s\n", dim, reset)

		// 2. Active Ghosts & Ports Summary
		ghostColor := green
		if ghostCount > 0 {
			ghostColor = red
		}
		fmt.Printf(" %sEnvironment Status:%s Orphaned Processes: %s%d%s | Listening Ports: %s%d%s\n",
			bold, reset, ghostColor, ghostCount, reset, cyan, len(activePorts), reset)

		// 3. Dev Processes List (Top 6)
		fmt.Printf("\n%s%s--- [ACTIVE DEV PROCESSES] ---%s\n", bold, yellow, reset)
		fmt.Printf(" %-6s %-12s %-26s %-10s %-8s %s\n", "PID", "CATEGORY", "NAME / PROJECT", "RAM", "UPTIME", "STATUS")
		limit := 6
		if len(procs) < limit {
			limit = len(procs)
		}
		for i := 0; i < limit; i++ {
			p := procs[i]
			status := fmt.Sprintf("%s[ACTIVE]%s", green, reset)
			if p.IsGhost {
				status = fmt.Sprintf("%s%s[ORPHAN]%s", bold, red, reset)
			} else if p.IsService {
				status = fmt.Sprintf("%s[SERVICE]%s", blue, reset)
			}
			proj := p.Project
			if proj == "" {
				proj = p.Name
			}
			if len(proj) > 25 {
				proj = proj[:22] + "..."
			}
			fmt.Printf(" %-6d %-12s %-26s %-10s %-8s %s\n",
				p.PID, p.Category, proj, fmt.Sprintf("%.1f MB", p.MemoryMB), fmt.Sprintf("%.1fh", p.UptimeHours), status)
		}
		if len(procs) == 0 {
			fmt.Printf(" %s(No development processes detected)%s\n", dim, reset)
		}

		// 4. Listening Ports (Top 6)
		fmt.Printf("\n%s%s--- [LISTENING TCP PORTS] ---%s\n", bold, cyan, reset)
		fmt.Printf(" %-7s %-28s %-8s %-16s %s\n", "PORT", "SERVICE", "PID", "PROCESS", "RELEASABLE")
		pLimit := 6
		if len(activePorts) < pLimit {
			pLimit = len(activePorts)
		}
		for i := 0; i < pLimit; i++ {
			p := activePorts[i]
			killLabel := fmt.Sprintf("%sYes (f %d)%s", green, p.Port, reset)
			if !p.CanKill {
				killLabel = fmt.Sprintf("%sSystem%s", dim, reset)
			}
			fmt.Printf(" :%-6d %-28s %-8d %-16s %s\n",
				p.Port, p.Hint, p.PID, p.Name, killLabel)
		}
		if len(activePorts) == 0 {
			fmt.Printf(" %s(No listening ports detected)%s\n", dim, reset)
		}

		// 5. Actions Bar
		fmt.Printf("\n%s───────────────────────────────────────────────────────────────────────────────%s\n", dim, reset)
		fmt.Printf(" %sACTIONS (EMERGENCY):%s\n", bold, reset)
		fmt.Printf("  %s[g]%s Terminate All Orphans  %s[f <port>]%s Release Port  %s[d]%s Docker Prune  %s[q]%s Exit\n",
			bold+red, reset, bold+yellow, reset, bold+blue, reset, bold, reset)

		if message != "" {
			fmt.Printf("\n %s[INFO] %s%s\n", bold+green, message, reset)
		}
		fmt.Print(" > ")
	}

	render()

	for {
		select {
		case <-ticker.C:
			render()
		case cmd := <-inputChan:
			parts := strings.Fields(cmd)
			if len(parts) == 0 {
				render()
				continue
			}
			switch strings.ToLower(parts[0]) {
			case "q", "exit", "quit":
				fmt.Print(clearHome)
				fmt.Println("ProcSentry TUI terminated.")
				return
			case "g", "ghosts", "kill-ghosts":
				count, freed, _ := processes.KillAllGhosts()
				message = fmt.Sprintf("[OK] Terminated %d orphan processes. Reclaimed %.1f MB.", count, freed)
			case "f", "free":
				if len(parts) > 1 {
					port, err := strconv.Atoi(parts[1])
					if err != nil {
						message = "[ERROR] Invalid port number. Example: f 3000"
					} else {
						if err := ports.FreePort(port); err != nil {
							message = fmt.Sprintf("[ERROR] Failed to release port :%d: %v", port, err)
						} else {
							message = fmt.Sprintf("[OK] Port :%d released successfully", port)
						}
					}
				} else {
					message = "[WARN] Specify port number. Example: f 3000"
				}
			case "d", "docker", "prune":
				if err := docker.PruneContainers(); err != nil {
					message = fmt.Sprintf("[ERROR] Docker prune failed: %v", err)
				} else {
					message = "[OK] Stopped Docker containers and caches pruned."
				}
			case "r", "refresh":
				message = "Telemetry data refreshed."
			default:
				message = fmt.Sprintf("Unknown command: %s. Valid: g, f <port>, d, q", parts[0])
			}
			render()
		}
	}
}
