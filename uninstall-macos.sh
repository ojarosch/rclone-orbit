#!/usr/bin/env bash
set -eu

PLIST="$HOME/Library/LaunchAgents/com.user.rclone-orbit.plist"
launchctl bootout "gui/$(id -u)" "$PLIST" >/dev/null 2>&1 || true
rm -f "$PLIST"
echo "Uninstalled rclone-orbit LaunchAgent. Config/logs left in place."
