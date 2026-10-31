"""Developer Port Conflict Radar.
Detects listening ports, maps them to processes, and allows freeing ports in 1-click.
"""

import os
import psutil
import subprocess
from typing import List, Dict, Any, Optional

COMMON_DEV_PORTS = {
    1431: "MSSQL Admin Connection",
    1433: "Microsoft SQL Server (MSSQL)",
    1434: "MSSQL Dedicated Admin (DAC)",
    1435: "MSSQL Internal Worker",
    3000: "React / Next.js / Node",
    3001: "Dev Server / Alternate",
    4000: "GraphQL / Hexo",
    4200: "Angular CLI",
    5000: "Flask / .NET / Python",
    5173: "Vite / Svelte / Vue",
    8000: "Django / FastAPI / Uvicorn",
    8080: "Spring Boot / Tomcat / Vue",
    8443: "HTTPS Dev Server",
    9000: "PHP-FPM / SonarQube / MinIO",
    9222: "Chrome Remote Debugging (CDP)",
    5432: "PostgreSQL",
    6379: "Redis",
    27017: "MongoDB",
    3306: "MySQL / MariaDB",
    11434: "Ollama Local LLM API",
    48888: "Dev Laptop Sentinel (Self)"
}


def get_listening_ports() -> List[Dict[str, Any]]:
    """Returns list of currently listening TCP ports and associated processes."""
    results = []
    seen = set()

    try:
        connections = psutil.net_connections(kind="inet")
    except Exception:
        connections = []

    for conn in connections:
        if conn.status != psutil.CONN_LISTEN:
            continue

        port = conn.laddr.port
        ip = conn.laddr.ip
        key = (ip, port)
        if key in seen:
            continue
        seen.add(key)

        pid = conn.pid
        proc_name = "System / Other"
        cmdline = ""
        memory_mb = 0.0
        can_kill = False

        if pid:
            try:
                p = psutil.Process(pid)
                proc_name = p.name()
                cmdline = " ".join(p.cmdline() or [])
                mem = p.memory_info()
                memory_mb = round(mem.rss / (1024 * 1024), 1) if mem else 0.0
                can_kill = (p.uids().real == os.getuid())
            except Exception:
                pass

        # Intelligent service hint resolution
        cmd_lower = cmdline.lower()
        proc_lower = proc_name.lower()
        
        if port in COMMON_DEV_PORTS:
            hint = COMMON_DEV_PORTS[port]
        elif "language_server" in cmd_lower or "language_server" in proc_lower:
            hint = "IDE Language Server (LSP)"
        elif "antigravity-ide" in cmd_lower or "antigravity-ide" in proc_lower:
            hint = "Antigravity IDE Internal"
        elif "sqlservr" in cmd_lower or "sqlservr" in proc_lower:
            hint = "Microsoft SQL Server"
        elif "chrome" in cmd_lower:
            hint = "Chrome Browser Debugger"
        elif "python" in proc_lower and port == 48888:
            hint = "Dev Sentinel Monitor"
        elif port > 1024:
            hint = "Custom Dev Service"
        else:
            hint = "System Port"

        is_dev_port = port in COMMON_DEV_PORTS or port >= 3000

        results.append({
            "port": port,
            "ip": ip,
            "pid": pid,
            "name": proc_name,
            "cmdline": cmdline[:100] + ("..." if len(cmdline) > 100 else ""),
            "memory_mb": memory_mb,
            "can_kill": can_kill,
            "is_dev_port": is_dev_port,
            "hint": hint
        })

    # Sort dev ports first, then by port number
    results.sort(key=lambda x: (not x["is_dev_port"], x["port"]))
    return results


def free_port(port: int) -> Dict[str, Any]:
    """Finds and terminates the process listening on the specified port."""
    ports = get_listening_ports()
    target = next((p for p in ports if p["port"] == port), None)

    if not target or not target["pid"]:
        return {"success": False, "error": f"На порту {port} нет активного процесса или он принадлежит другой учетной записи"}

    pid = target["pid"]
    try:
        p = psutil.Process(pid)
        p_name = p.name()
        p.terminate()
        try:
            p.wait(timeout=1.0)
        except psutil.TimeoutExpired:
            p.kill()

        return {
            "success": True,
            "message": f"Порт {port} успешно освобожден (завершен процесс {p_name}, PID {pid})"
        }
    except Exception as e:
        return {"success": False, "error": f"Не удалось завершить процесс: {str(e)}"}
