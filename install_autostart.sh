#!/usr/bin/env bash
# Autostart installer for Dev Laptop Sentinel
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_PATH="$HOME/.local/bin/dev-sentinel"
if [ ! -f "$APP_PATH" ]; then
    APP_PATH="$SCRIPT_DIR/dev-sentinel"
fi
AUTOSTART_DIR="$HOME/.config/autostart"
DESKTOP_FILE="$AUTOSTART_DIR/dev-sentinel.desktop"

chmod +x "$APP_PATH"
mkdir -p "$AUTOSTART_DIR"

cat <<EOF > "$DESKTOP_FILE"
[Desktop Entry]
Type=Application
Version=1.0
Name=Dev Laptop Sentinel (Go Engine)
GenericName=Laptop Health & Dev Workspace Guardian
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
echo "[OK] DevAegis успешно добавлен в автозагрузку!"
echo "   Файл: $DESKTOP_FILE"
echo "   Теперь DevAegis будет автоматически запускаться при входе в систему."
echo "=================================================="
