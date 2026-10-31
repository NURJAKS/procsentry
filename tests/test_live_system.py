"""Automated Live System Integration Test Suite for Dev Laptop Sentinel.
NO MOCKS: Tests against real laptop hardware sensors, Linux kernel interfaces,
psutil processes, listening sockets, Docker daemon, and live REST API endpoints.
"""

import os
import sys
import time
import socket
import json
import urllib.request
import urllib.error
import unittest
from typing import Dict, Any, List

# Ensure parent directory is in sys.path
BASE_DIR = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
if BASE_DIR not in sys.path:
    sys.path.insert(0, BASE_DIR)

from core.hardware import (
    get_cpu_info,
    get_nvme_temp,
    get_ram_info,
    get_battery_info,
    get_gpu_info,
    get_system_telemetry
)
from core.processes import get_dev_processes, is_dev_process, kill_process
from core.ports import get_listening_ports, free_port
from core.docker_cleaner import is_docker_available, get_docker_telemetry
from core.disk_cleaner import scan_caches
from core.alerts import check_and_alert


class TestRealHardwareSensors(unittest.TestCase):
    """Verifies actual hardware telemetry on the Acer Nitro 5 laptop."""

    def test_cpu_sensor_real_values(self) -> None:
        cpu = get_cpu_info()
        self.assertIsInstance(cpu, dict)
        # Check core counts on Intel i5-11400H
        self.assertEqual(cpu["physical_cores"], 6, "Expected 6 physical cores on i5-11400H")
        self.assertEqual(cpu["core_count"], 12, "Expected 12 logical threads on i5-11400H")
        
        # Check temperature range (must be plausible room-to-load range)
        temp = cpu["package_temp"]
        self.assertGreater(temp, 25.0, "CPU package temperature suspiciously low")
        self.assertLess(temp, 105.0, "CPU package temperature above TjMax")
        
        # Check load percentage
        self.assertGreaterEqual(cpu["load_percent"], 0.0)
        self.assertLessEqual(cpu["load_percent"], 100.0)
        self.assertEqual(len(cpu["load_per_core"]), 12)

    def test_nvme_real_sensor(self) -> None:
        nvme_t = get_nvme_temp()
        if nvme_t is not None:
            self.assertGreater(nvme_t, 20.0, "NVMe temp suspiciously low")
            self.assertLess(nvme_t, 85.0, "NVMe temp exceeds critical threshold")

    def test_ram_real_kernel_metrics(self) -> None:
        ram = get_ram_info()
        self.assertGreater(ram["total_gb"], 14.0, "Expected ~15-16 GB total RAM")
        self.assertGreater(ram["used_gb"], 0.0)
        self.assertGreater(ram["available_gb"], 0.0)
        self.assertIn(ram["pressure"], ["HEALTHY", "WARNING", "CRITICAL"])
        self.assertGreaterEqual(ram["percent"], 0.0)
        self.assertLessEqual(ram["percent"], 100.0)

    def test_battery_real_metrics(self) -> None:
        bat = get_battery_info()
        self.assertGreaterEqual(bat["percent"], 0.0)
        self.assertLessEqual(bat["percent"], 100.0)
        self.assertIsInstance(bat["is_plugged"], bool)
        self.assertGreater(bat["health_pct"], 40.0, "Battery health reported too low or invalid")
        self.assertGreaterEqual(bat["cycle_count"], 0)
        self.assertGreaterEqual(bat["watts"], 0.0)

    def test_gpu_telemetry(self) -> None:
        gpu = get_gpu_info()
        self.assertTrue(gpu["has_nvidia"])
        self.assertIn("RTX 3050 Ti", gpu["name"])

    def test_system_telemetry_aggregation(self) -> None:
        telemetry = get_system_telemetry()
        self.assertIn(telemetry["status"], ["NOMINAL", "WARNING", "CRITICAL"])
        self.assertIn("cpu", telemetry)
        self.assertIn("ram", telemetry)
        self.assertIn("battery", telemetry)
        self.assertGreater(telemetry["uptime"], 0)


class TestRealProcessHunter(unittest.TestCase):
    """Verifies dev process scanner against real running processes on the machine."""

    def test_process_detection(self) -> None:
        procs = get_dev_processes()
        self.assertIsInstance(procs, list)
        for p in procs:
            self.assertIn("pid", p)
            self.assertIn("name", p)
            self.assertIn("memory_mb", p)
            self.assertIn("cpu_percent", p)
            self.assertIn("uptime_hours", p)
            self.assertIn("is_ghost", p)
            self.assertGreaterEqual(p["memory_mb"], 0.0)

    def test_category_matching(self) -> None:
        self.assertEqual(is_dev_process("python3", "python -m uvicorn app:app"), "python")
        self.assertEqual(is_dev_process("node", "node /app/node_modules/vite/bin/vite.js"), "node")
        self.assertEqual(is_dev_process("postgres", "postgres -D /data"), "database")
        self.assertEqual(is_dev_process("cargo", "cargo run --release"), "compiler")
        # System exclusions
        self.assertIsNone(is_dev_process("gnome-terminal", "gnome-terminal"))
        self.assertIsNone(is_dev_process("chrome", "google-chrome"))

    def test_kill_nonexistent_process_handles_gracefully(self) -> None:
        # Testing non-existent PID 9999999
        res = kill_process(9999999)
        self.assertTrue(res.get("success"), "Should gracefully handle already terminated process")

    def test_ghost_exclusions_and_service_detection(self) -> None:
        from core.processes import GHOST_EXCLUSIONS, is_system_managed_service
        self.assertIn("tracker_bot", GHOST_EXCLUSIONS)
        # Test that any process matching tracker_bot is not flagged as ghost
        procs = get_dev_processes()
        for p in procs:
            if "tracker_bot" in p["cmdline"]:
                self.assertFalse(p["is_ghost"], "tracker_bot must NOT be flagged as ghost")
                self.assertTrue(p["is_service"], "tracker_bot should be flagged as service")


