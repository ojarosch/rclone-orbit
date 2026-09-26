package app

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

type Config struct {
	LocalDir            string
	RemoteLive          string
	RemoteBackup        string
	BackupRetentionDays string
	PruneIntervalDays   string
	DebounceSeconds     string
	RcloneTransfers     string
	Lockfile            string
	WebhookURL          string
}

func DefaultConfig() Config {
	home, _ := os.UserHomeDir()
	return Config{
		LocalDir:            filepath.Join(home, "Documents"),
		RemoteLive:          "remote:sync",
		RemoteBackup:        "remote:backup",
		BackupRetentionDays: "90",
		PruneIntervalDays:   "7",
		DebounceSeconds:     "300",
		RcloneTransfers:     "2",
		Lockfile:            "/tmp/rclone_orbit.lock",
	}
}

func ConfigFromValues(v []string) Config {
	c := DefaultConfig()
	set := func(i int, dst *string, expand bool) {
		if len(v) > i && strings.TrimSpace(v[i]) != "" {
			*dst = strings.TrimSpace(v[i])
			if expand {
				*dst = expandHome(*dst)
			}
		}
	}
	set(0, &c.LocalDir, true)
	set(1, &c.RemoteLive, false)
	set(2, &c.RemoteBackup, false)
	set(3, &c.BackupRetentionDays, false)
	set(4, &c.PruneIntervalDays, false)
	set(5, &c.DebounceSeconds, false)
	set(6, &c.RcloneTransfers, false)
	set(7, &c.WebhookURL, false)
	return c
}

func Validate(c Config) error {
	var problems []string
	if c.LocalDir == "" {
		problems = append(problems, "local folder is required")
	} else if st, err := os.Stat(c.LocalDir); err != nil || !st.IsDir() {
		problems = append(problems, "local folder must exist: "+c.LocalDir)
	}
	for name, value := range map[string]string{"live rclone path": c.RemoteLive, "backup rclone path": c.RemoteBackup} {
		if !strings.Contains(value, ":") || strings.HasPrefix(value, ":") {
			problems = append(problems, name+" must look like remote:path")
		}
	}
	for name, value := range map[string]string{
		"backup retention days": c.BackupRetentionDays,
		"prune interval days":   c.PruneIntervalDays,
		"debounce seconds":      c.DebounceSeconds,
		"rclone transfers":      c.RcloneTransfers,
	} {
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 {
			problems = append(problems, name+" must be a positive integer")
		}
	}
	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}

func expandHome(s string) string {
	if s == "~" || strings.HasPrefix(s, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, strings.TrimPrefix(s, "~/"))
	}
	return s
}

func ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "rclone-orbit", "rclone-orbit.env")
}

func ScriptPath() string {
	return ScriptPathFor(DefaultConfig())
}

func ScriptPathFor(c Config) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin", "rclone_orbit_worker_"+RemoteName(c.RemoteLive))
}

func RemoteName(remotePath string) string {
	name := strings.SplitN(remotePath, ":", 2)[0]
	name = strings.ToLower(name)
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "default"
	}
	return b.String()
}

func LaunchAgentPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", "com.user.rclone-orbit.plist")
}

func SystemdPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", "rclone-orbit.service")
}

func Doctor() []string {
	var out []string
	missing := map[string]bool{}
	check := func(name string) {
		if p, err := exec.LookPath(name); err == nil {
			out = append(out, "✓ "+name+" found: "+p)
		} else {
			out = append(out, "✗ "+name+" missing")
			missing[name] = true
		}
	}
	check("rclone")
	if runtime.GOOS == "darwin" {
		check("fswatch")
	} else {
		check("inotifywait")
	}
	if remotes, err := RcloneRemotes(); err == nil && len(remotes) > 0 {
		out = append(out, fmt.Sprintf("✓ rclone remotes: %s", strings.Join(remotes, ", ")))
	} else if err != nil {
		out = append(out, "✗ rclone listremotes failed: "+err.Error())
	} else {
		out = append(out, "✗ no rclone remotes configured")
	}
	if runtime.GOOS == "linux" && (missing["rclone"] || missing["inotifywait"]) {
		out = append(out, "hint Bazzite with Homebrew: brew install rclone inotify-tools")
		out = append(out, "hint Fedora Workstation: sudo dnf install rclone inotify-tools")
		out = append(out, "hint Arch/CachyOS: sudo pacman -S rclone inotify-tools")
	}
	return out
}

