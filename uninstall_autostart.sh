#!/usr/bin/env bash
# Autostart remover for Dev Laptop Sentinel
set -e

DESKTOP_FILE="$HOME/.config/autostart/dev-sentinel.desktop"

if [ -f "$DESKTOP_FILE" ]; then
    rm -f "$DESKTOP_FILE"
    echo "[OK] DevAegis удален из автозагрузки ($DESKTOP_FILE)"
else
    echo "[INFO] Файл автозагрузки не найден."
fi
