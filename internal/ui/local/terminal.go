package local

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
	"gitviewer/internal/infrastructure/ssh"
	"gitviewer/internal/infrastructure/storage"
)

const maxTerminalSnapshot = 256 << 10

var (
	terminalRuntimeLoad    = terminal.Load
	terminalRuntimeTimeout = 10 * time.Second
)

type terminalTab struct {
	ID       string
	Kind     string
	Host     string
	Cwd      string
	Title    string
	Snapshot []byte
	term     *terminal.Terminal
	fontSize float32
	loading  bool
	err      string
}

type terminalSessionRecord struct {
	Kind     string `json:"kind"`
	Host     string `json:"host,omitempty"`
	Cwd      string `json:"cwd,omitempty"`
	Title    string `json:"title,omitempty"`
	Snapshot []byte `json:"snapshot,omitempty"`
}

type terminalSessionFile struct {
	Version int                     `json:"version"`
	Active  int                     `json:"active"`
	Tabs    []terminalSessionRecord `json:"tabs"`
}

func (a *App) localTerminalCwd() string {
	candidate := a.FolderPath
	if candidate == "" && a.Repository.Repository.Location == "local" {
		candidate = a.Repository.Repository.Path
	}
	if candidate != "" {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		return home
	}
	return "."
}

func (a *App) activeTerminal() *terminalTab {
	if a.TerminalActive < 0 || a.TerminalActive >= len(a.TerminalTabs) {
		return nil
	}
	return a.TerminalTabs[a.TerminalActive]
}

func (a *App) syncActiveTerminalState() {
	tab := a.activeTerminal()
	if tab == nil {
		a.TerminalLoading = false
		a.TerminalError = ""
		return
	}
	a.TerminalLoading = tab.loading
	a.TerminalError = tab.err
}

func (a *App) ToggleTerminal() {
	if a.TerminalOpen {
		a.TerminalOpen = false
		a.saveTerminalSessions()
		return
	}
	a.TerminalOpen = true
	if len(a.TerminalTabs) == 0 {
		a.NewLocalTerminal()
		return
	}
	if a.TerminalActive < 0 || a.TerminalActive >= len(a.TerminalTabs) {
		a.TerminalActive = 0
	}
	a.ensureTerminal()
}

func (a *App) newTerminal(kind, host, cwd, title string, snapshot []byte) *terminalTab {
	a.terminalCounter++
	prefix := kind
	if kind == "remote" {
		prefix = "ssh"
	}
	tab := &terminalTab{
		ID:       fmt.Sprintf("%s-%d", prefix, a.terminalCounter),
		Kind:     kind,
		Host:     host,
		Cwd:      cwd,
		Title:    title,
		Snapshot: append([]byte(nil), snapshot...),
	}
	a.TerminalTabs = append(a.TerminalTabs, tab)
	a.TerminalActive = len(a.TerminalTabs) - 1
	a.syncActiveTerminalState()
	return tab
}

// NewLocalTerminal opens a local login shell tab in the active folder.
func (a *App) NewLocalTerminal() {
	a.TerminalOpen = true
	a.newTerminal("local", "", a.localTerminalCwd(), "Local", nil)
	a.ensureTerminal()
}

// NewRemoteTerminal opens a real interactive OpenSSH shell. It deliberately
// accepts only SSH config aliases and never a free-form command string.
func (a *App) NewRemoteTerminal(host string) {
	host = strings.TrimSpace(host)
	if !ssh.ValidHost(host) {
		a.TerminalError = "Choose a valid SSH host alias"
		return
	}
	a.TerminalOpen = true
	a.newTerminal("remote", host, "", host, nil)
	a.ensureTerminal()
}

func (a *App) SelectTerminalTab(index int) {
	if index < 0 || index >= len(a.TerminalTabs) {
		return
	}
	a.TerminalActive = index
	a.syncActiveTerminalState()
	a.ensureTerminal()
}

