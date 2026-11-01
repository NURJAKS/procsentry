"""Ubuntu Top-Panel Tray Indicator for DevAegis (Dev Sentinel).
Uses AyatanaAppIndicator3 to display live CPU temperature and RAM in the GNOME panel.
"""

import os
import sys
import json
import logging
import subprocess
import urllib.request
import webbrowser

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


class SentinelTray:
    def __init__(self, port: int = 48888):
        self.port = port
        self.dashboard_url = f"http://localhost:{port}"
        self.indicator = None
        self.menu = None
        self.item_status = None
        self.item_cpu = None
        self.item_ram = None
        self.item_bat = None
        self.item_docker = None

    def start(self):
        if not GTK_AVAILABLE:
            print("[Tray] GTK/AyatanaAppIndicator3 not available, skipping tray.")
            return

        if not os.environ.get("DISPLAY") and not os.environ.get("WAYLAND_DISPLAY"):
            print("[Tray] No display detected, skipping tray icon.")
            return

        try:
            self.indicator = AyatanaAppIndicator3.Indicator.new(
                "dev_aegis_sentinel",
                "utilities-system-monitor",
                AyatanaAppIndicator3.IndicatorCategory.APPLICATION_STATUS
            )
            self.indicator.set_status(AyatanaAppIndicator3.IndicatorStatus.ACTIVE)
            self.indicator.set_title("DevAegis Sentinel")
            self.indicator.set_label(" 55°C | 10G", " 99°C | 99.9G")

            self.menu = Gtk.Menu()

            # Status header
            self.item_status = Gtk.MenuItem(label="🛡️ DevAegis: Все системы в норме")
            self.item_status.set_sensitive(False)
            self.menu.append(self.item_status)

            self.menu.append(Gtk.SeparatorMenuItem())

            # 1. Quick Launchers
            item_open = Gtk.MenuItem(label="🌐 Открыть Веб-дашборд (Ctrl+Alt+S)")
            item_open.connect("activate", self.on_open_dashboard)
            self.menu.append(item_open)

            item_tui = Gtk.MenuItem(label="⚡ Экстренный TUI HUD (Ctrl+Alt+E)")
            item_tui.connect("activate", self.on_open_tui)
            self.menu.append(item_tui)

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
            item_ghosts = Gtk.MenuItem(label="🧹 Убить dev-призраков (Ctrl+Alt+G)")
            item_ghosts.connect("activate", self.on_kill_ghosts)
            self.menu.append(item_ghosts)

            item_docker_clean = Gtk.MenuItem(label="🐳 Очистить неиспользуемый Docker")
            item_docker_clean.connect("activate", self.on_prune_docker)
            self.menu.append(item_docker_clean)

            self.menu.append(Gtk.SeparatorMenuItem())

            # 4. Quit
            item_quit = Gtk.MenuItem(label="❌ Закрыть иконку трея")
            item_quit.connect("activate", self.on_quit)
            self.menu.append(item_quit)

            self.menu.show_all()
            self.indicator.set_menu(self.menu)

            # Periodic update every 2.5s
            GLib.timeout_add_seconds(3, self.update_telemetry)
            self.update_telemetry()

            print("[Tray] AyatanaAppIndicator active in GNOME top panel.")
            Gtk.main()
        except Exception as e:
            logger.error("Error initializing tray: %s", e)
            print(f"[Tray] Error initializing tray: {e}")

    def fetch_api(self, endpoint: str):
        try:
            url = f"http://127.0.0.1:{self.port}{endpoint}"
            req = urllib.request.Request(url, headers={"User-Agent": "DevAegis-Tray"})
            with urllib.request.urlopen(req, timeout=1.5) as resp:
                return json.loads(resp.read().decode("utf-8"))
        except Exception:
            return None

    def post_api(self, endpoint: str):
        try:
            url = f"http://127.0.0.1:{self.port}{endpoint}"
            req = urllib.request.Request(url, data=b"{}", headers={"Content-Type": "application/json", "User-Agent": "DevAegis-Tray"})
            with urllib.request.urlopen(req, timeout=3.0) as resp:
                return json.loads(resp.read().decode("utf-8"))
        except Exception:
            return None

    def update_telemetry(self):
        try:
            data = self.fetch_api("/api/telemetry")
            if not data:
                # Fallback to local python modules if server not reached
                try:
                    from core.hardware import get_system_telemetry
                    data = get_system_telemetry()
                except Exception:
                    data = {}

            cpu_info = data.get("cpu", {})
            cpu_t = cpu_info.get("package_temp", 0)
            cpu_load = cpu_info.get("load_percent", 0)
            
            ram_info = data.get("ram", {})
            ram_u = ram_info.get("used_gb", 0)
            ram_t = ram_info.get("total_gb", 15.4)
            ram_press = ram_info.get("pressure", "HEALTHY")

            bat_info = data.get("battery", {})
            bat_p = bat_info.get("percent", 0)
            bat_w = bat_info.get("watts", 0)
            is_plugged = bat_info.get("is_plugged", False)

            # Update top panel label
            label_text = f" {cpu_t:.0f}°C | {ram_u:.1f}G"
            self.indicator.set_label(label_text, " 99°C | 99.9G")

            # Update menu items
            if self.item_status:
                status_text = data.get("status_text", "Все системы в норме")
                status = data.get("status", "NOMINAL")
                icon = "🛡️" if status == "NOMINAL" else ("⚠️" if status == "WARNING" else "🔥")
                self.item_status.set_label(f"{icon} DevAegis: {status_text}")

            if self.item_cpu:
                self.item_cpu.set_label(f"🔥 CPU: {cpu_t:.1f}°C (Загрузка: {cpu_load:.1f}%)")
            if self.item_ram:
                self.item_ram.set_label(f"💾 RAM: {ram_u:.1f} / {ram_t:.1f} GB [{ram_press}]")
            if self.item_bat:
                plugged = "Зарядка" if is_plugged else "Батарея"
                self.item_bat.set_label(f"🔋 Батарея: {bat_p:.0f}% ({bat_w:.1f}W, {plugged})")

            if self.item_docker:
                docker_data = self.fetch_api("/api/docker") or {}
                r_count = docker_data.get("running_count", 0)
                self.item_docker.set_label(f"🐳 Docker: {r_count} активных контейнеров")
        except Exception as e:
            logger.warning("Tray update error: %s", e)
        return True

    def on_open_dashboard(self, _widget):
        webbrowser.open(self.dashboard_url)

    def on_open_tui(self, _widget):
        try:
            subprocess.Popen(["gnome-terminal", "--title=DevAegis Emergency HUD", "--", "aegis", "tui"])
        except Exception as e:
            logger.error("Failed to open TUI: %s", e)

    def on_kill_ghosts(self, _widget):
        res = self.post_api("/api/processes/kill_ghosts")
        msg = "Все dev-призраки завершены!"
        if res and "freed_mb" in res:
            msg = f"Завершено {res.get('killed', 0)} процессов, освобождено {res.get('freed_mb', 0):.1f} MB"
        try:
            subprocess.run(["notify-send", "-a", "DevAegis", "🧹 Очистка призраков", msg])
        except Exception:
            pass

    def on_prune_docker(self, _widget):
        self.post_api("/api/docker/prune")
        try:
            subprocess.run(["notify-send", "-a", "DevAegis", "🐳 Docker Prune", "Остановленные контейнеры удалены"])
        except Exception:
            pass

    def on_quit(self, _widget):
        Gtk.main_quit()
        sys.exit(0)


if __name__ == "__main__":
    port = 48888
    if len(sys.argv) > 1:
        try:
            port = int(sys.argv[1])
        except ValueError:
            pass
    tray = SentinelTray(port=port)
    tray.start()
