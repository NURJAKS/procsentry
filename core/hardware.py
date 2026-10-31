"""Hardware telemetry collector for Acer Nitro 5 / Linux Laptops.
Reads CPU package, core temps, NVMe SSD temp, RAM, Battery health, watts, and power states.
"""

import os
import glob
import psutil
import subprocess
from typing import Dict, Any, List, Optional


def get_cpu_info() -> Dict[str, Any]:
    """Returns CPU load, frequencies, and temperatures."""
    load_per_core = psutil.cpu_percent(interval=None, percpu=True)
    load_overall = psutil.cpu_percent(interval=None)
    freq = psutil.cpu_freq()
    
    # Read core temperatures
    temps = psutil.sensors_temperatures()
    cpu_temps: Dict[str, float] = {}
    package_temp = 50.0
    core_temps: List[Dict[str, Any]] = []
    
    if "coretemp" in temps:
        for entry in temps["coretemp"]:
            label = entry.label or "Core"
            current = round(entry.current, 1) if entry.current is not None else 0.0
            if "Package" in label:
                package_temp = current
            else:
                core_temps.append({
                    "name": label,
                    "temp": current,
                    "high": entry.high,
                    "critical": entry.critical
                })
    elif "acpitz_0" in temps and temps["acpitz_0"]:
        package_temp = round(temps["acpitz_0"][0].current or 50.0, 1)

    # Throttling status check
    is_hot = package_temp >= 85.0
    is_critical = package_temp >= 92.0

    return {
        "load_percent": round(load_overall, 1),
        "load_per_core": [round(x, 1) for x in load_per_core],
        "core_count": psutil.cpu_count(logical=True),
        "physical_cores": psutil.cpu_count(logical=False),
        "package_temp": package_temp,
        "cores": core_temps,
        "is_hot": is_hot,
        "is_critical": is_critical,
        "current_freq_mhz": round(freq.current, 0) if freq else None,
        "max_freq_mhz": round(freq.max, 0) if freq else None,
    }


def get_nvme_temp() -> Optional[float]:
    """Returns NVMe SSD temperature."""
    temps = psutil.sensors_temperatures()
    if "nvme" in temps:
        for entry in temps["nvme"]:
            if entry.current is not None:
                return round(entry.current, 1)
    return None


def get_ram_info() -> Dict[str, Any]:
    """Returns RAM and Swap usage in GB and percentages."""
    vmem = psutil.virtual_memory()
    swap = psutil.swap_memory()
    
    total_gb = round(vmem.total / (1024 ** 3), 2)
    used_gb = round(vmem.used / (1024 ** 3), 2)
    available_gb = round(vmem.available / (1024 ** 3), 2)
    cached_gb = round((getattr(vmem, "cached", 0) + getattr(vmem, "buffers", 0)) / (1024 ** 3), 2)
    
    # RAM pressure level
    if available_gb < 1.2:
        pressure = "CRITICAL"
    elif available_gb < 2.5:
        pressure = "WARNING"
    else:
        pressure = "HEALTHY"
        
    return {
        "total_gb": total_gb,
        "used_gb": used_gb,
        "available_gb": available_gb,
        "cached_gb": cached_gb,
        "percent": round(vmem.percent, 1),
        "pressure": pressure,
        "swap_total_gb": round(swap.total / (1024 ** 3), 2),
        "swap_used_gb": round(swap.used / (1024 ** 3), 2),
        "swap_percent": round(swap.percent, 1)
    }


def get_battery_info() -> Dict[str, Any]:
    """Returns battery percentage, status, power draw in Watts, and wear level."""
    bat_base = "/sys/class/power_supply/BAT1"
    if not os.path.exists(bat_base):
        # Fallback to any battery
        bats = glob.glob("/sys/class/power_supply/BAT*")
        bat_base = bats[0] if bats else None

    ps_bat = psutil.sensors_battery()
    percent = round(ps_bat.percent, 1) if ps_bat else 0.0
    plugged = bool(ps_bat.power_plugged) if (ps_bat and ps_bat.power_plugged is not None) else True

    status = "Unknown"
    watts = 0.0
    voltage_v = 0.0
    current_a = 0.0
    health_pct = 100.0
    cycle_count = 0
    charge_full_mah = 0
    charge_design_mah = 0

    if bat_base and os.path.exists(bat_base):
        try:
            with open(os.path.join(bat_base, "status")) as f:
                status = f.read().strip()
                if ps_bat is None or ps_bat.power_plugged is None:
                    plugged = status != "Discharging"
        except Exception:
            status = "Charging" if plugged else "Discharging"

        # Read Voltage and Current for Wattage
        try:
            with open(os.path.join(bat_base, "voltage_now")) as f:
                voltage_v = int(f.read().strip()) / 1e6
        except Exception:
            pass

        try:
            with open(os.path.join(bat_base, "current_now")) as f:
                current_a = int(f.read().strip()) / 1e6
        except Exception:
            pass

        if voltage_v > 0 and current_a > 0:
            watts = round(voltage_v * current_a, 2)
        else:
            # Check power_now
            try:
                with open(os.path.join(bat_base, "power_now")) as f:
                    watts = round(int(f.read().strip()) / 1e6, 2)
            except Exception:
                pass

        # Health & Design calculation
        try:
            cf_path = os.path.join(bat_base, "charge_full")
            if not os.path.exists(cf_path):
                cf_path = os.path.join(bat_base, "energy_full")
            cfd_path = os.path.join(bat_base, "charge_full_design")
            if not os.path.exists(cfd_path):
                cfd_path = os.path.join(bat_base, "energy_full_design")

            with open(cf_path) as f1, open(cfd_path) as f2:
                cf = int(f1.read().strip())
                cfd = int(f2.read().strip())
                health_pct = round((cf / cfd) * 100, 1)
                charge_full_mah = round(cf / 1000, 0)
                charge_design_mah = round(cfd / 1000, 0)
        except Exception:
            pass

        try:
            with open(os.path.join(bat_base, "cycle_count")) as f:
                cycle_count = int(f.read().strip())
        except Exception:
            pass

    # Battery recommendations
    tips = []
    if plugged and percent > 90 and health_pct < 80:
        tips.append("Рекомендуется включить лимит заряда до 80% (Battery Care), чтобы остановить деградацию батареи.")
    if not plugged and watts > 25.0:
        tips.append(f"Высокое потребление ({watts}W) на батарее! Проверьте фоновые dev-серверы.")

    return {
        "percent": percent,
        "is_plugged": plugged,
        "status": status,
        "watts": watts,
        "voltage": round(voltage_v, 2),
        "health_pct": health_pct,
        "wear_pct": round(max(0.0, 100.0 - health_pct), 1),
        "cycle_count": cycle_count,
        "charge_full_mah": charge_full_mah,
        "charge_design_mah": charge_design_mah,
        "tips": tips
    }


