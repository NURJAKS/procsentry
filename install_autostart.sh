#!/usr/bin/env bash
# Autostart installer for ProcSentry
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_PATH="$HOME/.local/bin/procsentry"
if [ ! -f "$APP_PATH" ]; then
    if [ -f "$HOME/.local/bin/aegis" ]; then
        APP_PATH="$HOME/.local/bin/aegis"
    elif [ -f "$SCRIPT_DIR/bin/procsentry" ]; then
        APP_PATH="$SCRIPT_DIR/bin/procsentry"
    elif [ -f "$SCRIPT_DIR/procsentry" ]; then
        APP_PATH="$SCRIPT_DIR/procsentry"
    else
        APP_PATH="$SCRIPT_DIR/dev-sentinel"
    fi
fi
AUTOSTART_DIR="$HOME/.config/autostart"
DESKTOP_FILE="$AUTOSTART_DIR/procsentry.desktop"

chmod +x "$APP_PATH" 2>/dev/null || true
mkdir -p "$AUTOSTART_DIR"

cat <<EOF > "$DESKTOP_FILE"
[Desktop Entry]
Type=Application
Version=1.0
Name=ProcSentry (Go Engine)
GenericName=Zero-Overhead Linux Systems Guardian & Dev HUD
Comment=Ultra-fast Hardware Monitor, OOM Guard, Ghost Killer, and Port Radar
Exec=$APP_PATH
Icon=utilities-system-monitor
Terminal=false
Categories=System;Monitor;Development;
StartupNotify=false
X-GNOME-Autostart-enabled=true
EOF

chmod +x "$DESKTOP_FILE"

echo "=================================================="
echo "[OK] ProcSentry успешно добавлен в автозагрузку!"
echo "   Файл: $DESKTOP_FILE"
echo "   Теперь ProcSentry будет автоматически запускаться при входе в систему."
echo "=================================================="