func RcloneRemotes() ([]string, error) {
	b, err := exec.Command("rclone", "listremotes").Output()
	if err != nil {
		return nil, err
	}
	var remotes []string
	s := bufio.NewScanner(strings.NewReader(string(b)))
	for s.Scan() {
		remote := strings.TrimSuffix(strings.TrimSpace(s.Text()), ":")
		if remote != "" {
			remotes = append(remotes, remote)
		}
	}
	sort.Strings(remotes)
	return remotes, s.Err()
}

func WriteConfig(c Config) error {
	if err := Validate(c); err != nil {
		return err
	}
	return writeFileIfChanged(ConfigPath(), []byte(configContent(c)), 0o644)
}

func configContent(c Config) string {
	return fmt.Sprintf(`LOCAL_DIR=%q
REMOTE_LIVE=%q
REMOTE_BACKUP=%q
BACKUP_RETENTION_DAYS=%s
PRUNE_INTERVAL_DAYS=%s
DEBOUNCE_SECONDS=%s
RCLONE_TRANSFERS=%s
LOCKFILE=%q
WEBHOOK_URL=%q
`, c.LocalDir, c.RemoteLive, c.RemoteBackup, c.BackupRetentionDays, c.PruneIntervalDays, c.DebounceSeconds, c.RcloneTransfers, c.Lockfile, c.WebhookURL)
}

func writeFileIfChanged(path string, content []byte, mode os.FileMode) error {
	if current, err := os.ReadFile(path); err == nil && string(current) == string(content) { // #nosec G304 -- path is generated by this app (config, worker, plist, or service file).
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, content, mode)
}

func fileChanged(path string, content []byte) bool {
	current, err := os.ReadFile(path) // #nosec G304 -- path is generated by this app (worker, plist, or service file).
	return err != nil || string(current) != string(content)
}

func Install() error {
	cfg, err := LoadConfig()
	if err != nil {
		cfg = DefaultConfig()
	}
	return InstallWithConfig(cfg, true)
}

func LoadConfig() (Config, error) {
	cfg := DefaultConfig()
	b, err := os.ReadFile(ConfigPath())
	if err != nil {
		return cfg, err
	}
	for _, line := range strings.Split(string(b), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || strings.HasPrefix(key, "#") {
			continue
		}
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		switch key {
		case "LOCAL_DIR":
			cfg.LocalDir = expandHome(value)
		case "REMOTE_LIVE":
			cfg.RemoteLive = value
		case "REMOTE_BACKUP":
			cfg.RemoteBackup = value
		case "BACKUP_RETENTION_DAYS":
			cfg.BackupRetentionDays = value
		case "PRUNE_INTERVAL_DAYS":
			cfg.PruneIntervalDays = value
		case "DEBOUNCE_SECONDS":
			cfg.DebounceSeconds = value
		case "RCLONE_TRANSFERS":
			cfg.RcloneTransfers = value
		case "LOCKFILE":
			cfg.Lockfile = expandHome(value)
		case "WEBHOOK_URL":
			cfg.WebhookURL = value
		case "WEBHOOK_FILE":
			// Backward compatibility with older configs. The worker now reads WEBHOOK_URL directly.
		}
	}
	return cfg, Validate(cfg)
}

func InstallWithConfig(c Config, allowAdminPrompt bool) error {
	script, err := templateString("rclone_orbit.sh")
	if err != nil {
		return err
	}
	scriptPath := ScriptPathFor(c)
	scriptChanged := fileChanged(scriptPath, []byte(script))
	if err := writeFileIfChanged(scriptPath, []byte(script), 0o755); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(filepath.Dir(scriptPath), "rclone_orbit.sh"))
	switch runtime.GOOS {
	case "darwin":
		if err := installNewsyslogConfig(allowAdminPrompt); err != nil {
			return err
		}
		return installLaunchd(scriptPath, scriptChanged)
	case "linux":
		return installSystemd(scriptPath)
	default:
		return errors.New("install supports macOS and Linux only")
	}
}

func Uninstall() error {
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("launchctl", "bootout", "gui/"+fmt.Sprint(os.Getuid()), LaunchAgentPath()).Run() // #nosec G204 -- fixed launchctl command with current user's launchd domain and generated plist path.
		return os.RemoveAll(LaunchAgentPath())
	case "linux":
		_ = exec.Command("systemctl", "--user", "disable", "--now", "rclone-orbit.service").Run() // #nosec G204 -- fixed systemctl user-service command.
		return os.RemoveAll(SystemdPath())
	default:
		return errors.New("uninstall supports macOS and Linux only")
	}
}