func (a *App) NextTerminalTab() {
	if len(a.TerminalTabs) < 2 {
		return
	}
	a.SelectTerminalTab((a.TerminalActive + 1) % len(a.TerminalTabs))
}

func (a *App) PreviousTerminalTab() {
	if len(a.TerminalTabs) < 2 {
		return
	}
	a.SelectTerminalTab((a.TerminalActive - 1 + len(a.TerminalTabs)) % len(a.TerminalTabs))
}

func (a *App) ensureTerminal() {
	if tab := a.activeTerminal(); tab != nil {
		a.ensureTerminalTab(tab)
	}
}

func (a *App) ensureTerminalTab(tab *terminalTab) {
	if tab == nil || tab.term != nil || tab.loading {
		return
	}
	fontSize := float32(a.Settings.TerminalFontSize)
	if fontSize <= 0 {
		fontSize = 13
	}
	dispatch := a.Dispatch
	if dispatch == nil {
		tab.loading = false
		tab.err = "Terminal is unavailable before the window is ready"
		a.syncActiveTerminalState()
		return
	}
	options, err := a.terminalOptions(tab, fontSize)
	if err != nil {
		tab.err = err.Error()
		a.syncActiveTerminalState()
		return
	}
	tab.loading = true
	tab.err = ""
	a.syncActiveTerminalState()
	id := tab.ID
	remote, host, executor := tab.Kind == "remote", tab.Host, a.Service.Remote
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		if remote {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			args, err := executor.SessionArgs(ctx, host, ssh.TerminalSession)
			cancel()
			if err != nil {
				dispatch(func() {
					if index := a.terminalTabIndex(id); index >= 0 {
						a.TerminalTabs[index].loading = false
						a.TerminalTabs[index].err = message(err)
						a.syncActiveTerminalState()
					}
				})
				return
			}
			options.Command = append([]string{options.Command[0]}, append(args, host)...)
		}
		if loadErr := loadTerminalRuntime(); loadErr != nil {
			dispatch(func() {
				index := a.terminalTabIndex(id)
				if index < 0 {
					return
				}
				current := a.TerminalTabs[index]
				current.loading = false
				current.err = "Terminal unavailable: " + loadErr.Error()
				a.syncActiveTerminalState()
				a.saveTerminalSessions()
			})
			return
		}
		session, startErr := terminal.New(options)
		dispatch(func() {
			index := a.terminalTabIndex(id)
			if index < 0 {
				if session != nil {
					session.Close()
				}
				return
			}
			current := a.TerminalTabs[index]
			current.loading = false
			if startErr != nil {
				current.err = "Terminal unavailable: " + startErr.Error()
				a.syncActiveTerminalState()
				a.saveTerminalSessions()
				return
			}
			current.term = session
			current.fontSize = fontSize
			if len(current.Snapshot) > 0 {
				current.term.Feed(current.Snapshot)
			}
			current.err = ""
			a.syncActiveTerminalState()
		})
	}()
}

func loadTerminalRuntime() error {
	result := make(chan error, 1)
	go func() {
		result <- terminalRuntimeLoad()
	}()
	select {
	case err := <-result:
		return err
	case <-time.After(terminalRuntimeTimeout):
		return fmt.Errorf("the terminal runtime is still downloading; retry after libghostty-vt finishes loading")
	}
}