def get_gpu_mode() -> str:
    """Queries current PRIME GPU mode (intel, on-demand, nvidia)."""
    try:
        res = subprocess.run(
            ["prime-select", "query"],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            timeout=2
        )
        if res.returncode == 0 and res.stdout.strip():
            return res.stdout.strip().lower()
    except Exception:
        pass
    return "unknown"


def switch_gpu_mode(target_mode: str) -> Dict[str, Any]:
    """Attempts to switch GPU mode via pkexec or returns exact terminal command."""
    if target_mode not in ("intel", "on-demand", "nvidia"):
        return {"success": False, "error": f"Неизвестный режим: {target_mode}. Доступно: intel, on-demand, nvidia"}
    
    current = get_gpu_mode()
    if current == target_mode:
        return {"success": True, "message": f"Режим {target_mode} уже активен", "mode": current}

    # Attempt GUI authorization via pkexec
    try:
        res = subprocess.run(
            ["pkexec", "prime-select", target_mode],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            timeout=15
        )
        if res.returncode == 0:
            return {
                "success": True,
                "message": f"Режим успешно изменен на {target_mode}! Перезагрузите ноутбук для применения драйвера.",
                "requires_reboot": True,
                "mode": target_mode
            }
    except Exception:
        pass

    return {
        "success": False,
        "requires_sudo": True,
        "command": f"sudo prime-select {target_mode} && sudo reboot",
        "message": f"Для смены режима видеокарты требуются права root. Выполните команду в терминале:\nsudo prime-select {target_mode} && sudo reboot"
    }


def get_gpu_info() -> Dict[str, Any]:
    """Returns GPU model, active PRIME mode, power state, and temperature."""
    mode = get_gpu_mode()
    
    mode_labels = {
        "intel": "Intel Only (NVIDIA выключена)",
        "on-demand": "On-Demand (Гибрид: Intel + RTX 3050 Ti)",
        "nvidia": "NVIDIA Only (Макс. мощность)",
        "unknown": "Не определен"
    }

    info = {
        "name": "Intel TigerLake GT1 UHD + NVIDIA RTX 3050 Ti Mobile",
        "has_nvidia": True,
        "mode": mode,
        "mode_label": mode_labels.get(mode, mode),
        "recommended_mode": "on-demand",
        "is_optimal": (mode == "on-demand"),
        "temp": None,
        "power_draw": None,
        "status": mode_labels.get(mode, "Hybrid Mode")
    }

    # If NVIDIA is powered (on-demand or nvidia), query temperature and telemetry
    if mode in ("on-demand", "nvidia"):
        try:
            res = subprocess.run(
                ["nvidia-smi", "--query-gpu=temperature.gpu,power.draw,utilization.gpu", "--format=csv,noheader,nounits"],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                timeout=1
            )
            if res.returncode == 0 and res.stdout.strip():
                parts = [p.strip() for p in res.stdout.strip().split(",")]
                info["temp"] = float(parts[0])
                info["power_draw"] = f"{parts[1]}W"
                info["status"] = f"RTX 3050 Ti: {parts[0]}°C | {parts[1]}W"
        except Exception:
            pass
    else:
        info["status"] = "NVIDIA обесточена (Только Intel iGPU)"

    return info


def get_system_telemetry() -> Dict[str, Any]:
    """Combines all hardware metrics into a single snapshot."""
    cpu = get_cpu_info()
    ram = get_ram_info()
    bat = get_battery_info()
    nvme_temp = get_nvme_temp()
    gpu = get_gpu_info()
    
    # Calculate overall health status
    status = "NOMINAL"
    status_text = "Все системы в норме"
    status_color = "emerald"

    if cpu["is_critical"] or ram["pressure"] == "CRITICAL":
        status = "CRITICAL"
        status_text = "Внимание: Высокая нагрузка или перегрев!"
        status_color = "crimson"
    elif cpu["is_hot"] or ram["pressure"] == "WARNING" or (not bat["is_plugged"] and bat["watts"] > 25.0):
        status = "WARNING"
        status_text = "Предупреждение: Проверьте ресурсы"
        status_color = "amber"

    return {
        "status": status,
        "status_text": status_text,
        "status_color": status_color,
        "cpu": cpu,
        "nvme_temp": nvme_temp,
        "ram": ram,
        "battery": bat,
        "gpu": gpu,
        "uptime": round(psutil.boot_time(), 0)
    }
