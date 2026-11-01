package hardware

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type CoreTemp struct {
	Name string  `json:"name"`
	Temp float64 `json:"temp"`
}

type CPUInfo struct {
	LoadPercent float64    `json:"load_percent"`
	LoadPerCore []float64  `json:"load_per_core"`
	PackageTemp float64    `json:"package_temp"`
	Cores       []CoreTemp `json:"cores"`
	FreqMHz     float64    `json:"current_freq_mhz"`
	IsHot       bool       `json:"is_hot"`
	IsCritical  bool       `json:"is_critical"`
	CoreCount   int        `json:"core_count"`
}

type RAMInfo struct {
	TotalGB     float64 `json:"total_gb"`
	UsedGB      float64 `json:"used_gb"`
	AvailableGB float64 `json:"available_gb"`
	CachedGB    float64 `json:"cached_gb"`
	Percent     float64 `json:"percent"`
	Pressure    string  `json:"pressure"` // HEALTHY, WARNING, CRITICAL
	SwapTotalGB float64 `json:"swap_total_gb"`
	SwapUsedGB  float64 `json:"swap_used_gb"`
	SwapPercent float64 `json:"swap_percent"`
}

type BatteryInfo struct {
	Percent    float64  `json:"percent"`
	IsPlugged  bool     `json:"is_plugged"`
	Status     string   `json:"status"`
	Watts      float64  `json:"watts"`
	Voltage    float64  `json:"voltage"`
	HealthPct  float64  `json:"health_pct"`
	WearPct    float64  `json:"wear_pct"`
	CycleCount int      `json:"cycle_count"`
	Tips       []string `json:"tips"`
}

type GPUInfo struct {
	Mode   string  `json:"mode"`
	Status string  `json:"status"`
	Temp   float64 `json:"temp"`
}

type SystemTelemetry struct {
	Status      string      `json:"status"` // NOMINAL, WARNING, CRITICAL
	StatusText  string      `json:"status_text"`
	StatusColor string      `json:"status_color"`
	CPU         CPUInfo     `json:"cpu"`
	RAM         RAMInfo     `json:"ram"`
	Battery     BatteryInfo `json:"battery"`
	NVMeTemp    float64     `json:"nvme_temp"`
	GPU         GPUInfo     `json:"gpu"`
	UptimeSec   int64       `json:"uptime"`
}

// CPU tick tracker for precise deltas
type cpuTicks struct {
	user, nice, sys, idle, iowait, irq, softirq, steal uint64
}

func (t cpuTicks) total() uint64 {
	return t.user + t.nice + t.sys + t.idle + t.iowait + t.irq + t.softirq + t.steal
}

func (t cpuTicks) idleTotal() uint64 {
	return t.idle + t.iowait
}

var (
	prevOverallTicks cpuTicks
	prevCoreTicks    []cpuTicks
	ticksMutex       sync.Mutex
	hwmonCoretemp    string
	hwmonNVMe        string
	hwmonInitialized bool
	hwmonInitMutex   sync.Mutex
	cachedGPUMode    = "unknown"
	lastGPUModeCheck time.Time
)

func initHwmon() {
	hwmonInitMutex.Lock()
	defer hwmonInitMutex.Unlock()
	if hwmonInitialized {
		return
	}

	matches, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	for _, p := range matches {
		nameBytes, err := os.ReadFile(filepath.Join(p, "name"))
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(nameBytes))
		if name == "coretemp" {
			hwmonCoretemp = p
		} else if name == "nvme" {
			hwmonNVMe = p
		}
	}
	hwmonInitialized = true
}

