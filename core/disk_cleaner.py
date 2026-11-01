"""Developer disk cache analyzer and space recovery.
Inspects developer caches (~/.cache, ~/.npm, ~/.cargo, gradle, etc.).
"""

import os
import time
import shutil
import subprocess
from typing import Dict, Any, List

TARGET_CACHES = [
    {"name": "NPM Cache", "path": os.path.expanduser("~/.npm"), "desc": "Кэш загруженных пакетов Node.js"},
    {"name": "Yarn Cache", "path": os.path.expanduser("~/.cache/yarn"), "desc": "Кэш пакетов Yarn"},
    {"name": "Cargo / Rust Cache", "path": os.path.expanduser("~/.cargo/registry/cache"), "desc": "Архивы пакетов Crates.io"},
    {"name": "Pip Cache", "path": os.path.expanduser("~/.cache/pip"), "desc": "Кэш пакетов Python Wheels"},
    {"name": "Thumbnail Cache", "path": os.path.expanduser("~/.cache/thumbnails"), "desc": "Миниатюры картинок GNOME"},
    {"name": "Chrome / Browser Cache", "path": os.path.expanduser("~/.cache/google-chrome"), "desc": "Кэш страниц и медиа браузера Chrome"}
]

_CACHES_CACHE: Dict[str, Any] = {"results": [], "time": 0.0}


def get_dir_size_mb(path: str) -> float:
    """Calculates directory size in Megabytes."""
    if not os.path.exists(path):
        return 0.0
    total = 0
    try:
        # Use du -sm for fast disk analysis
        res = subprocess.run(["du", "-sm", path], stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True, timeout=2)
        if res.returncode == 0 and res.stdout.strip():
            return float(res.stdout.split()[0])
    except Exception:
        pass

    # Fallback to os.walk
    try:
        for root, _, files in os.walk(path):
            for f in files:
                fp = os.path.join(root, f)
                if not os.path.islink(fp):
                    total += os.path.getsize(fp)
        return round(total / (1024 * 1024), 1)
    except Exception:
        return 0.0


def scan_caches(force: bool = False) -> List[Dict[str, Any]]:
    """Returns scanned developer caches and their sizes. Cached for 60s."""
    now = time.time()
    if not force and (now - _CACHES_CACHE["time"] < 60.0) and _CACHES_CACHE["results"]:
        return _CACHES_CACHE["results"]

    results = []
    for item in TARGET_CACHES:
        path = item["path"]
        if os.path.exists(path):
            size_mb = get_dir_size_mb(path)
            results.append({
                "name": item["name"],
                "path": path,
                "desc": item["desc"],
                "size_mb": size_mb,
                "size_display": f"{round(size_mb / 1024, 2)} GB" if size_mb >= 1024 else f"{round(size_mb, 1)} MB"
            })
    results.sort(key=lambda x: x["size_mb"], reverse=True)
    _CACHES_CACHE["results"] = results
    _CACHES_CACHE["time"] = now
    return results


def clean_cache(name: str) -> Dict[str, Any]:
    """Cleans a specific target cache."""
    _CACHES_CACHE["time"] = 0.0
    target = next((item for item in TARGET_CACHES if item["name"] == name), None)
    if not target:
        return {"success": False, "error": f"Кэш '{name}' не найден"}

    path = target["path"]
    if not os.path.exists(path):
        return {"success": True, "message": f"{name} уже пуст"}

    try:
        # Remove contents inside the directory rather than the directory itself
        for item in os.listdir(path):
            item_path = os.path.join(path, item)
            if os.path.isdir(item_path):
                shutil.rmtree(item_path, ignore_errors=True)
            else:
                try:
                    os.remove(item_path)
                except Exception:
                    pass
        return {"success": True, "message": f"Кэш '{name}' успешно очищен"}
    except Exception as e:
        return {"success": False, "error": str(e)}
