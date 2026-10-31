"""Docker resource inspector, container manager, and cleanup engine.
Monitors running containers with live CPU/RAM metrics, controls containers, and cleans NVMe space.
"""

import subprocess
import json
import logging
from typing import Dict, Any, List

logger = logging.getLogger(__name__)


def is_docker_available() -> bool:
    """Checks if Docker command and daemon are accessible."""
    try:
        res = subprocess.run(["docker", "info"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=2)
        return res.returncode == 0
    except Exception as e:
        logger.warning("Docker info check failed: %s", e)
        return False


def get_running_container_stats() -> Dict[str, Dict[str, str]]:
    """Fetches real-time CPU% and memory stats for all running containers."""
    stats_map = {}
    try:
        res = subprocess.run(
            ["docker", "stats", "--no-stream", "--format", "{{json .}}"],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            timeout=3
        )
        if res.returncode == 0 and res.stdout.strip():
            for line in res.stdout.strip().splitlines():
                if line:
                    data = json.loads(line)
                    name = data.get("Name", "")
                    cid = data.get("ID", "")
                    stats_data = {
                        "cpu": data.get("CPUPerc", "0%"),
                        "mem": data.get("MemUsage", "0B"),
                        "mem_pct": data.get("MemPerc", "0%"),
                        "net": data.get("NetIO", "0B"),
                        "pids": data.get("PIDs", "0")
                    }
                    if name:
                        stats_map[name] = stats_data
                    if cid:
                        stats_map[cid] = stats_data
    except Exception as e:
        logger.warning("Could not fetch docker stats: %s", e)
    return stats_map


def get_docker_telemetry() -> Dict[str, Any]:
    """Returns all containers (with live CPU/RAM stats for running ones) and reclaimable disk cache."""
    if not is_docker_available():
        return {
            "available": False,
            "containers": [],
            "running_containers": [],
            "stats": {},
            "reclaimable_summary": "Docker daemon недоступен или выключен"
        }

    containers: List[Dict[str, Any]] = []
    stats_map = get_running_container_stats()

    try:
        res = subprocess.run(
            ["docker", "ps", "-a", "--format", "{{json .}}"],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            timeout=3
        )
        if res.returncode == 0 and res.stdout.strip():
            for line in res.stdout.strip().splitlines():
                if line:
                    data = json.loads(line)
                    is_running = "Up" in data.get("Status", "")
                    name = data.get("Names", "unknown")
                    cid = data.get("ID", "")[:12]

                    c_stats = stats_map.get(name) or stats_map.get(cid) or {
                        "cpu": "--",
                        "mem": "--",
                        "mem_pct": "--",
                        "net": "--"
                    }

                    containers.append({
                        "id": cid,
                        "name": name,
                        "image": data.get("Image", "unknown"),
                        "status": data.get("Status", ""),
                        "is_running": is_running,
                        "ports": data.get("Ports", ""),
                        "stats": c_stats
                    })
    except Exception as e:
        logger.error("Error listing docker containers: %s", e)

    # Reclaimable disk stats
    df_info = {
        "images_reclaimable": "0 B",
        "containers_reclaimable": "0 B",
        "volumes_reclaimable": "0 B",
        "build_cache_reclaimable": "0 B",
        "total_reclaimable": "0 B"
    }
    try:
        res_df = subprocess.run(
            ["docker", "system", "df", "--format", "{{json .}}"],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            timeout=3
        )
        if res_df.returncode == 0 and res_df.stdout.strip():
            for line in res_df.stdout.strip().splitlines():
                if line:
                    item = json.loads(line)
                    t = item.get("Type", "").lower()
                    rec = item.get("Reclaimable", "0B")
                    if "image" in t:
                        df_info["images_reclaimable"] = rec
                    elif "container" in t:
                        df_info["containers_reclaimable"] = rec
                    elif "volume" in t:
                        df_info["volumes_reclaimable"] = rec
                    elif "build" in t:
                        df_info["build_cache_reclaimable"] = rec
    except Exception as e:
        logger.warning("Error fetching docker system df: %s", e)

    # Sort running containers to the top
    containers.sort(key=lambda c: not c["is_running"])

    return {
        "available": True,
        "containers": containers,
        "running_count": sum(1 for c in containers if c["is_running"]),
        "stopped_count": sum(1 for c in containers if not c["is_running"]),
        "disk": df_info
    }


def start_container(container_id: str) -> Dict[str, Any]:
    """Starts a Docker container."""
    try:
        res = subprocess.run(["docker", "start", container_id], capture_output=True, text=True, timeout=10)
        return {"success": res.returncode == 0, "message": f"Контейнер {container_id} запущен", "error": res.stderr}
    except Exception as e:
        return {"success": False, "error": str(e)}


def stop_container(container_id: str) -> Dict[str, Any]:
    """Stops a Docker container."""
    try:
        res = subprocess.run(["docker", "stop", container_id], capture_output=True, text=True, timeout=10)
        return {"success": res.returncode == 0, "message": f"Контейнер {container_id} остановлен", "error": res.stderr}
    except Exception as e:
        return {"success": False, "error": str(e)}


def restart_container(container_id: str) -> Dict[str, Any]:
    """Restarts a Docker container."""
    try:
        res = subprocess.run(["docker", "restart", container_id], capture_output=True, text=True, timeout=15)
        return {"success": res.returncode == 0, "message": f"Контейнер {container_id} перезапущен", "error": res.stderr}
    except Exception as e:
        return {"success": False, "error": str(e)}


def get_container_logs(container_id: str, tail: int = 60) -> Dict[str, Any]:
    """Fetches recent logs of a Docker container."""
    try:
        res = subprocess.run(
            ["docker", "logs", "--tail", str(tail), container_id],
            capture_output=True,
            text=True,
            timeout=5
        )
        logs = res.stdout or res.stderr or "Логи пусты"
        return {"success": True, "logs": logs}
    except Exception as e:
        return {"success": False, "error": str(e)}


def prune_docker(mode: str = "stopped") -> Dict[str, Any]:
    """Prunes docker containers, unused images, or build cache."""
    if not is_docker_available():
        return {"success": False, "error": "Docker недоступен"}

    cmd = ["docker"]
    if mode == "stopped":
        cmd.extend(["container", "prune", "-f"])
    elif mode == "images":
        cmd.extend(["image", "prune", "-f"])
    elif mode == "system":
        cmd.extend(["system", "prune", "-f"])
    else:
        return {"success": False, "error": f"Неизвестный режим {mode}"}

    try:
        res = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, timeout=15)
        return {
            "success": res.returncode == 0,
            "output": res.stdout.strip() or res.stderr.strip()
        }
    except Exception as e:
        return {"success": False, "error": str(e)}
