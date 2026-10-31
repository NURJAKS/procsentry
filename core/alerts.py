"""Smart alert engine using desktop notifications (notify-send) with cooldowns.
Protects the laptop from thermal damage and memory freezes without notification spam.
"""

import time
import subprocess
import shutil
from typing import Dict, Any

COOLDOWNS: Dict[str, float] = {}


def send_notification(title: str, message: str, urgency: str = "normal", icon: str = "utilities-system-monitor") -> bool:
    """Sends a desktop notification using notify-send."""
    if not shutil.which("notify-send"):
        return False
    try:
        subprocess.run(
            ["notify-send", "-a", "Dev Laptop Sentinel", "-u", urgency, "-i", icon, title, message],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            timeout=2
        )
        return True
    except Exception:
        return False


def check_and_alert(telemetry: Dict[str, Any]) -> None:
    """Evaluates telemetry and sends desktop notifications if thresholds are violated."""
    now = time.time()
    
    cpu = telemetry.get("cpu", {})
    ram = telemetry.get("ram", {})
    battery = telemetry.get("battery", {})

    # 1. Thermal Overheat Warning (> 85°C)
    pkg_temp = cpu.get("package_temp", 50.0)
    if pkg_temp >= 86.0:
        last = COOLDOWNS.get("temp", 0)
        if now - last > 300:  # 5 min cooldown
            send_notification(
                "🔥 Внимание: Перегрев процессора!",
                f"Температура CPU достигла {pkg_temp}°C. Проверьте вентиляцию или тяжелые dev-процессы.",
                urgency="critical",
                icon="dialog-warning"
            )
            COOLDOWNS["temp"] = now

    # 2. Critical RAM Pressure (< 1.2 GB available)
    avail_gb = ram.get("available_gb", 5.0)
    if avail_gb < 1.2:
        last = COOLDOWNS.get("ram", 0)
        if now - last > 300:  # 5 min cooldown
            send_notification(
                "💾 Опасно мало свободной ОЗУ!",
                f"Осталось всего {avail_gb} GB ОЗУ! Возможен глухой завис системы. Освободите память в Sentinel.",
                urgency="critical",
                icon="dialog-error"
            )
            COOLDOWNS["ram"] = now

    # 3. High Battery Discharge Rate (> 28W while on battery)
    if not battery.get("is_plugged", True):
        watts = battery.get("watts", 0.0)
        if watts > 28.0:
            last = COOLDOWNS.get("battery", 0)
            if now - last > 600:  # 10 min cooldown
                send_notification(
                    "⚡ Высокий разряд батареи!",
                    f"Ноутбук разряжается со скоростью {watts}W. Закройте неиспользуемые dev-серверы.",
                    urgency="normal",
                    icon="battery-caution"
                )
                COOLDOWNS["battery"] = now


def send_welcome_alert(telemetry: Dict[str, Any]) -> None:
    """Sends a boot greeting summary."""
    cpu_t = telemetry.get("cpu", {}).get("package_temp", 50)
    ram_u = telemetry.get("ram", {}).get("used_gb", 9)
    ram_t = telemetry.get("ram", {}).get("total_gb", 15)
    bat_p = telemetry.get("battery", {}).get("percent", 60)
    
    send_notification(
        "🛡️ Dev Sentinel активен",
        f"Система в норме | CPU {cpu_t}°C | RAM {ram_u}/{ram_t} GB | Батарея {bat_p}%",
        urgency="low",
        icon="utilities-system-monitor"
    )
