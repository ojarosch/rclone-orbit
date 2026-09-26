#!/usr/bin/env bash

# rclone_orbit.sh — watch ~/Documents and rclone-sync to encrypted remote.
# v1: shell daemon using rclone + fswatch/inotifywait, with webhook alerts and weekly backup prune.

# Defaults. Override in ~/.config/rclone-orbit/rclone-orbit.env or environment.
LOCAL_DIR="${LOCAL_DIR:-$HOME/Documents}"
REMOTE_LIVE="${REMOTE_LIVE:-remote:sync}"
REMOTE_BACKUP="${REMOTE_BACKUP:-remote:backup}"
BACKUP_RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-90}"
PRUNE_INTERVAL_DAYS="${PRUNE_INTERVAL_DAYS:-7}"
DEBOUNCE_SECONDS="${DEBOUNCE_SECONDS:-300}"
RCLONE_TRANSFERS="${RCLONE_TRANSFERS:-2}"
LOCKFILE="${LOCKFILE:-/tmp/rclone_orbit.lock}"
WEBHOOK_URL="${WEBHOOK_URL:-}"
CONFIG_FILE="${CONFIG_FILE:-$HOME/.config/rclone-orbit/rclone-orbit.env}"

# shellcheck source=/dev/null
[ -f "$CONFIG_FILE" ] && . "$CONFIG_FILE"

export PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:$PATH"

log() {
    echo "[$(date +'%Y-%m-%d %H:%M:%S')] $*"
}

need() {
    command -v "$1" >/dev/null 2>&1 || {
        log "Missing dependency: $1"
        exit 127
    }
}

need rclone

# Single-instance guard via PID lockfile.
if [ -f "$LOCKFILE" ]; then
    OLD_PID=$(cat "$LOCKFILE" 2>/dev/null)
    if [ -n "$OLD_PID" ] && kill -0 "$OLD_PID" 2>/dev/null; then
        log "Another instance (pid $OLD_PID) holds the lock. Exiting."
        exit 0
    fi
    log "Removing stale lock (pid ${OLD_PID:-unknown} gone)."
fi
echo $$ > "$LOCKFILE"
cleanup() {
    rm -f "$LOCKFILE"
}
trap cleanup EXIT INT TERM

DISCORD_WEBHOOK_URL="$WEBHOOK_URL"

notify_discord() {
    local error_msg="$1"
    [ -z "$DISCORD_WEBHOOK_URL" ] && return 0

    if command -v jq >/dev/null 2>&1; then
        local payload
        payload=$(jq -n --arg msg "🚨 **Rclone Sync Failed** 🚨\n\`\`\`text\n${error_msg}\n\`\`\`" '{content: $msg}')
        curl -sS -H "Content-Type: application/json" -X POST -d "$payload" "$DISCORD_WEBHOOK_URL" >/dev/null 2>&1 || true
    else
        log "jq missing; skipping Discord notification."
    fi
}

prune_backup() {
    # ponytail: simple timestamp file; enough for one daemon, upgrade to per-remote state if reused for many remotes.
    PRUNE_STATE="$HOME/.cache/rclone_orbit_last_prune"
    mkdir -p "$HOME/.cache"
    LAST_PRUNE=0
    [ -f "$PRUNE_STATE" ] && LAST_PRUNE=$(cat "$PRUNE_STATE" 2>/dev/null || echo 0)
    NOW=$(date +%s)
    if [ $(( NOW - LAST_PRUNE )) -ge $(( PRUNE_INTERVAL_DAYS * 86400 )) ]; then
        log "Pruning backup entries older than ${BACKUP_RETENTION_DAYS}d..."
        if PRUNE_OUTPUT=$(rclone delete "$REMOTE_BACKUP" --min-age "${BACKUP_RETENTION_DAYS}d" --rmdirs -v 2>&1); then
            echo "$PRUNE_OUTPUT" | tail -n 3
            date +%s > "$PRUNE_STATE"
        else
            echo "$PRUNE_OUTPUT" | tail -n 10
            notify_discord "$(echo "$PRUNE_OUTPUT" | tail -n 10)"
        fi
    fi
}

sync_once() {
    log "Executing secure sync operation..."

    if ! OUTPUT=$(rclone sync "$LOCAL_DIR" "$REMOTE_LIVE" \
            --backup-dir "$REMOTE_BACKUP/$(date +%Y-%m-%d)" \
            --fast-list --transfers "$RCLONE_TRANSFERS" -v 2>&1); then
        log "Sync failed. Triggering webhook..."
        notify_discord "$(echo "$OUTPUT" | tail -n 10)"
    else
        log "Sync completed successfully."
        prune_backup
    fi
}

watch_changes() {
    if command -v fswatch >/dev/null 2>&1; then
        log "Watching $LOCAL_DIR for changes (${DEBOUNCE_SECONDS}s debounce via fswatch)..."
        fswatch -o -l "$DEBOUNCE_SECONDS" "$LOCAL_DIR" | while read -r num; do
            log "$num change(s) detected. Triggering sync."
            sync_once
        done
    elif command -v inotifywait >/dev/null 2>&1; then
        log "Watching $LOCAL_DIR for changes (${DEBOUNCE_SECONDS}s debounce via inotifywait)..."
        while inotifywait -r -e close_write,move,create,delete "$LOCAL_DIR" >/dev/null 2>&1; do
            sleep "$DEBOUNCE_SECONDS"
            log "Change detected. Triggering sync."
            sync_once
        done
    else
        log "Missing watcher: install fswatch on macOS or inotify-tools on Linux."
        exit 127
    fi
}

log "Starting sync daemon..."
sync_once
watch_changes
