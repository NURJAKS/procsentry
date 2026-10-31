#!/usr/bin/env python3
"""Main Entrypoint for Dev Laptop Sentinel.
Runs FastAPI web service, background thermal/RAM watchdog, and Ubuntu GNOME tray indicator.
"""

import os
import sys
import time
import threading
import argparse
import webbrowser
import uvicorn

# Add current folder to sys.path
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from core.hardware import get_system_telemetry
from core.alerts import check_and_alert, send_welcome_alert
from tray.tray_indicator import SentinelTray
from web.server import app

PORT = 48888


def background_watchdog():
    """Monitors telemetry in background and fires alerts if thresholds are breached."""
    # Wait 2 seconds for boot stability
    time.sleep(2)
    try:
        initial = get_system_telemetry()
        send_welcome_alert(initial)
    except Exception as e:
        print(f"[Watchdog] Initial alert error: {e}")

    while True:
        try:
            telemetry = get_system_telemetry()
            check_and_alert(telemetry)
        except Exception as e:
            print(f"[Watchdog] Loop error: {e}")
        time.sleep(4)


def run_web_server(port: int):
    """Runs FastAPI uvicorn server."""
    uvicorn.run(app, host="127.0.0.1", port=port, log_level="warning")


def main():
    parser = argparse.ArgumentParser(description="Dev Laptop Sentinel - Hardware & Dev Environment Monitor")
    parser.add_argument("--port", type=int, default=PORT, help=f"Web dashboard port (default: {PORT})")
    parser.add_argument("--no-tray", action="store_true", help="Run in headless mode without system tray")
    parser.add_argument("--open", action="store_true", help="Automatically open web dashboard in browser")
    args = parser.parse_args()

    print(f"==================================================")
    print(f"🛡️  Dev Laptop Sentinel starting on port {args.port}")
    print(f"    Dashboard: http://localhost:{args.port}")
    print(f"    Laptop: Acer Nitro 5 (AN515-57) • Ubuntu Linux")
    print(f"==================================================")

    # 1. Start background alert watchdog thread
    watchdog_thread = threading.Thread(target=background_watchdog, daemon=True)
    watchdog_thread.start()

    # 2. Start Web server thread
    web_thread = threading.Thread(target=run_web_server, args=(args.port,), daemon=True)
    web_thread.start()

    # Optional open browser
    if args.open:
        def open_browser():
            time.sleep(1)
            webbrowser.open(f"http://localhost:{args.port}")
        threading.Thread(target=open_browser, daemon=True).start()

    # 3. Main thread runs Tray or stays alive
    if not args.no_tray and (os.environ.get("DISPLAY") or os.environ.get("WAYLAND_DISPLAY")):
        tray = SentinelTray(port=args.port)
        tray.start()
    else:
        print("[Sentinel] Running without tray. Press Ctrl+C to stop.")
        try:
            while True:
                time.sleep(1)
        except KeyboardInterrupt:
            print("\n[Sentinel] Stopped.")


if __name__ == "__main__":
    main()