// GetCPUInfo reads CPU load and temperatures from /proc and /sys in ~0.2ms
func GetCPUInfo() CPUInfo {
	initHwmon()
	ticksMutex.Lock()
	defer ticksMutex.Unlock()

	statFile, err := os.Open("/proc/stat")
	info := CPUInfo{
		PackageTemp: 50.0,
		LoadPerCore: []float64{},
		Cores:       []CoreTemp{},
	}

	if err == nil {
		defer statFile.Close()
		scanner := bufio.NewScanner(statFile)
		var currentCoreTicks []cpuTicks

		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "cpu ") {
				var user, nice, sys, idle, iowait, irq, softirq, steal uint64
				fmt.Sscanf(line, "cpu  %d %d %d %d %d %d %d %d",
					&user, &nice, &sys, &idle, &iowait, &irq, &softirq, &steal)
				curr := cpuTicks{user, nice, sys, idle, iowait, irq, softirq, steal}

				if prevOverallTicks.total() > 0 {
					totalDelta := float64(curr.total() - prevOverallTicks.total())
					idleDelta := float64(curr.idleTotal() - prevOverallTicks.idleTotal())
					if totalDelta > 0 {
						pct := (1.0 - (idleDelta / totalDelta)) * 100.0
						info.LoadPercent = math.Round(math.Max(0, math.Min(100, pct))*10) / 10
					}
				}
				prevOverallTicks = curr
			} else if strings.HasPrefix(line, "cpu") && len(line) > 3 && line[3] >= '0' && line[3] <= '9' {
				var cpuID int
				var user, nice, sys, idle, iowait, irq, softirq, steal uint64
				fmt.Sscanf(line, "cpu%d %d %d %d %d %d %d %d %d",
					&cpuID, &user, &nice, &sys, &idle, &iowait, &irq, &softirq, &steal)
				curr := cpuTicks{user, nice, sys, idle, iowait, irq, softirq, steal}
				currentCoreTicks = append(currentCoreTicks, curr)

				coreIdx := len(currentCoreTicks) - 1
				var coreLoad float64
				if coreIdx < len(prevCoreTicks) && prevCoreTicks[coreIdx].total() > 0 {
					totalDelta := float64(curr.total() - prevCoreTicks[coreIdx].total())
					idleDelta := float64(curr.idleTotal() - prevCoreTicks[coreIdx].idleTotal())
					if totalDelta > 0 {
						pct := (1.0 - (idleDelta / totalDelta)) * 100.0
						coreLoad = math.Round(math.Max(0, math.Min(100, pct))*10) / 10
					}
				}
				info.LoadPerCore = append(info.LoadPerCore, coreLoad)
			}
		}
		if len(currentCoreTicks) > 0 {
			prevCoreTicks = currentCoreTicks
			info.CoreCount = len(currentCoreTicks)
		}
	}

	// Read CPU Freq
	if freqBytes, err := os.ReadFile("/sys/devices/system/cpu/cpu0/cpufreq/scaling_cur_freq"); err == nil {
		if khz, err := strconv.ParseFloat(strings.TrimSpace(string(freqBytes)), 64); err == nil {
			info.FreqMHz = math.Round(khz / 1000.0)
		}
	}

	// Read Temps from coretemp hwmon
	if hwmonCoretemp != "" {
		// Package temp
		if b, err := os.ReadFile(filepath.Join(hwmonCoretemp, "temp1_input")); err == nil {
			if millideg, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64); err == nil {
				info.PackageTemp = math.Round(millideg / 100.0) / 10.0
			}
		}
		// Physical cores temp (temp2_input .. temp7_input)
		for i := 2; i <= 7; i++ {
			tempPath := filepath.Join(hwmonCoretemp, fmt.Sprintf("temp%d_input", i))
			labelPath := filepath.Join(hwmonCoretemp, fmt.Sprintf("temp%d_label", i))
			if b, err := os.ReadFile(tempPath); err == nil {
				if millideg, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64); err == nil {
					coreTemp := math.Round(millideg / 100.0) / 10.0
					label := fmt.Sprintf("Core %d", i-2)
					if lb, err := os.ReadFile(labelPath); err == nil {
						label = strings.TrimSpace(string(lb))
					}
					info.Cores = append(info.Cores, CoreTemp{
						Name: label,
						Temp: coreTemp,
					})
				}
			}
		}
	}

	info.IsHot = info.PackageTemp >= 85.0
	info.IsCritical = info.PackageTemp >= 92.0
	return info
}