class TestRealPortConflictRadar(unittest.TestCase):
    """Verifies live port detection by binding a real test socket on loopback."""

    def test_live_socket_detection(self) -> None:
        test_port = 48899
        sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        try:
            sock.bind(("127.0.0.1", test_port))
            sock.listen(1)

            # Query listening ports
            ports = get_listening_ports()
            target = next((p for p in ports if p["port"] == test_port), None)
            
            self.assertIsNotNone(target, f"Port {test_port} should be detected in listening ports")
            if target:
                self.assertEqual(target["port"], test_port)
                self.assertEqual(target["ip"], "127.0.0.1")
                self.assertTrue(target["is_dev_port"])
        finally:
            sock.close()


class TestRealDockerInspection(unittest.TestCase):
    """Verifies communication with Docker daemon without mocks."""

    def test_docker_telemetry_live(self) -> None:
        available = is_docker_available()
        self.assertTrue(available, "Docker daemon should be available on this dev laptop")
        
        data = get_docker_telemetry()
        self.assertTrue(data["available"])
        self.assertIn("containers", data)
        self.assertIn("disk", data)
        # Verify containers were parsed
        self.assertIsInstance(data["containers"], list)
        # Check disk space stats format
        self.assertIn("images_reclaimable", data["disk"])


class TestRealCacheScanner(unittest.TestCase):
    """Verifies that developer caches on NVMe are correctly located and measured."""

    def test_scan_caches(self) -> None:
        caches = scan_caches()
        self.assertIsInstance(caches, list)
        self.assertGreater(len(caches), 0, "Expected at least one dev cache on machine")
        for c in caches:
            self.assertIn("name", c)
            self.assertIn("path", c)
            self.assertIn("size_mb", c)
            self.assertGreaterEqual(c["size_mb"], 0.0)


class TestRealAlertEngine(unittest.TestCase):
    """Verifies alert evaluation with current live telemetry."""

    def test_alert_check_executes_safely(self) -> None:
        telemetry = get_system_telemetry()
        # Should execute without throwing any exception
        check_and_alert(telemetry)


class TestRealFastAPILiveEndpoints(unittest.TestCase):
    """Verifies live REST API endpoints on the running server (http://127.0.0.1:48888)."""

    BASE_URL = "http://127.0.0.1:48888"

    def _get_json(self, path: str) -> Dict[str, Any]:
        req = urllib.request.Request(f"{self.BASE_URL}{path}")
        with urllib.request.urlopen(req, timeout=3) as res:
            self.assertEqual(res.status, 200)
            return json.loads(res.read().decode())

    def test_endpoint_telemetry(self) -> None:
        data = self._get_json("/api/telemetry")
        self.assertIn("status", data)
        self.assertIn("cpu", data)
        self.assertIn("ram", data)
        self.assertIn("battery", data)

    def test_endpoint_processes(self) -> None:
        procs = self._get_json("/api/processes")
        self.assertIsInstance(procs, list)

    def test_endpoint_ports(self) -> None:
        ports = self._get_json("/api/ports")
        self.assertIsInstance(ports, list)
        # At minimum port 48888 should be in the list
        our_port = next((p for p in ports if p["port"] == 48888), None)
        self.assertIsNotNone(our_port, "Dev Sentinel's own port 48888 should be detected")

    def test_endpoint_docker(self) -> None:
        docker = self._get_json("/api/docker")
        self.assertTrue(docker.get("available"))

    def test_endpoint_caches(self) -> None:
        caches = self._get_json("/api/caches")
        self.assertIsInstance(caches, list)

    def test_endpoint_html_and_assets(self) -> None:
        # Verify index.html loads with 200 OK
        with urllib.request.urlopen(f"{self.BASE_URL}/", timeout=3) as res:
            self.assertEqual(res.status, 200)
            content = res.read().decode()
            self.assertIn("SENTINEL", content)
            self.assertIn("NITRO 5", content)

        # Verify static CSS
        with urllib.request.urlopen(f"{self.BASE_URL}/static/style.css", timeout=3) as res:
            self.assertEqual(res.status, 200)

        # Verify static JS
        with urllib.request.urlopen(f"{self.BASE_URL}/static/app.js", timeout=3) as res:
            self.assertEqual(res.status, 200)


class TestAutostartIntegrity(unittest.TestCase):
    """Verifies autostart scripts exist and are executable."""

    def test_scripts_executable(self) -> None:
        install_script = os.path.join(BASE_DIR, "install_autostart.sh")
        uninstall_script = os.path.join(BASE_DIR, "uninstall_autostart.sh")
        app_entry = os.path.join(BASE_DIR, "app.py")

        self.assertTrue(os.path.isfile(install_script))
        self.assertTrue(os.access(install_script, os.X_OK))

        self.assertTrue(os.path.isfile(uninstall_script))
        self.assertTrue(os.access(uninstall_script, os.X_OK))

        self.assertTrue(os.path.isfile(app_entry))
        self.assertTrue(os.access(app_entry, os.X_OK))


if __name__ == "__main__":
    unittest.main(verbosity=2)
