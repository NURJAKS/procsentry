#!/usr/bin/env bash
# Autostart remover for ProcSentry
set -e

AUTOSTART_DIR="$HOME/.config/autostart"

for FILE in "$AUTOSTART_DIR/procsentry.desktop" "$AUTOSTART_DIR/dev-sentinel.desktop"; do
    if [ -f "$FILE" ]; then
        rm -f "$FILE"
        echo "[OK] Удален из автозагрузки: $FILE"
    fi
done
echo "[OK] ProcSentry автозагрузка очищена."