func Status() string {
	switch runtime.GOOS {
	case "darwin":
		b, err := exec.Command("launchctl", "print", "gui/"+fmt.Sprint(os.Getuid())+"/com.user.rclone_orbit").CombinedOutput() // #nosec G204 -- fixed launchctl status query for current user's service.
		if err != nil {
			return strings.TrimSpace(string(b))
		}
		return strings.TrimSpace(string(b))
	case "linux":
		b, _ := exec.Command("systemctl", "--user", "status", "rclone-orbit.service", "--no-pager").CombinedOutput() // #nosec G204 -- fixed systemctl user-service status query.
		return strings.TrimSpace(string(b))
	default:
		return "status supports macOS and Linux only"
	}
}

func LogPaths() (string, string) {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Logs", "rclone_orbit.log"), filepath.Join(home, "Library", "Logs", "rclone_orbit_error.log")
}

func NewsyslogTempPath() string {
	return filepath.Join(os.TempDir(), "com.ojarosch.rclone-orbit.conf")
}

func NewsyslogSystemPath() string {
	return "/etc/newsyslog.d/com.ojarosch.rclone-orbit.conf"
}

func NewsyslogInstalled() bool {
	_, err := os.Stat(NewsyslogSystemPath())
	return err == nil
}

func SameConfig(a, b Config) bool {
	return configContent(a) == configContent(b)
}

func PostInstallNote() string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	if _, err := os.Stat(NewsyslogSystemPath()); err == nil {
		return "newsyslog installed: " + NewsyslogSystemPath()
	}
	return "newsyslog config staged: sudo cp " + NewsyslogTempPath() + " " + NewsyslogSystemPath()
}

func installNewsyslogConfig(allowAdminPrompt bool) error {
	home, _ := os.UserHomeDir()
	content := fmt.Sprintf(`# logfilename                                      [owner:group]  mode count size when  flags
%s ojarosch:staff 644  5     1000 * J
%s ojarosch:staff 644  5     1000 * J
`, filepath.Join(home, "Library", "Logs", "rclone_orbit.log"), filepath.Join(home, "Library", "Logs", "rclone_orbit_error.log"))
	tmp := NewsyslogTempPath()
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		return err
	}
	// ponytail: admin install is optional; if declined/unavailable, leave ready-to-copy file in /tmp.
	if exec.Command("sudo", "-n", "true").Run() == nil { // #nosec G204 -- fixed sudo availability probe.
		return exec.Command("sudo", "cp", tmp, NewsyslogSystemPath()).Run() // #nosec G204 -- fixed copy from generated temp config to fixed newsyslog path.
	}
	if !allowAdminPrompt {
		return nil
	}
	script := "cp " + strconv.Quote(tmp) + " " + strconv.Quote(NewsyslogSystemPath())
	return exec.Command("osascript", "-e", "do shell script "+strconv.Quote(script)+" with administrator privileges").Run() // #nosec G204 -- fixed AppleScript admin prompt for copying generated newsyslog config.
}

func installLaunchd(scriptPath string, forceRestart bool) error {
	home, _ := os.UserHomeDir()
	plist, err := templateString("launchd.plist")
	if err != nil {
		return err
	}
	plist = strings.ReplaceAll(plist, "__SCRIPT_PATH__", scriptPath)
	plist = strings.ReplaceAll(plist, "__HOME__", home)
	path := LaunchAgentPath()
	plistChanged := fileChanged(path, []byte(plist))
	if err := writeFileIfChanged(path, []byte(plist), 0o644); err != nil {
		return err
	}
	if !forceRestart && !plistChanged {
		return nil
	}
	uid := fmt.Sprint(os.Getuid())
	_ = exec.Command("launchctl", "bootout", "gui/"+uid, path).Run()                       // #nosec G204 -- fixed launchctl command with current user's launchd domain and generated plist path.
	if err := exec.Command("launchctl", "bootstrap", "gui/"+uid, path).Run(); err != nil { // #nosec G204 -- fixed launchctl command with current user's launchd domain and generated plist path.
		return err
	}
	return exec.Command("launchctl", "kickstart", "-k", "gui/"+uid+"/com.user.rclone_orbit").Run() // #nosec G204 -- fixed launchctl restart for current user's service.
}

func installSystemd(scriptPath string) error {
	service, err := templateString("systemd.service")
	if err != nil {
		return err
	}
	service = strings.ReplaceAll(service, "%h/Documents/Repos/rclone-orbit/bin/rclone_orbit.sh", scriptPath)
	path := SystemdPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(service), 0o600); err != nil {
		return err
	}
	if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil { // #nosec G204 -- fixed systemctl user-service command.
		return err
	}
	return exec.Command("systemctl", "--user", "enable", "--now", "rclone-orbit.service").Run() // #nosec G204 -- fixed systemctl user-service command.
}
