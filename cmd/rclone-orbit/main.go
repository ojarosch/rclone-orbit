package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ojarosch/rclone-orbit/internal/app"
)

type installDone struct{ err error }

type field struct {
	label   string
	input   textinput.Model
	options []string
}

type model struct {
	remotes []string
	remote  int
	fields  []field
	focus   int // 0 = remote selector, 1..n = fields
	spinner spinner.Model
	working bool
	done    bool
	msg     string
}

var title = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
var help = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
var errStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
var okStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))

func initialModel() model {
	cfg, err := app.LoadConfig()
	if err != nil {
		cfg = app.DefaultConfig()
	}
	remotes, _ := app.RcloneRemotes()
	remoteName := app.RemoteName(cfg.RemoteLive)
	if len(remotes) == 0 {
		remotes = []string{remoteName}
	}
	remoteIdx := 0
	for i, r := range remotes {
		if r == remoteName {
			remoteIdx = i
		}
	}
	fields := []field{
		newField("Local folder", cfg.LocalDir, nil),
		newField("Live path on remote", remotePathPart(cfg.RemoteLive, "sync"), nil),
		newField("Backup path on remote", remotePathPart(cfg.RemoteBackup, "backup"), nil),
		newField("Backup retention days", cfg.BackupRetentionDays, []string{"30", "60", "90", "180"}),
		newField("Prune interval days", cfg.PruneIntervalDays, []string{"1", "7", "14", "30"}),
		newField("Debounce seconds", cfg.DebounceSeconds, []string{"60", "300", "600", "1800"}),
		newField("Rclone transfers", cfg.RcloneTransfers, []string{"1", "2", "4", "8"}),
		newField("Discord webhook URL (optional)", cfg.WebhookURL, nil),
	}
	if runtime.GOOS == "darwin" {
		fields = append(fields, newField("Install macOS log rotation with admin prompt?", "yes", []string{"yes", "no"}))
	}
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	return model{remotes: remotes, remote: remoteIdx, fields: fields, spinner: spin}
}

func newField(label, value string, options []string) field {
	in := textinput.New()
	in.Prompt = label + ": "
	in.SetValue(value)
	in.CharLimit = 256
	return field{label: label, input: in, options: options}
}

func remotePathPart(path, fallback string) string {
	_, rest, ok := strings.Cut(path, ":")
	if !ok || rest == "" {
		return fallback
	}
	return rest
}

func (m model) Init() tea.Cmd { return textinput.Blink }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.working {
		switch msg := msg.(type) {
		case installDone:
			m.working = false
			if msg.err != nil {
				m.msg = "install failed: " + msg.err.Error()
				return m, nil
			}
			m.done = true
			m.msg = "installed. config: " + app.ConfigPath()
			if note := app.PostInstallNote(); note != "" {
				m.msg += "\n" + note
			}
			return m, tea.Quit
		case tea.KeyMsg:
			if msg.String() == "ctrl+c" || msg.String() == "esc" {
				return m, tea.Quit
			}
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "enter", "tab", "down", "j":
			if m.focus < len(m.fields) {
				return m.focusNext(), nil
			}
			return m.startInstall()
		case "shift+tab", "up", "k":
			return m.focusPrev(), nil
		case "left", "h":
			return m.cycleOption(-1), nil
		case "right", "l":
			return m.cycleOption(1), nil
		}
	}

	if m.focus > 0 {
		var cmd tea.Cmd
		idx := m.focus - 1
		m.fields[idx].input, cmd = m.fields[idx].input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) startInstall() (tea.Model, tea.Cmd) {
	cfg := m.config()
	allowAdminPrompt := false
	if runtime.GOOS == "darwin" {
		allowAdminPrompt = yes(m.fields[len(m.fields)-1].input.Value())
	}
	if err := app.Validate(cfg); err != nil {
		m.msg = "config failed: " + err.Error()
		return m, nil
	}
	if current, err := app.LoadConfig(); err == nil && app.SameConfig(current, cfg) && (!allowAdminPrompt || app.NewsyslogInstalled()) {
		m.done = true
		m.msg = "no changes. config: " + app.ConfigPath()
		return m, tea.Quit
	}
	m.working = true
	m.msg = "writing config and installing service..."
	return m, tea.Batch(m.spinner.Tick, installCmd(cfg, allowAdminPrompt))
}