// GetRAMInfo reads memory metrics from /proc/meminfo in ~0.05ms
func GetRAMInfo() RAMInfo {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return RAMInfo{Pressure: "HEALTHY"}
	}
	defer f.Close()

	var memTotalKB, memAvailKB, buffersKB, cachedKB, swapTotalKB, swapFreeKB float64

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		val, _ := strconv.ParseFloat(fields[1], 64)
		switch fields[0] {
		case "MemTotal:":
			memTotalKB = val
		case "MemAvailable:":
			memAvailKB = val
		case "Buffers:":
			buffersKB = val
		case "Cached:":
			cachedKB = val
		case "SwapTotal:":
			swapTotalKB = val
		case "SwapFree:":
			swapFreeKB = val
		}
	}

	kbToGB := 1.0 / (1024.0 * 1024.0)
	totalGB := math.Round(memTotalKB*kbToGB*100) / 100
	availGB := math.Round(memAvailKB*kbToGB*100) / 100
	usedGB := math.Round((memTotalKB-memAvailKB)*kbToGB*100) / 100
	cacheGB := math.Round((buffersKB+cachedKB)*kbToGB*100) / 100

	pct := 0.0
	if totalGB > 0 {
		pct = math.Round((usedGB/totalGB)*1000) / 10
	}

	swapTotal := math.Round(swapTotalKB*kbToGB*100) / 100
	swapUsed := math.Round((swapTotalKB-swapFreeKB)*kbToGB*100) / 100
	swapPct := 0.0
	if swapTotal > 0 {
		swapPct = math.Round((swapUsed/swapTotal)*1000) / 10
	}

	pressure := "HEALTHY"
	if availGB < 1.2 {
		pressure = "CRITICAL"
	} else if availGB < 2.5 {
		pressure = "WARNING"
	}

	return RAMInfo{
		TotalGB:     totalGB,
		UsedGB:      usedGB,
		AvailableGB: availGB,
		CachedGB:    cacheGB,
		Percent:     pct,
		Pressure:    pressure,
		SwapTotalGB: swapTotal,
		SwapUsedGB:  swapUsed,
		SwapPercent: swapPct,
	}
}

// GetBatteryInfo reads from /sys/class/power_supply/BAT1 in ~0.08ms
func GetBatteryInfo() BatteryInfo {
	batDir := "/sys/class/power_supply/BAT1"
	if _, err := os.Stat(batDir); os.IsNotExist(err) {
		// Fallback
		matches, _ := filepath.Glob("/sys/class/power_supply/BAT*")
		if len(matches) > 0 {
			batDir = matches[0]
		} else {
			return BatteryInfo{Percent: 100, IsPlugged: true, Status: "Full", HealthPct: 100}
		}
	}

	readInt := func(name string) int64 {
		b, err := os.ReadFile(filepath.Join(batDir, name))
		if err != nil {
			return 0
		}
		val, _ := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
		return val
	}

	readString := func(name string) string {
		b, err := os.ReadFile(filepath.Join(batDir, name))
		if err != nil {
			return "Unknown"
		}
		return strings.TrimSpace(string(b))
	}

	capacity := float64(readInt("capacity"))
	status := readString("status")
	isPlugged := status != "Discharging"

	volU := float64(readInt("voltage_now"))
	currU := float64(readInt("current_now"))
	powerU := float64(readInt("power_now"))

	watts := 0.0
	if volU > 0 && currU > 0 {
		watts = (volU / 1e6) * (currU / 1e6)
	} else if powerU > 0 {
		watts = powerU / 1e6
	}
	watts = math.Round(watts*100) / 100

	cf := float64(readInt("charge_full"))
	if cf == 0 {
		cf = float64(readInt("energy_full"))
	}
	cfd := float64(readInt("charge_full_design"))
	if cfd == 0 {
		cfd = float64(readInt("energy_full_design"))
	}

	health := 100.0
	if cfd > 0 && cf > 0 {
		health = math.Round((cf/cfd)*1000) / 10
	}
	wear := math.Max(0, math.Round((100.0-health)*10)/10)
	cycles := int(readInt("cycle_count"))

	var tips []string
	if isPlugged && capacity > 90 && health < 80 {
		tips = append(tips, "Рекомендуется включить лимит заряда до 80% для защиты батареи.")
	}
	if !isPlugged && watts > 25.0 {
		tips = append(tips, fmt.Sprintf("Высокий расход (%.1fW) на батарее! Проверьте dev-процессы.", watts))
	}

	return BatteryInfo{
		Percent:    capacity,
		IsPlugged:  isPlugged,
		Status:     status,
		Watts:      watts,
		Voltage:    math.Round((volU/1e6)*100) / 100,
		HealthPct:  health,
		WearPct:    wear,
		CycleCount: cycles,
		Tips:       tips,
	}
}

