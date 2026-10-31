"""Ubuntu Top-Panel Tray Indicator for Dev Laptop Sentinel.
Uses AyatanaAppIndicator3 to display live CPU temperature and RAM in the GNOME panel.
"""

import os
import sys
import webbrowser
import subprocess
import threading
import time
import logging

logger = logging.getLogger(__name__)

try:
    import gi
    gi.require_version('Gtk', '3.0')
    gi.require_version('AyatanaAppIndicator3', '0.1')
    from gi.repository import Gtk, AyatanaAppIndicator3, GLib
    GTK_AVAILABLE = True
except Exception as e:
    logger.warning("GTK or AyatanaAppIndicator3 not available: %s", e)
    GTK_AVAILABLE = False

from core.hardware import get_system_telemetry
from core.processes import kill_all_ghosts
from core.docker_cleaner import prune_docker, get_docker_telemetry

DASHBOARD_URL = "http://localhost:48888"


class SentinelTray:
    def __init__(self, port: int = 48888):
        self.port = port
        self.dashboard_url = f"http://localhost:{port}"
        self.indicator = None
        self.menu = None
        self.item_cpu = None
        self.item_ram = None
        self.item_bat = None
        self.item_docker = None

    def start(self):
        if not GTK_AVAILABLE:
            print("[Tray] GTK/AyatanaAppIndicator3 not available, running in headless daemon mode.")
            return

        # Check if DISPLAY or WAYLAND_DISPLAY is set
        if not os.environ.get("DISPLAY") and not os.environ.get("WAYLAND_DISPLAY"):
            print("[Tray] No display detected, skipping tray icon.")
            return

        try:
            self.indicator = AyatanaAppIndicator3.Indicator.new(
                "dev_laptop_sentinel",
                "utilities-system-monitor",
                AyatanaAppIndicator3.IndicatorCategory.APPLICATION_STATUS
            )
            self.indicator.set_status(AyatanaAppIndicator3.IndicatorStatus.ACTIVE)
            self.indicator.set_title("Dev Laptop Sentinel")
            self.indicator.set_label(" 56°C | 9.8G", " 99°C | 99.9G")

            self.menu = Gtk.Menu()

            # 1. Open Dashboard
            item_open = Gtk.MenuItem(label="🚀 Открыть Sentinel Dashboard (Ctrl+Alt+S)")
            item_open.connect("activate", self.on_open_dashboard)
            self.menu.append(item_open)

            self.menu.append(Gtk.SeparatorMenuItem())

            # 2. Live Info Items
            self.item_cpu = Gtk.MenuItem(label="🔥 CPU: --°C")
            self.item_cpu.set_sensitive(False)
            self.menu.append(self.item_cpu)

            self.item_ram = Gtk.MenuItem(label="💾 RAM: -- GB")
            self.item_ram.set_sensitive(False)
            self.menu.append(self.item_ram)

            self.item_bat = Gtk.MenuItem(label="🔋 Батарея: --%")
            self.item_bat.set_sensitive(False)
            self.menu.append(self.item_bat)

            self.item_docker = Gtk.MenuItem(label="🐳 Docker: --")
            self.item_docker.set_sensitive(False)
            self.menu.append(self.item_docker)

            self.menu.append(Gtk.SeparatorMenuItem())

            # 3. Actions
            item_ghosts = Gtk.MenuItem(label="🧹 Завершить забытые dev-серверы")
            item_ghosts.connect("activate", self.on_kill_ghosts)
            self.menu.append(item_ghosts)

            item_docker_clean = Gtk.MenuItem(label="🐳 Очистить неиспользуемый Docker")
            item_docker_clean.connect("activate", self.on_prune_docker)
            self.menu.append(item_docker_clean)

            self.menu.append(Gtk.SeparatorMenuItem())

            # 4. Quit
            item_quit = Gtk.MenuItem(label="❌ Закрыть Sentinel")
            item_quit.connect("activate", self.on_quit)
            self.menu.append(item_quit)

            self.menu.show_all()
            self.indicator.set_menu(self.menu)

            # Periodic update every 3s
            GLib.timeout_add_seconds(3, self.update_telemetry)
            self.update_telemetry()

            print("[Tray] AyatanaAppIndicator active in top panel.")
            Gtk.main()
        except Exception as e:
            logger.error("Error initializing tray: %s", e)
            print(f"[Tray] Error initializing tray: {e}")

    def update_telemetry(self):
        try:
            data = get_system_telemetry()
            cpu_t = data.get("cpu", {}).get("package_temp", 50)
            ram_u = data.get("ram", {}).get("used_gb", 0)
            ram_t = data.get("ram", {}).get("total_gb", 15)
            bat_p = data.get("battery", {}).get("percent", 0)
            bat_w = data.get("battery", {}).get("watts", 0)

            # Update top panel label
            label_text = f" {cpu_t}°C | {ram_u}G"
            self.indicator.set_label(label_text, " 99°C | 99.9G")

            # Update menu items
            if self.item_cpu:
                self.item_cpu.set_label(f"🔥 CPU: {cpu_t}°C ({data.get('cpu', {}).get('load_percent', 0)}% Load)")
            if self.item_ram:
                self.item_ram.set_label(f"💾 RAM: {ram_u} / {ram_t} GB")
            if self.item_bat:
                plugged = "AC" if data.get("battery", {}).get("is_plugged") else "Bat"
                self.item_bat.set_label(f"🔋 Батарея: {bat_p}% ({bat_w}W, {plugged})")
            
            if self.item_docker:
                docker_data = get_docker_telemetry()
                r_count = docker_data.get("running_count", 0)
                self.item_docker.set_label(f"🐳 Docker: {r_count} активных контейнеров")
        except Exception as e:
            logger.warning("Tray update error: %s", e)
        return True

    def on_open_dashboard(self, _widget):
        webbrowser.open(self.dashboard_url)

    def on_kill_ghosts(self, _widget):
        res = kill_all_ghosts()
        try:
            subprocess.run(["notify-send", "-a", "Dev Sentinel", "🧹 Очистка призраков", res.get("message", "Готово")])
        except Exception:
            pass

    def on_prune_docker(self, _widget):
        res = prune_docker("stopped")
        try:
            subprocess.run(["notify-send", "-a", "Dev Sentinel", "🐳 Docker Prune", "Остановленные контейнеры удалены"])
        except Exception:
            pass

    def on_quit(self, _widget):
        Gtk.main_quit()
        sys.exit(0)
