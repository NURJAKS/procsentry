package tui

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"dev-sentinel/pkg/docker"
	"dev-sentinel/pkg/hardware"
	"dev-sentinel/pkg/ports"
	"dev-sentinel/pkg/processes"
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
	bgRed     = "\033[41m"
	bgGreen   = "\033[42m"
	bgYellow  = "\033[43m"
	bgCyan    = "\033[46m"
	clearHome = "\033[H\033[2J"
)

// RunTUI launches the emergency terminal dashboard
func RunTUI() {
	// Enable raw/line input loop
	message := "Готов к работе в экстренных ситуациях (ЧП)."
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
		fmt.Printf("%s%s║   🛡️  DEV SENTINEL — EMERGENCY HUD (Acer Nitro 5 • Ubuntu Linux)             ║%s\n", bold, cyan, reset)
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

		fmt.Printf(" %s⚡ CPU:%s %s%.1f°C%s (%.1f%%) | %s💾 RAM:%s %s%.1f/%.1f GB%s [%s%s%s] | %s🔋 BAT:%s %.0f%% (%.1fW)\n",
			bold, reset, cpuColor, telem.CPU.PackageTemp, reset, telem.CPU.LoadPercent,
			bold, reset, ramColor, telem.RAM.UsedGB, telem.RAM.TotalGB, reset, ramColor, telem.RAM.Pressure, reset,
			bold, reset, telem.Battery.Percent, telem.Battery.Watts)

		fmt.Printf(" %s🔥 NVMe:%s %.1f°C | %s🎮 GPU:%s %s | %s⏱️  Uptime:%s %s\n",
			bold, reset, telem.NVMeTemp,
			bold, reset, telem.GPU.Status,
			bold, reset, time.Duration(time.Now().Unix()-telem.UptimeSec)*time.Second)

		fmt.Printf("%s───────────────────────────────────────────────────────────────────────────────%s\n", dim, reset)

		// 2. Active Ghosts & Ports Summary
		ghostColor := green
		if ghostCount > 0 {
			ghostColor = red
		}
		fmt.Printf(" %sСтатус среды:%s 👻 Призраков: %s%d%s | 🔌 Открытых портов: %s%d%s\n",
			bold, reset, ghostColor, ghostCount, reset, cyan, len(activePorts), reset)

		// 3. Dev Processes List (Top 6)
		fmt.Printf("\n%s%s--- [АКТИВНЫЕ DEV-ПРОЦЕССЫ] ---%s\n", bold, yellow, reset)
		fmt.Printf(" %-6s %-12s %-24s %-10s %-8s %s\n", "PID", "КАТЕГОРИЯ", "ИМЯ / ПРОЕКТ", "RAM", "UPTIME", "СТАТУС")
		limit := 6
		if len(procs) < limit {
			limit = len(procs)
		}
		for i := 0; i < limit; i++ {
			p := procs[i]
			status := fmt.Sprintf("%sАктивен%s", green, reset)
			if p.IsGhost {
				status = fmt.Sprintf("%s%s👻 ПРИЗРАК%s", bold, red, reset)
			} else if p.IsService {
				status = fmt.Sprintf("%s⚙️ Сервис%s", blue, reset)
			}
			proj := p.Project
			if proj == "" {
				proj = p.Name
			}
			if len(proj) > 23 {
				proj = proj[:20] + "..."
			}
			fmt.Printf(" %-6d %-12s %-24s %-10s %-8s %s\n",
				p.PID, p.Category, proj, fmt.Sprintf("%.1f MB", p.MemoryMB), fmt.Sprintf("%.1fh", p.UptimeHours), status)
		}
		if len(procs) == 0 {
			fmt.Printf(" %s(Dev-процессы не обнаружены)%s\n", dim, reset)
		}

		// 4. Listening Ports (Top 6)
		fmt.Printf("\n%s%s--- [ПРОСЛУШИВАЕМЫЕ ПОРТЫ] ---%s\n", bold, cyan, reset)
		fmt.Printf(" %-7s %-28s %-8s %-14s %s\n", "ПОРТ", "СЕРВИС", "PID", "ПРОЦЕСС", "ОСВОБОЖДЕНИЕ")
		pLimit := 6
		if len(activePorts) < pLimit {
			pLimit = len(activePorts)
		}
		for i := 0; i < pLimit; i++ {
			p := activePorts[i]
			killLabel := fmt.Sprintf("%sДа (клик)%s", green, reset)
			if !p.CanKill {
				killLabel = fmt.Sprintf("%sСистемный%s", dim, reset)
			}
			fmt.Printf(" :%-6d %-28s %-8d %-14s %s\n",
				p.Port, p.Hint, p.PID, p.Name, killLabel)
		}
		if len(activePorts) == 0 {
			fmt.Printf(" %s(Нет прослушиваемых портов)%s\n", dim, reset)
		}

		// 5. Actions Bar
		fmt.Printf("\n%s───────────────────────────────────────────────────────────────────────────────%s\n", dim, reset)
		fmt.Printf(" %sДЕЙСТВИЯ (ЧП):%s\n", bold, reset)
		fmt.Printf("  %s[g]%s Завершить ВСЕХ призраков  %s[f <порт>]%s Освободить порт  %s[d]%s Docker Prune  %s[q]%s Выход\n",
			bold+red, reset, bold+yellow, reset, bold+blue, reset, bold, reset)

		if message != "" {
			fmt.Printf("\n %s💡 %s%s\n", bold+green, message, reset)
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
				fmt.Println("Dev Sentinel TUI завершил работу.")
				return
			case "g", "ghosts", "kill-ghosts":
				count, freed, _ := processes.KillAllGhosts()
				message = fmt.Sprintf("✅ Завершено призраков: %d, освобождено памяти: %.1f MB", count, freed)
			case "f", "free":
				if len(parts) > 1 {
					port, err := strconv.Atoi(parts[1])
					if err != nil {
						message = "❌ Ошибка: укажите корректный номер порта, например: f 3000"
					} else {
						if err := ports.FreePort(port); err != nil {
							message = fmt.Sprintf("❌ Ошибка освобождения порта :%d: %v", port, err)
						} else {
							message = fmt.Sprintf("✅ Порт :%d успешно освобожден!", port)
						}
					}
				} else {
					message = "⚠️ Укажите порт, например: f 3000"
				}
			case "d", "docker", "prune":
				if err := docker.PruneContainers(); err != nil {
					message = fmt.Sprintf("❌ Ошибка Docker Prune: %v", err)
				} else {
					message = "✅ Остановленные контейнеры Docker очищены!"
				}
			case "r", "refresh":
				message = "Данные обновлены."
			default:
				message = fmt.Sprintf("Неизвестная команда: %s. Доступно: g, f <порт>, d, q", parts[0])
			}
			render()
		}
	}
}