// GetNVMeTemp reads from nvme hwmon in ~0.04ms
func GetNVMeTemp() float64 {
	initHwmon()
	if hwmonNVMe != "" {
		if b, err := os.ReadFile(filepath.Join(hwmonNVMe, "temp1_input")); err == nil {
			if m, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64); err == nil {
				return math.Round(m / 100.0) / 10.0
			}
		}
	}
	return 40.0
}

// GetGPUMode reads prime GPU mode with 30s cache
func GetGPUMode() string {
	if time.Since(lastGPUModeCheck) < 30*time.Second && cachedGPUMode != "unknown" {
		return cachedGPUMode
	}

	// Read /etc/prime-discrete or dpkg alternative config
	mode := "on-demand"
	if b, err := os.ReadFile("/etc/prime-discrete"); err == nil {
		mode = strings.TrimSpace(string(b))
	} else if b, err := os.ReadFile("/var/lib/ubuntu-drivers-common/prime-select"); err == nil {
		mode = strings.TrimSpace(string(b))
	}

	cachedGPUMode = mode
	lastGPUModeCheck = time.Now()
	return cachedGPUMode
}

func GetGPUInfo() GPUInfo {
	mode := GetGPUMode()
	status := "On-Demand (Гибрид: Intel UHD + RTX 3050 Ti)"
	if mode == "intel" {
		status = "Intel Only (NVIDIA выключена)"
	} else if mode == "nvidia" {
		status = "NVIDIA Only (Макс. производительность)"
	}

	return GPUInfo{
		Mode:   mode,
		Status: status,
		Temp:   0.0,
	}
}

// GetSystemTelemetry aggregates all sensor data in ~0.5ms
func GetSystemTelemetry() SystemTelemetry {
	cpu := GetCPUInfo()
	ram := GetRAMInfo()
	bat := GetBatteryInfo()
	nvme := GetNVMeTemp()
	gpu := GetGPUInfo()

	status := "NOMINAL"
	statusText := "Все системы в норме"
	statusColor := "emerald"

	if cpu.IsCritical || ram.Pressure == "CRITICAL" {
		status = "CRITICAL"
		statusText = "Внимание: Высокая нагрузка или перегрев!"
		statusColor = "crimson"
	} else if cpu.IsHot || ram.Pressure == "WARNING" || (!bat.IsPlugged && bat.Watts > 25.0) {
		status = "WARNING"
		statusText = "Предупреждение: Проверьте ресурсы"
		statusColor = "amber"
	}

	var uptimeSec int64
	if b, err := os.ReadFile("/proc/uptime"); err == nil {
		parts := strings.Fields(string(b))
		if len(parts) > 0 {
			f, _ := strconv.ParseFloat(parts[0], 64)
			uptimeSec = time.Now().Unix() - int64(f)
		}
	}

	return SystemTelemetry{
		Status:      status,
		StatusText:  statusText,
		StatusColor: statusColor,
		CPU:         cpu,
		RAM:         ram,
		Battery:     bat,
		NVMeTemp:    nvme,
		GPU:         gpu,
		UptimeSec:   uptimeSec,
	}
}