func (a *App) terminalOptions(tab *terminalTab, fontSize float32) (terminal.Options, error) {
	onExit := func(code int) {
		dispatch := a.Dispatch
		if dispatch == nil {
			return
		}
		dispatch(func() {
			index := a.terminalTabIndex(tab.ID)
			if index < 0 || a.closed {
				return
			}
			current := a.TerminalTabs[index]
			if code == 0 {
				current.err = "Shell exited"
			} else {
				current.err = fmt.Sprintf("Shell exited with code %d", code)
			}
			a.syncActiveTerminalState()
			a.saveTerminalSessions()
		})
	}
	onTitle := func(title string) {
		dispatch := a.Dispatch
		if dispatch == nil {
			return
		}
		title = terminalTitle(title)
		if title == "" {
			return
		}
		dispatch(func() {
			if index := a.terminalTabIndex(tab.ID); index >= 0 {
				a.TerminalTabs[index].Title = title
			}
		})
	}
	options := terminal.Options{
		Font:    terminal.Font{Size: fontSize},
		OnExit:  onExit,
		OnTitle: onTitle,
	}
	if tab.Kind == "remote" {
		command, err := a.remoteTerminalCommand(tab.Host)
		if err != nil {
			return options, err
		}
		options.Command = command
		home, _ := os.UserHomeDir()
		options.Dir = home
		return options, nil
	}
	cwd := tab.Cwd
	if cwd == "" {
		cwd = a.localTerminalCwd()
		tab.Cwd = cwd
	}
	if info, err := os.Stat(cwd); err != nil || !info.IsDir() {
		cwd = a.localTerminalCwd()
		tab.Cwd = cwd
	}
	options.Dir = cwd
	return options, nil
}

func terminalTitle(title string) string {
	title = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(title, "\x00", ""), "\n", " "))
	if len(title) > 48 {
		title = title[:48]
	}
	return title
}

func (a *App) remoteTerminalCommand(host string) ([]string, error) {
	if !ssh.ValidHost(host) {
		return nil, fmt.Errorf("Choose a valid SSH host alias")
	}
	executor := a.Service.Remote
	// This UI-thread placeholder does no authentication. The worker obtains
	// TerminalSession arguments before starting the PTY.
	executable := executor.SSHPath
	if executable == "" {
		executable = "ssh"
	}
	return []string{executable, host}, nil
}

func (a *App) terminalTabIndex(id string) int {
	for i, tab := range a.TerminalTabs {
		if tab.ID == id {
			return i
		}
	}
	return -1
}

func (a *App) RestartTerminal() {
	tab := a.activeTerminal()
	if tab == nil {
		return
	}
	tab.Snapshot = nil
	if tab.term != nil {
		tab.term.Close()
		tab.term = nil
	}
	tab.err = ""
	tab.loading = false
	a.ensureTerminalTab(tab)
}

func (a *App) closeTerminalTab(index int) {
	if index < 0 || index >= len(a.TerminalTabs) {
		return
	}
	tab := a.TerminalTabs[index]
	if tab.term != nil {
		tab.Snapshot = boundedSnapshot(tab.term.Snapshot())
		tab.term.Close()
		tab.term = nil
	}
	a.TerminalTabs = append(a.TerminalTabs[:index], a.TerminalTabs[index+1:]...)
	if len(a.TerminalTabs) == 0 {
		a.TerminalActive = -1
	} else if a.TerminalActive > index || a.TerminalActive >= len(a.TerminalTabs) {
		a.TerminalActive--
	}
	if a.TerminalActive < 0 && len(a.TerminalTabs) > 0 {
		a.TerminalActive = 0
	}
	a.syncActiveTerminalState()
	a.saveTerminalSessions()
}

func (a *App) CloseActiveTerminal() {
	a.closeTerminalTab(a.TerminalActive)
	if tab := a.activeTerminal(); tab != nil {
		a.ensureTerminalTab(tab)
	}
}

func (a *App) closeTerminal() {
	a.saveTerminalSessions()
	for _, tab := range a.TerminalTabs {
		if tab.term != nil {
			tab.term.Close()
			tab.term = nil
		}
	}
	a.TerminalLoading = false
}