func (m model) config() app.Config {
	v := make([]string, 8)
	v[0] = m.fields[0].input.Value()
	remote := m.remotes[m.remote]
	v[1] = remote + ":" + strings.TrimPrefix(m.fields[1].input.Value(), ":")
	v[2] = remote + ":" + strings.TrimPrefix(m.fields[2].input.Value(), ":")
	for i := 3; i <= 7; i++ {
		v[i] = m.fields[i].input.Value()
	}
	return app.ConfigFromValues(v)
}

func installCmd(cfg app.Config, allowAdminPrompt bool) tea.Cmd {
	return func() tea.Msg {
		if err := app.WriteConfig(cfg); err != nil {
			return installDone{err: err}
		}
		return installDone{err: app.InstallWithConfig(cfg, allowAdminPrompt)}
	}
}

func (m model) focusNext() model {
	if m.focus > 0 {
		m.fields[m.focus-1].input.Blur()
	}
	m.focus = (m.focus + 1) % (len(m.fields) + 1)
	if m.focus > 0 {
		m.fields[m.focus-1].input.Focus()
	}
	m.msg = ""
	return m
}

func (m model) focusPrev() model {
	if m.focus > 0 {
		m.fields[m.focus-1].input.Blur()
	}
	m.focus--
	if m.focus < 0 {
		m.focus = len(m.fields)
	}
	if m.focus > 0 {
		m.fields[m.focus-1].input.Focus()
	}
	m.msg = ""
	return m
}

func (m model) cycleOption(delta int) model {
	if m.focus == 0 {
		m.remote = (m.remote + delta + len(m.remotes)) % len(m.remotes)
		return m
	}
	f := &m.fields[m.focus-1]
	if len(f.options) == 0 {
		return m
	}
	cur := strings.TrimSpace(f.input.Value())
	idx := -1
	for i, opt := range f.options {
		if opt == cur {
			idx = i
		}
	}
	if idx < 0 {
		idx = 0
	} else {
		idx = (idx + delta + len(f.options)) % len(f.options)
	}
	f.input.SetValue(f.options[idx])
	return m
}

func yes(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "" || s == "y" || s == "yes" || s == "true" || s == "1"
}

func (m model) View() string {
	if m.done {
		return okStyle.Render(m.msg) + "\n"
	}
	var b strings.Builder
	b.WriteString(title.Render("rclone-orbit setup") + "\n\n")
	for _, line := range app.Doctor() {
		b.WriteString(line + "\n")
	}
	remote := m.remotes[m.remote]
	b.WriteString("\n")
	remoteLine := "Rclone remote: " + remote
	if m.focus == 0 {
		remoteLine = "> " + remoteLine + help.Render("  ←/→ "+strings.Join(m.remotes, ", "))
	} else {
		remoteLine = "  " + remoteLine
	}
	b.WriteString(remoteLine + "\n")
	b.WriteString("  Live target preview: " + remote + ":" + strings.TrimPrefix(m.fields[1].input.Value(), ":") + "\n")
	b.WriteString("  Backup target preview: " + remote + ":" + strings.TrimPrefix(m.fields[2].input.Value(), ":") + "\n\n")
	for i := range m.fields {
		b.WriteString(m.fields[i].input.View())
		if len(m.fields[i].options) > 0 && m.focus == i+1 {
			b.WriteString(help.Render("  ←/→ presets: " + strings.Join(m.fields[i].options, ", ")))
		}
		b.WriteString("\n")
	}
	if m.msg != "" {
		if m.working {
			b.WriteString("\n" + m.spinner.View() + " " + m.msg + "\n")
		} else {
			b.WriteString("\n" + errStyle.Render(m.msg) + "\n")
		}
	}
	b.WriteString("\n" + help.Render("tab/down next • up previous • ←/→ choose/presets • enter install on final field • esc quit") + "\n")
	return b.String()
}

func main() {
	cmd := "wizard"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "wizard":
		_, err := tea.NewProgram(initialModel()).Run()
		fatal(err)
	case "doctor":
		for _, line := range app.Doctor() {
			fmt.Println(line)
		}
	case "install":
		fatal(app.Install())
		fmt.Println("installed")
		if note := app.PostInstallNote(); note != "" {
			fmt.Println(note)
		}
	case "uninstall":
		fatal(app.Uninstall())
		fmt.Println("uninstalled")
	case "status":
		fmt.Println(app.Status())
	case "logs":
		out, errLog := app.LogPaths()
		fmt.Println(out)
		fmt.Println(errLog)
	case "write-config":
		fatal(app.WriteConfig(app.DefaultConfig()))
		fmt.Println(app.ConfigPath())
	default:
		fmt.Println("usage: rclone-orbit [wizard|doctor|install|uninstall|status|logs|write-config]")
	}
}

func fatal(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
