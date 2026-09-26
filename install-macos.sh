#!/usr/bin/env bash
set -eu

ROOT="$(cd "$(dirname "$0")" && pwd)"
SOURCE_SCRIPT="$ROOT/bin/rclone_orbit.sh"
SCRIPT="$HOME/.local/bin/rclone_orbit.sh"
PLIST="$HOME/Library/LaunchAgents/com.user.rclone-orbit.plist"

chmod +x "$SOURCE_SCRIPT"
mkdir -p "$HOME/.local/bin" "$HOME/Library/LaunchAgents" "$HOME/Library/Logs" "$HOME/.config/rclone-orbit"
cp "$SOURCE_SCRIPT" "$SCRIPT"
chmod +x "$SCRIPT"

sed \
  -e "s#__SCRIPT_PATH__#$SCRIPT#g" \
  -e "s#__HOME__#$HOME#g" \
  "$ROOT/launchd/com.user.rclone-orbit.plist.template" > "$PLIST"

launchctl bootout "gui/$(id -u)" "$PLIST" >/dev/null 2>&1 || true
launchctl bootstrap "gui/$(id -u)" "$PLIST"
launchctl kickstart -k "gui/$(id -u)/com.user.rclone_orbit"

echo "Installed rclone-orbit LaunchAgent: $PLIST"