func (a *App) terminalView(c *ui.Context) {
	height := a.TerminalHeight
	if height < 140 {
		height = 140
	}
	if height > 520 {
		height = 520
	}
	ui.Column(c).Label("Integrated terminal").FillWidth().Height(height).Shrink(0).
		Background(c.Theme().Background).BorderWidth(1, 0, 0, 0).BorderColor(c.Theme().Border).Children(func() {
		ui.Row(c).Height(32).FillWidth().Padding(0, 8).Gap(6).AlignItems(ui.Center).
			Background(c.Theme().Surface).BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Children(func() {
			ui.Icon(c, terminalIcon).Size(14, 14).TextColor(c.Theme().TextMuted)
			ui.Text(c, "TERMINAL").Bold().FontSize(10).TextColor(c.Theme().TextMuted)
			ui.Row(c).Grow(1).MinWidth(0).Height(28).Gap(2).Clip().Children(func() {
				for i, tab := range a.TerminalTabs {
					i, tab := i, tab
					label := tab.Title
					if label == "" {
						label = tab.Kind
					}
					item := ui.ButtonBase(c).Label("Terminal "+label).Tooltip(label).Height(24).MaxWidth(180).
						MinWidth(70).Padding(0, 6).Gap(4).Radius(3).Cursor(ui.CursorPointer).
						OnClick(func() { a.SelectTerminalTab(i) })
					if i == a.TerminalActive {
						item.Background(c.Theme().Background).TextColor(c.Theme().Text)
					}
					item.ContextMenu(func(m *ui.Menu) {
						if m.Item("Close terminal").Chosen() {
							a.closeTerminalTab(i)
						}
						if m.Item("Restart terminal").Chosen() {
							a.SelectTerminalTab(i)
							a.RestartTerminal()
						}
					})
					item.Children(func() {
						ui.Icon(c, terminalIcon).Size(12, 12).Shrink(0).TextColor(c.Theme().TextMuted)
						ui.Text(c, label).FontSize(11).NoWrap().Grow(1).MinWidth(0)
					})
				}
			})
			newTerminal := ui.Button(c, "").Label("New terminal").Tooltip("New terminal").
				Size(38, 22).Padding(0, 4).Gap(2).Radius(3).BorderWidth(0).
				Background(c.Theme().Surface).TextColor(c.Theme().TextMuted).
				AlignItems(ui.Center).Justify(ui.Center).Shrink(0).Cursor(ui.CursorPointer)
			newTerminal.Children(func() {
				ui.Icon(c, plusIcon).Size(14, 14).Shrink(0)
				ui.Icon(c, chevronDownIcon).Size(10, 10).Shrink(0)
			})
			newTerminal.Menu(func(m *ui.Menu) {
				if m.Item("New Terminal").Chosen() {
					a.NewLocalTerminal()
				}
				if len(a.SSHHosts) == 0 {
					if m.Item("New SSH Terminal…").Chosen() {
						a.OpenRemoteDialog()
					}
				} else {
					m.Submenu("New SSH Terminal", func(m *ui.Menu) {
						for _, host := range a.SSHHosts {
							host := host
							if m.Item(host).Chosen() {
								a.NewRemoteTerminal(host)
							}
						}
					})
				}
			})
			iconButton(c, "Restart terminal", "Restart terminal", refreshIcon).
				Size(22, 22).Radius(3).BorderWidth(0).Background(c.Theme().Surface).
				TextColor(c.Theme().TextMuted).Disabled(a.activeTerminal() == nil).OnClick(a.RestartTerminal)
			iconButton(c, "Close terminal panel", "Close terminal panel", panelCloseIcon).
				Size(22, 22).Radius(3).BorderWidth(0).Background(c.Theme().Surface).
				TextColor(c.Theme().TextMuted).OnClick(func() {
				a.TerminalOpen = false
				a.saveTerminalSessions()
			})
		})
		tab := a.activeTerminal()
		switch {
		case tab == nil:
			ui.Column(c).Grow(1).FillWidth().Center().Gap(8).Children(func() {
				ui.Text(c, "No terminal session").FontSize(12).TextColor(c.Theme().TextMuted)
				ui.Button(c, "New Terminal").Height(26).OnClick(a.NewLocalTerminal)
			})
		case tab.loading:
			ui.Column(c).Grow(1).FillWidth().Center().Children(func() {
				ui.Text(c, "Starting terminal…").FontSize(12).TextColor(c.Theme().TextMuted)
			})
		case tab.err != "" && tab.term == nil:
			ui.Column(c).Grow(1).FillWidth().Center().Gap(8).Children(func() {
				ui.Text(c, tab.err).FontSize(12).TextColor(c.Theme().Danger)
				ui.Button(c, "Retry").Height(26).OnClick(a.RestartTerminal)
			})
		case tab.term != nil:
			if tab.fontSize != float32(a.Settings.TerminalFontSize) && a.Settings.TerminalFontSize > 0 {
				tab.term.SetFont(terminal.Font{Size: float32(a.Settings.TerminalFontSize)})
				tab.fontSize = float32(a.Settings.TerminalFontSize)
			}
			terminal.View(c, tab.term).Grow(1).FillWidth().AutoFocus()
		default:
			ui.Column(c).Grow(1).FillWidth().Center().Children(func() {
				ui.Button(c, "Start Terminal").Height(28).OnClick(a.ensureTerminal)
			})
		}
	})
}

