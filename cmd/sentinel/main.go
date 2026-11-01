package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"dev-sentinel/pkg/docker"
	"dev-sentinel/pkg/hardware"
	"dev-sentinel/pkg/ports"
	"dev-sentinel/pkg/processes"
	"dev-sentinel/pkg/tui"
	"dev-sentinel/pkg/webserver"
)

const defaultPort = 48888

func sendNotification(title, msg string) {
	_ = exec.Command("notify-send", "-a", "Dev Sentinel", title, msg).Run()
}

func runWatchdog() {
	time.Sleep(2 * time.Second)
	initial := hardware.GetSystemTelemetry()
	sendNotification("🛡️ Dev Sentinel (Go Engine)",
		fmt.Sprintf("Запущен: CPU %.1f°C | Свободно RAM %.1f GB", initial.CPU.PackageTemp, initial.RAM.AvailableGB))

	var lastAlertTime time.Time
	for {
		time.Sleep(4 * time.Second)
		telem := hardware.GetSystemTelemetry()

		// Alert throttle (at most once every 60s for alerts)
		if time.Since(lastAlertTime) > 60*time.Second {
			if telem.CPU.IsCritical {
				sendNotification("🔥 КРИТИЧЕСКИЙ ПЕРЕГРЕВ CPU!",
					fmt.Sprintf("Температура процессора %.1f°C! Завершите ресурсоемкие задачи.", telem.CPU.PackageTemp))
				lastAlertTime = time.Now()
			} else if telem.RAM.Pressure == "CRITICAL" {
				sendNotification("💾 ОПАСНОСТЬ OOM! Память на исходе!",
					fmt.Sprintf("Свободно лишь %.1f GB RAM! Нажмите [g] для очистки dev-призраков.", telem.RAM.AvailableGB))
				lastAlertTime = time.Now()
			}
		}
	}
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "tui":
			tui.RunTUI()
			return
		case "ghosts", "kill-ghosts":
			count, freed, _ := processes.KillAllGhosts()
			fmt.Printf("✅ Завершено призраков: %d, освобождено памяти: %.1f MB\n", count, freed)
			return
		case "free":
			if len(os.Args) < 3 {
				fmt.Println("Укажите номер порта, например: dev-sentinel free 3000")
				os.Exit(1)
			}
			port, err := strconv.Atoi(os.Args[2])
			if err != nil {
				fmt.Printf("Некорректный порт: %s\n", os.Args[2])
				os.Exit(1)
			}
			if err := ports.FreePort(port); err != nil {
				fmt.Printf("❌ Ошибка: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("✅ Порт :%d успешно освобожден!\n", port)
			return
		case "prune":
			if err := docker.PruneContainers(); err != nil {
				fmt.Printf("❌ Ошибка Docker: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("✅ Остановленные контейнеры Docker очищены!")
			return
		case "status":
			telem := hardware.GetSystemTelemetry()
			fmt.Printf("Статус: %s | CPU: %.1f°C (%.1f%%) | RAM: %.1f/%.1f GB (Свободно: %.1f GB) | BAT: %.0f%% (%.1fW) | NVMe: %.1f°C\n",
				telem.Status, telem.CPU.PackageTemp, telem.CPU.LoadPercent,
				telem.RAM.UsedGB, telem.RAM.TotalGB, telem.RAM.AvailableGB,
				telem.Battery.Percent, telem.Battery.Watts, telem.NVMeTemp)
			return
		case "--help", "-h", "help":
			fmt.Println("Dev Sentinel (Go Engine) — Персональный страж ноутбука разработчика")
			fmt.Println("\nИспользование:")
			fmt.Println("  dev-sentinel              Запуск сервиса мониторинга и веб-дашборда")
			fmt.Println("  dev-sentinel tui          Запуск экстренного терминального HUD (ЧП-режим)")
			fmt.Println("  dev-sentinel ghosts       1-клик завершение всех забытых dev-серверов")
			fmt.Println("  dev-sentinel free <порт>  1-клик освобождение порта")
			fmt.Println("  dev-sentinel prune        Очистка остановленных контейнеров Docker")
			fmt.Println("  dev-sentinel status       Краткий снимок состояния системы")
			return
		}
	}

	portFlag := flag.Int("port", defaultPort, "Web dashboard port")
	flag.Parse()

	// Locate static directory
	exePath, err := os.Executable()
	baseDir := "."
	if err == nil {
		baseDir = filepath.Dir(exePath)
	}
	staticDir := filepath.Join(baseDir, "web", "static")
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		// Try from Dev directory
		staticDir = "/home/nurjaks/Dev/dev-sentinel/web/static"
	}

	fmt.Println("==================================================")
	fmt.Printf("🛡️  Dev Sentinel (Go Ultra-Speed Engine) на порту %d\n", *portFlag)
	fmt.Printf("    Веб-дашборд: http://localhost:%d\n", *portFlag)
	fmt.Println("    Терминальный TUI: dev-sentinel tui")
	fmt.Println("    Память бинарника: ~15 MB | Отклик: < 1 ms")
	fmt.Println("    Ноутбук: Acer Nitro 5 (AN515-57) • Ubuntu Linux")
	fmt.Println("==================================================")

	// Start background alert watchdog
	go runWatchdog()

	// Start high-speed HTTP server
	if err := webserver.StartServer(*portFlag, staticDir); err != nil {
		fmt.Printf("Ошибка запуска сервера: %v\n", err)
		os.Exit(1)
	}
}
