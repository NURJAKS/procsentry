#!/usr/bin/env bash
# Autostart installer for Dev Laptop Sentinel
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_PATH="$SCRIPT_DIR/app.py"
AUTOSTART_DIR="$HOME/.config/autostart"
DESKTOP_FILE="$AUTOSTART_DIR/dev-sentinel.desktop"

chmod +x "$APP_PATH"
mkdir -p "$AUTOSTART_DIR"

cat <<EOF > "$DESKTOP_FILE"
[Desktop Entry]
Type=Application
Version=1.0
Name=Dev Laptop Sentinel
GenericName=Laptop Health & Dev Workspace Monitor
Comment=Monitors hardware temperatures, RAM pressure, ghost servers, and ports
Exec=/usr/bin/python3 "$APP_PATH"
Icon=utilities-system-monitor
Terminal=false
Categories=System;Monitor;Development;
StartupNotify=false
X-GNOME-Autostart-enabled=true
EOF

chmod +x "$DESKTOP_FILE"

echo "=================================================="
echo "✅ Dev Laptop Sentinel успешно добавлен в автозагрузку!"
echo "   Файл: $DESKTOP_FILE"
echo "   Теперь Sentinel будет автоматически запускаться при входе в систему."
echo "=================================================="
