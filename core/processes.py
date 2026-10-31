"""Developer process and leak hunter.
Identifies dev servers, memory leaks, runaway CPU processes, and orphaned ghosts.
"""

import os
import time
import signal
import psutil
from typing import List, Dict, Any, Optional

DEV_KEYWORDS = {
    "node": ["node", "vite", "next", "nuxt", "webpack", "esbuild", "bun", "deno", "ts-node"],
    "python": ["python", "python3", "uvicorn", "gunicorn", "flask", "django", "celery", "tracker_bot", "scrapy"],
    "database": ["postgres", "redis-server", "mongod", "mysqld"],
    "compiler": ["cargo", "rustc", "go run", "javac", "gradle"]
}


# Processes or keywords that should NEVER be flagged as abandoned "ghosts"
GHOST_EXCLUSIONS = [
    "tracker_bot",
    "jobradar",
    "notify.tracker_bot",
    "mcp-server",
    "mcp-server-fetch",
    "mcp-server-github",
    "mcp-server-filesystem",
]

import re

def parse_friendly_process_info(name: str, cmdline: str) -> Dict[str, str]:
    """Transforms raw command lines and generic process names into clear, human-readable labels."""
    cmd_lower = cmdline.lower()

    # 1. Antigravity IDE AI MCP Servers
    if "mcp-server-fetch" in cmd_lower:
        return {
            "title": "MCP: Web Fetcher (AI Tool)",
            "subtitle": "Инструмент Antigravity AI для загрузки веб-страниц",
            "project": "Antigravity AI Sidecar",
            "badge_category": "ai"
        }
    if "mcp-server-github" in cmd_lower:
        return {
            "title": "MCP: GitHub Integration",
            "subtitle": "Инструмент Antigravity AI для работы с репозиториями",
            "project": "Antigravity AI Sidecar",
            "badge_category": "ai"
        }
    if "mcp-server-filesystem" in cmd_lower:
        # Check watched path
        match = re.search(r"mcp-server-filesystem\s+([^\s]+)", cmdline)
        path = match.group(1) if match else "Workspace"
        short_path = path.replace(os.path.expanduser("~"), "~")
        return {
            "title": "MCP: FileSystem Bridge",
            "subtitle": f"Мост доступа к файловой системе ({short_path})",
            "project": f"Antigravity AI ({short_path})",
            "badge_category": "ai"
        }

    # 2. Telegram bots / Job scrapers
    if "jobradar" in cmd_lower or "tracker_bot" in cmd_lower or "auto-отклики" in cmd_lower:
        return {
            "title": "Job Tracker Telegram Bot",
            "subtitle": "Фоновый бот отслеживания вакансий и откликов",
            "project": "~/Dev/auto-отклики extended",
            "badge_category": "bot"
        }

    # 3. Known Dev Frameworks
    if "vite" in cmd_lower:
        return {
            "title": "Vite Dev Server",
            "subtitle": "Локальный веб-сервер фронтенда (HMR)",
            "project": extract_project_name(cmdline),
            "badge_category": "frontend"
        }
    if "next" in cmd_lower and ("dev" in cmd_lower or "start" in cmd_lower):
        return {
            "title": "Next.js Server",
            "subtitle": "Сервер приложения Next.js",
            "project": extract_project_name(cmdline),
            "badge_category": "frontend"
        }
    if "uvicorn" in cmd_lower:
        return {
            "title": "Uvicorn (FastAPI / ASGI)",
            "subtitle": "Высокопроизводительный Python API сервер",
            "project": extract_project_name(cmdline),
            "badge_category": "backend"
        }
    if "gunicorn" in cmd_lower:
        return {
            "title": "Gunicorn WSGI Server",
            "subtitle": "Python веб-сервер приложений",
            "project": extract_project_name(cmdline),
            "badge_category": "backend"
        }
    if "celery" in cmd_lower:
        return {
            "title": "Celery Worker",
            "subtitle": "Фоновый обработчик асинхронных задач очереди",
            "project": extract_project_name(cmdline),
            "badge_category": "backend"
        }

    # 4. Extract Project Folder from ~/Dev/
    proj = extract_project_name(cmdline)
    script = extract_script_name(cmdline)
    
    clean_title = f"{proj} ({script})" if proj and script else (proj or script or name)

    return {
        "title": clean_title,
        "subtitle": cmdline[:80] + ("..." if len(cmdline) > 80 else ""),
        "project": proj or "Локальный скрипт",
        "badge_category": "python" if "python" in name.lower() else ("node" if "node" in name.lower() else "custom")
    }


def extract_project_name(cmdline: str) -> str:
    """Extracts project name if running inside /home/nurjaks/Dev/..."""
    match = re.search(r"/Dev/([^/\s]+)", cmdline)
    if match:
        return f"~/Dev/{match.group(1)}"
    return ""


def extract_script_name(cmdline: str) -> str:
    """Extracts filename of executed script (e.g. app.py, server.ts)."""
    match = re.search(r"([a-zA-Z0-9_\-\.]+\.(?:py|js|ts|mjs|cjs|sh))\b", cmdline)
    if match:
        return match.group(1)
    return ""