func (a *App) terminalSessionsPath() string {
	base := a.Service.Recent.Path
	if base == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(base), "terminal-sessions.json")
}

func (a *App) loadTerminalSessions() {
	path := a.terminalSessionsPath()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var saved terminalSessionFile
	if json.Unmarshal(data, &saved) != nil || saved.Version != 1 || len(saved.Tabs) > 20 {
		return
	}
	for _, record := range saved.Tabs {
		if record.Kind != "local" && record.Kind != "remote" {
			continue
		}
		if record.Kind == "remote" && !ssh.ValidHost(record.Host) {
			continue
		}
		if record.Kind == "local" && record.Cwd != "" {
			if info, err := os.Stat(record.Cwd); err != nil || !info.IsDir() {
				record.Cwd = a.localTerminalCwd()
			}
		}
		snapshot := record.Snapshot
		if len(snapshot) > maxTerminalSnapshot {
			snapshot = snapshot[len(snapshot)-maxTerminalSnapshot:]
		}
		a.newTerminal(record.Kind, record.Host, record.Cwd, record.Title, snapshot)
	}
	if len(a.TerminalTabs) > 0 {
		a.TerminalActive = min(max(saved.Active, 0), len(a.TerminalTabs)-1)
	}
	a.syncActiveTerminalState()
}

func (a *App) saveTerminalSessions() {
	path := a.terminalSessionsPath()
	if path == "" {
		return
	}
	saved := terminalSessionFile{Version: 1, Active: max(a.TerminalActive, 0), Tabs: make([]terminalSessionRecord, 0, len(a.TerminalTabs))}
	for _, tab := range a.TerminalTabs {
		snapshot := tab.Snapshot
		if tab.term != nil {
			snapshot = tab.term.Snapshot()
		}
		snapshot = boundedSnapshot(snapshot)
		saved.Tabs = append(saved.Tabs, terminalSessionRecord{
			Kind: tab.Kind, Host: tab.Host, Cwd: tab.Cwd, Title: tab.Title, Snapshot: snapshot,
		})
	}
	_ = storage.SaveJSON(path, saved)
}

func boundedSnapshot(snapshot []byte) []byte {
	if len(snapshot) <= maxTerminalSnapshot {
		return append([]byte(nil), snapshot...)
	}
	start := len(snapshot) - maxTerminalSnapshot
	for start < len(snapshot) && snapshot[start] != 0x1b {
		start++
	}
	if start >= len(snapshot) {
		return nil
	}
	return append([]byte(nil), snapshot[start:]...)
}
