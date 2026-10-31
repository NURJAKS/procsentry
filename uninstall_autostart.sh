#!/usr/bin/env bash
# Autostart remover for Dev Laptop Sentinel
set -e

DESKTOP_FILE="$HOME/.config/autostart/dev-sentinel.desktop"

if [ -f "$DESKTOP_FILE" ]; then
    rm -f "$DESKTOP_FILE"
    echo "✅ Dev Laptop Sentinel удален из автозагрузки ($DESKTOP_FILE)"
else
    echo "ℹ️ Файл автозагрузки не найден."
fi
