# rclone-orbit

`rclone-orbit` is a small setup tool for a lightweight rclone-based live sync service.

The Go binary handles setup, checks, and service generation. The long-running service is still a plain shell script, generated from embedded templates, so it stays easy to inspect and debug.

## Install

```sh
brew tap ojarosch/tap
brew trust ojarosch/tap
brew install rclone-orbit
```

From source:

```sh
go install github.com/ojarosch/rclone-orbit/cmd/rclone-orbit@latest
```

## Commands

```sh
rclone-orbit wizard       # interactive setup
rclone-orbit doctor       # dependency and rclone preflight checks
rclone-orbit install      # install generated script + service from current config
rclone-orbit uninstall    # remove service, keep config/logs
rclone-orbit status       # show launchd/systemd status
rclone-orbit logs         # print log paths
rclone-orbit write-config # write default config
```

## Runtime dependencies

- `rclone`
- macOS watcher: `fswatch`
- Linux watcher: `inotifywait` from `inotify-tools`
- Optional Discord webhook alerts: `jq` + `curl`

## Config

The wizard writes:

```text
~/.config/rclone-orbit/rclone-orbit.env
```

Defaults:

```sh
LOCAL_DIR="$HOME/Documents"
REMOTE_LIVE="remote:sync"
REMOTE_BACKUP="remote:backup"
BACKUP_RETENTION_DAYS=90
PRUNE_INTERVAL_DAYS=7
DEBOUNCE_SECONDS=300
RCLONE_TRANSFERS=2
```

## Generated files

macOS:

```text
~/.local/bin/rclone_orbit.sh
~/Library/LaunchAgents/com.user.rclone-orbit.plist
~/Library/Logs/rclone_orbit.log
~/Library/Logs/rclone_orbit_error.log
```

Linux:

```text
~/.local/bin/rclone_orbit.sh
~/.config/systemd/user/rclone-orbit.service
```

## Development checks

```sh
go test ./...
shellcheck bin/rclone_orbit.sh install-macos.sh uninstall-macos.sh
```
