package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/NURJAKS/procsentry/pkg/docker"
	"github.com/NURJAKS/procsentry/pkg/hardware"
	"github.com/NURJAKS/procsentry/pkg/ports"
	"github.com/NURJAKS/procsentry/pkg/processes"
	"github.com/NURJAKS/procsentry/pkg/tui"
	"github.com/NURJAKS/procsentry/pkg/webserver"
)

const defaultPort = 48888

func sendNotification(title, msg string) {
	_ = exec.Command("notify-send", "-a", "ProcSentry", title, msg).Run()
}

func runWatchdog() {
	time.Sleep(2 * time.Second)
	initial := hardware.GetSystemTelemetry()
	sendNotification("ProcSentry Watchdog",
		fmt.Sprintf("Started: CPU %.1f°C | Free RAM %.1f GB", initial.CPU.PackageTemp, initial.RAM.AvailableGB))

	var lastAlertTime time.Time
	for {
		time.Sleep(4 * time.Second)
		telem := hardware.GetSystemTelemetry()

		// Alert throttle (at most once every 60s for alerts)
		if time.Since(lastAlertTime) > 60*time.Second {
			if telem.CPU.IsCritical {
				sendNotification("[ALERT] High CPU Temperature",
					fmt.Sprintf("CPU reached %.1f°C. Check runaway tasks.", telem.CPU.PackageTemp))
				lastAlertTime = time.Now()
			} else if telem.RAM.Pressure == "CRITICAL" {
				sendNotification("[ALERT] Low Memory Warning",
					fmt.Sprintf("Only %.1f GB RAM free. Execute 'aegis ghosts' to reclaim memory.", telem.RAM.AvailableGB))
				lastAlertTime = time.Now()
			}
		}
	}
}

func runTrayIndicator() {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return
	}
	time.Sleep(1 * time.Second)

	// Check if already running
	if err := exec.Command("pgrep", "-f", "tray_indicator.py").Run(); err == nil {
		return
	}

	exePath, err := os.Executable()
	baseDir := "."
	if err == nil {
		baseDir = filepath.Dir(exePath)
	}
	homeDir, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(baseDir, "tray", "tray_indicator.py"),
		filepath.Join(baseDir, "..", "tray", "tray_indicator.py"),
		filepath.Join(homeDir, "Dev", "dev-sentinel", "tray", "tray_indicator.py"),
	}

	var trayScript string
	for _, cand := range candidates {
		if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
			trayScript = cand
			break
		}
	}

	if trayScript != "" {
		cmd := exec.Command("python3", trayScript)
		cmd.Env = os.Environ()
		_ = cmd.Start()
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
			fmt.Printf("[OK] Terminated %d ghost processes, reclaimed %.1f MB RAM\n", count, freed)
			return
		case "free":
			if len(os.Args) < 3 {
				fmt.Println("Usage: aegis free <port>")
				os.Exit(1)
			}
			port, err := strconv.Atoi(os.Args[2])
			if err != nil {
				fmt.Printf("[ERROR] Invalid port: %s\n", os.Args[2])
				os.Exit(1)
			}
			if err := ports.FreePort(port); err != nil {
				fmt.Printf("[ERROR] %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("[OK] Port :%d released successfully\n", port)
			return
		case "prune":
			if err := docker.PruneContainers(); err != nil {
				fmt.Printf("[ERROR] Docker: %v\n", err)
				os.Exit(1)
			}
			fmt.Println("[OK] Stopped Docker containers and caches pruned")
			return
		case "status":
			telem := hardware.GetSystemTelemetry()
			fmt.Printf("Status: %s | CPU: %.1f°C (%.1f%%) | RAM: %.1f/%.1f GB (Free: %.1f GB) | BAT: %.0f%% (%.1fW) | NVMe: %.1f°C\n",
				telem.Status, telem.CPU.PackageTemp, telem.CPU.LoadPercent,
				telem.RAM.UsedGB, telem.RAM.TotalGB, telem.RAM.AvailableGB,
				telem.Battery.Percent, telem.Battery.Watts, telem.NVMeTemp)
			return
		case "--help", "-h", "help":
			fmt.Println("ProcSentry — Zero-Overhead Linux Systems Guardian & Dev Workspace HUD")
			fmt.Println("\nUsage:")
			fmt.Println("  procsentry              Start daemon (telemetry service & web server)")
			fmt.Println("  procsentry tui          Launch emergency terminal HUD")
			fmt.Println("  procsentry ghosts       Terminate orphaned background dev processes")
			fmt.Println("  procsentry free <port>  Release listening TCP port")
			fmt.Println("  procsentry prune        Prune stopped Docker containers")
			fmt.Println("  procsentry status       Show single-line hardware summary")
			return
		}
	}

	portFlag := flag.Int("port", defaultPort, "Web dashboard port")
	flag.Parse()

	// Locate static directory dynamically
	exePath, err := os.Executable()
	baseDir := "."
	if err == nil {
		baseDir = filepath.Dir(exePath)
	}

	homeDir, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(baseDir, "web", "static"),
		filepath.Join(baseDir, "..", "web", "static"),
		filepath.Join(".", "web", "static"),
		filepath.Join(homeDir, "Dev", "procsentry", "web", "static"),
		filepath.Join(homeDir, "Dev", "dev-sentinel", "web", "static"),
		filepath.Join(homeDir, "procsentry", "web", "static"),
		filepath.Join(homeDir, ".local", "share", "procsentry", "web", "static"),
		filepath.Join(homeDir, ".local", "share", "dev-sentinel", "web", "static"),
	}

	staticDir := filepath.Join(baseDir, "web", "static")
	for _, cand := range candidates {
		if fi, err := os.Stat(cand); err == nil && fi.IsDir() {
			staticDir = cand
			break
		}
	}

	fmt.Println("==================================================")
	fmt.Printf("ProcSentry Systems Watchdog running on port %d\n", *portFlag)
	fmt.Printf("Web Dashboard: http://localhost:%d\n", *portFlag)
	fmt.Println("Emergency TUI: procsentry tui")
	fmt.Println("Process Memory: ~14 MB | Telemetry Latency: < 1 ms")
	fmt.Println("==================================================")

	// Start background alert watchdog
	go runWatchdog()

	// Start GNOME top-panel tray indicator (if display available)
	go runTrayIndicator()

	// Start high-speed HTTP server
	if err := webserver.StartServer(*portFlag, staticDir); err != nil {
		fmt.Printf("Ошибка запуска сервера: %v\n", err)
		os.Exit(1)
	}
}