def is_system_managed_service(pid: int) -> bool:
    """Checks if a process is managed by systemd as an OS daemon or user service unit."""
    try:
        with open(f"/proc/{pid}/cgroup", "r", encoding="utf-8") as f:
            content = f.read()
            if "/system.slice" in content:
                return True
            for line in content.splitlines():
                if ".service" in line:
                    return True
    except (FileNotFoundError, ProcessLookupError, PermissionError):
        pass
    return False


def is_dev_process(name: str, cmdline_str: str) -> Optional[str]:
    """Checks if a process matches developer tooling keywords."""
    cmd_lower = cmdline_str.lower()
    name_lower = name.lower()

    # Exclude system or IDE internal helper processes
    if "antigravity-ide" in cmd_lower or "language_server" in cmd_lower or "systemd" in cmd_lower:
        return None
    if "gnome-terminal" in cmd_lower or "chrome" in cmd_lower:
        return None

    for category, keywords in DEV_KEYWORDS.items():
        for kw in keywords:
            if kw in name_lower or f" {kw} " in f" {cmd_lower} " or cmd_lower.startswith(f"{kw} "):
                return category
    return None


def get_dev_processes() -> List[Dict[str, Any]]:
    """Returns all active developer processes owned by current user."""
    current_uid = os.getuid()
    now = time.time()
    results = []

    for proc in psutil.process_iter(["pid", "ppid", "name", "cmdline", "cpu_percent", "memory_info", "create_time", "uids"]):
        try:
            info = proc.info
            uids = info.get("uids")
            if uids and uids.real != current_uid:
                continue

            pid = info["pid"]
            if pid == os.getpid():
                continue

            name = info.get("name") or "unknown"
            cmdline_list = info.get("cmdline") or []
            cmdline = " ".join(cmdline_list)
            
            category = is_dev_process(name, cmdline)
            if not category:
                continue

            mem_info = info.get("memory_info")
            rss_mb = round(mem_info.rss / (1024 * 1024), 1) if mem_info else 0.0
            cpu = round(proc.cpu_percent(interval=None), 1)
            create_time = info.get("create_time", now)
            uptime_hours = round((now - create_time) / 3600.0, 1)
            ppid = info.get("ppid", 0)

            # Heuristic for "Ghost / Abandoned":
            # 1. Parent process is init (ppid == 1) meaning terminal closed
            # 2. Or running for > 3 hours without active interaction
            #
            # Exceptions (NOT ghosts):
            # - Whitelisted persistent bots/tools (e.g. tracker_bot, jobradar, mcp-server)
            # - Processes managed as systemd services (/system.slice or *.service)
            is_service = is_system_managed_service(pid)
            cmd_lower = cmdline.lower()
            is_whitelisted = any(ex in cmd_lower for ex in GHOST_EXCLUSIONS)

            if is_whitelisted or is_service:
                is_ghost = False
            else:
                is_ghost = (ppid == 1) or (uptime_hours > 3.0 and cpu < 1.0)

            friendly_meta = parse_friendly_process_info(name, cmdline)

            results.append({
                "pid": pid,
                "ppid": ppid,
                "name": name,
                "category": friendly_meta.get("badge_category", category),
                "title": friendly_meta.get("title", name),
                "subtitle": friendly_meta.get("subtitle", cmdline),
                "project": friendly_meta.get("project", ""),
                "cmdline": cmdline,
                "memory_mb": rss_mb,
                "cpu_percent": cpu,
                "uptime_hours": uptime_hours,
                "is_ghost": is_ghost,
                "is_service": is_service or is_whitelisted
            })
        except (psutil.NoSuchProcess, psutil.AccessDenied):
            continue

    # Sort: ghosts first, then by memory usage descending
    results.sort(key=lambda x: (not x["is_ghost"], -x["memory_mb"]))
    return results


def kill_process(pid: int, force: bool = False) -> Dict[str, Any]:
    """Terminates a process safely with fallback to SIGKILL."""
    try:
        p = psutil.Process(pid)
        name = p.name()
        
        # Verify ownership
        if p.uids().real != os.getuid():
            return {"success": False, "error": "Недостаточно прав (чужой процесс)"}

        if force:
            p.kill()
        else:
            p.terminate()
            # Wait up to 1 second
            try:
                p.wait(timeout=1.0)
            except psutil.TimeoutExpired:
                p.kill()

        return {"success": True, "message": f"Процесс {name} (PID {pid}) успешно завершен"}
    except psutil.NoSuchProcess:
        return {"success": True, "message": "Процесс уже был завершен"}
    except Exception as e:
        return {"success": False, "error": str(e)}


def kill_all_ghosts() -> Dict[str, Any]:
    """Terminates all detected ghost/abandoned developer processes."""
    dev_procs = get_dev_processes()
    ghosts = [p for p in dev_procs if p["is_ghost"]]
    
    killed = 0
    freed_mb = 0.0
    for g in ghosts:
        res = kill_process(g["pid"])
        if res.get("success"):
            killed += 1
            freed_mb += g["memory_mb"]

    return {
        "killed_count": killed,
        "freed_mb": round(freed_mb, 1),
        "message": f"Завершено {killed} зависших процессов, освобождено {round(freed_mb, 1)} MB ОЗУ"
    }
