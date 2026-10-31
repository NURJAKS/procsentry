"""FastAPI backend server for Dev Laptop Sentinel.
Provides REST API for telemetry, process management, port killer, Docker, and cache cleanup.
"""

import os
from fastapi import FastAPI, HTTPException
from fastapi.staticfiles import StaticFiles
from fastapi.responses import FileResponse, JSONResponse
from pydantic import BaseModel
from typing import Optional

from core.hardware import get_system_telemetry, get_gpu_mode, switch_gpu_mode
from core.processes import get_dev_processes, kill_process, kill_all_ghosts
from core.ports import get_listening_ports, free_port
from core.docker_cleaner import (
    get_docker_telemetry,
    prune_docker,
    start_container,
    stop_container,
    restart_container,
    get_container_logs
)
from core.disk_cleaner import scan_caches, clean_cache

app = FastAPI(title="Dev Laptop Sentinel API", version="1.0.0")

# Models
class KillProcessRequest(BaseModel):
    pid: int
    force: bool = False

class FreePortRequest(BaseModel):
    port: int

class DockerPruneRequest(BaseModel):
    mode: str = "stopped"

class ContainerActionRequest(BaseModel):
    id: str

class CleanCacheRequest(BaseModel):
    name: str

class SwitchGpuRequest(BaseModel):
    mode: str


@app.get("/api/telemetry")
async def api_telemetry():
    return get_system_telemetry()


@app.get("/api/gpu/mode")
async def api_gpu_mode():
    mode = get_gpu_mode()
    return {
        "current": mode,
        "options": ["intel", "on-demand", "nvidia"],
        "recommended": "on-demand"
    }


@app.post("/api/gpu/switch")
async def api_gpu_switch(req: SwitchGpuRequest):
    return switch_gpu_mode(req.mode)


@app.get("/api/processes")
async def api_processes():
    return get_dev_processes()


@app.post("/api/processes/kill")
async def api_kill_process(req: KillProcessRequest):
    res = kill_process(req.pid, req.force)
    if not res.get("success"):
        raise HTTPException(status_code=400, detail=res.get("error", "Error"))
    return res


@app.post("/api/processes/kill_ghosts")
async def api_kill_ghosts():
    return kill_all_ghosts()


@app.get("/api/ports")
async def api_ports():
    return get_listening_ports()


@app.post("/api/ports/free")
async def api_free_port(req: FreePortRequest):
    res = free_port(req.port)
    if not res.get("success"):
        raise HTTPException(status_code=400, detail=res.get("error", "Error"))
    return res


@app.get("/api/docker")
async def api_docker():
    return get_docker_telemetry()


@app.post("/api/docker/start")
async def api_docker_start(req: ContainerActionRequest):
    res = start_container(req.id)
    if not res.get("success"):
        raise HTTPException(status_code=400, detail=res.get("error", "Error starting container"))
    return res


@app.post("/api/docker/stop")
async def api_docker_stop(req: ContainerActionRequest):
    res = stop_container(req.id)
    if not res.get("success"):
        raise HTTPException(status_code=400, detail=res.get("error", "Error stopping container"))
    return res


@app.post("/api/docker/restart")
async def api_docker_restart(req: ContainerActionRequest):
    res = restart_container(req.id)
    if not res.get("success"):
        raise HTTPException(status_code=400, detail=res.get("error", "Error restarting container"))
    return res


@app.get("/api/docker/logs/{container_id}")
async def api_docker_logs(container_id: str):
    res = get_container_logs(container_id)
    if not res.get("success"):
        raise HTTPException(status_code=400, detail=res.get("error", "Error reading logs"))
    return res


@app.post("/api/docker/prune")
async def api_docker_prune(req: DockerPruneRequest):
    res = prune_docker(req.mode)
    if not res.get("success"):
        raise HTTPException(status_code=400, detail=res.get("error", "Error"))
    return res


@app.get("/api/caches")
async def api_caches():
    return scan_caches()


@app.post("/api/caches/clean")
async def api_clean_cache(req: CleanCacheRequest):
    res = clean_cache(req.name)
    if not res.get("success"):
        raise HTTPException(status_code=400, detail=res.get("error", "Error"))
    return res


# Static files mount
static_dir = os.path.join(os.path.dirname(__file__), "static")
if os.path.exists(static_dir):
    app.mount("/static", StaticFiles(directory=static_dir), name="static")

    @app.api_route("/", methods=["GET", "HEAD"])
    async def serve_index():
        return FileResponse(os.path.join(static_dir, "index.html"))
