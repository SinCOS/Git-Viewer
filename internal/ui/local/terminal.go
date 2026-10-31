package local

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

type terminalSession struct {
	term     *terminal.Terminal
	cwd      string
	fontSize float32
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

func (a *App) ToggleTerminal() {
	if a.TerminalOpen {
		a.TerminalOpen = false
		return
	}
	a.TerminalOpen = true
	a.ensureTerminal()
}

func (a *App) ensureTerminal() {
	cwd := a.localTerminalCwd()
	fontSize := float32(a.Settings.TerminalFontSize)
	if fontSize <= 0 {
		fontSize = 13
	}
	if a.terminal != nil && a.terminal.cwd == cwd {
		if a.terminal.fontSize != fontSize {
			a.terminal.term.SetFont(terminal.Font{Size: fontSize})
			a.terminal.fontSize = fontSize
		}
		return
	}
	if a.TerminalLoading {
		return
	}
	if a.terminal != nil {
		a.terminal.term.Close()
		a.terminal = nil
	}
	a.TerminalLoading = true
	a.TerminalError = ""
	dispatch := a.Dispatch
	if dispatch == nil {
		a.TerminalLoading = false
		a.TerminalError = "Terminal is unavailable before the window is ready"
		return
	}
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		session, err := terminal.New(terminal.Options{
			Dir:  cwd,
			Font: terminal.Font{Size: fontSize},
			OnExit: func(code int) {
				dispatch(func() {
					if a.closed {
						return
					}
					if code == 0 {
						a.TerminalError = "Shell exited"
					} else {
						a.TerminalError = fmt.Sprintf("Shell exited with code %d", code)
					}
				})
			},
		})
		dispatch(func() {
			if a.closed {
				if session != nil {
					session.Close()
				}
				return
			}
			a.TerminalLoading = false
			if err != nil {
				a.TerminalError = "Terminal unavailable: " + err.Error()
				return
			}
			a.terminal = &terminalSession{term: session, cwd: cwd, fontSize: fontSize}
			a.TerminalError = ""
		})
	}()
}

func (a *App) closeTerminal() {
	if a.terminal != nil {
		a.terminal.term.Close()
		a.terminal = nil
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
		ui.Row(c).Height(30).FillWidth().Padding(0, 8).Gap(6).AlignItems(ui.Center).
			Background(c.Theme().Surface).BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Children(func() {
			ui.Icon(c, terminalIcon).Size(14, 14).TextColor(c.Theme().TextMuted)
			ui.Text(c, "TERMINAL").Bold().FontSize(10).TextColor(c.Theme().TextMuted)
			cwd := a.localTerminalCwd()
			if home, err := os.UserHomeDir(); err == nil && home != "" {
				if rel, err := filepath.Rel(home, cwd); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					cwd = "~/" + filepath.ToSlash(rel)
				}
			}
			ui.Text(c, cwd).FontSize(10).TextColor(c.Theme().TextMuted).NoWrap().Grow(1)
			ui.Button(c, "Restart").Height(22).OnClick(func() {
				a.closeTerminal()
				a.ensureTerminal()
			})
			ui.Button(c, "Close").Height(22).OnClick(func() { a.TerminalOpen = false })
		})
		switch {
		case a.TerminalLoading:
			ui.Column(c).Grow(1).FillWidth().Center().Gap(6).Children(func() {
				ui.Text(c, "Starting terminal…").FontSize(12).TextColor(c.Theme().TextMuted)
			})
		case a.TerminalError != "" && a.terminal == nil:
			ui.Column(c).Grow(1).FillWidth().Center().Gap(8).Children(func() {
				ui.Text(c, a.TerminalError).FontSize(12).TextColor(c.Theme().Danger)
				ui.Button(c, "Retry").Height(26).OnClick(func() {
					a.TerminalError = ""
					a.ensureTerminal()
				})
			})
		case a.terminal != nil:
			if a.terminal.fontSize != float32(a.Settings.TerminalFontSize) && a.Settings.TerminalFontSize > 0 {
				a.terminal.term.SetFont(terminal.Font{Size: float32(a.Settings.TerminalFontSize)})
				a.terminal.fontSize = float32(a.Settings.TerminalFontSize)
			}
			terminal.View(c, a.terminal.term).Grow(1).FillWidth().AutoFocus()
		default:
			ui.Column(c).Grow(1).FillWidth().Center().Children(func() {
				ui.Button(c, "Start Terminal").Height(28).OnClick(a.ensureTerminal)
			})
		}
	})
}
