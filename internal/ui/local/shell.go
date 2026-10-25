package local

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

// shell.go owns the window frame: global toolbar, mode rail, panel mounting
// and the status bar. Panels are mounted here and must not mount each other.
// Modes map onto Pane values shared with the toolbar buttons:
// "commit"→"Status", "log"→"" (history), "worktree"→"Branches".

// UIState is layout-only UI state. Business data stays on App.
type UIState struct {
	SidebarWidth   float32
	InspectorWidth float32
}

func (a *App) View(c *ui.Context) {
	c.OnShortcut(ui.Cmd, ui.KeyR, a.Refresh)
	c.OnShortcut(ui.Cmd, ui.KeyO, a.Choose)
	c.OnShortcut(ui.Cmd, ui.Key1, func() { a.Pane = "Status" })
	c.OnShortcut(ui.Cmd, ui.Key2, func() { a.Pane = "" })
	c.OnShortcut(ui.Cmd, ui.Key3, func() { a.Pane = "Branches" })
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(8).Gap(8).Children(func() {
			ui.Text(c, "Git Viewer").Bold()
			if a.Workspace.FilePath != "" {
				ui.Text(c, pathLabel(a.Workspace.FilePath)).TextColor(c.Theme().TextMuted)
			}
			if a.Loading || a.DialogOpen {
				ui.Spinner(c).Label("Git operation in progress")
			}
			ui.Text(c, a.Message).TextColor(c.Theme().TextMuted).Grow(1).NoWrap()
			if host := a.Repository.SSHHost; host != "" {
				ui.Textf(c, "SSH · %s", host)
			}
			ui.Button(c, "Refresh").Disabled(a.Repository.Repository.RepositoryID == "").OnClick(a.Refresh)
			ui.Button(c, "Cancel").Disabled(!a.Loading).OnClick(a.Cancel)
			ui.Button(c, "Choose repository…").OnClick(a.Choose)
		})
		a.featureToolbar(c)
		if a.StoreWarning {
			ui.Text(c, "Recent repositories could not be read or saved. Check application configuration.").TextColor(c.Theme().Danger)
		}
		ui.Split(c, &a.SidebarWidth, func() { a.sidebar(c) }, func() {
			ui.Split(c, &a.StatusWidth, func() {
				ui.Row(c).Fill().Children(func() {
					a.modeRail(c)
					ui.Column(c).Fill().Children(func() { a.mainView(c) }).Grow(1)
				})
			}, func() { a.diffView(c) }).Fill()
		}).Grow(1).FillWidth()
		a.statusBar(c)
	})
}

// modeRail mounts the vertical mode switcher. a.Mode is derived from Pane so
// the rail, the toolbar buttons and the Cmd+1/2/3 shortcuts stay in sync.
func (a *App) modeRail(c *ui.Context) {
	// Sync the rail from Pane only when Pane changed outside the rail, so
	// toolbar buttons and shortcuts update the rail without fighting it.
	if a.Pane != a.railPane {
		a.railPane = a.Pane
		switch a.Pane {
		case "Status":
			a.Mode = "commit"
		case "Branches":
			a.Mode = "worktree"
		default:
			a.Mode = "log"
		}
	}
	changed := ui.Sidebar(c, &a.Mode, func() {
		ui.SidebarSection(c, "Workspace", nil, func() {
			ui.SidebarItem(c, "commit", nil, "Commit").Children(func() {
				if n := len(a.Entries); n > 0 {
					ui.Badge(c, fmt.Sprintf("%d", n))
				}
			})
			ui.SidebarItem(c, "log", nil, "Git Log")
			ui.SidebarItem(c, "worktree", nil, "Worktree")
		})
	}).Width(120).Changed()
	if changed {
		switch a.Mode {
		case "commit":
			a.Pane = "Status"
		case "worktree":
			a.Pane = "Branches"
		default:
			a.Pane = ""
		}
		a.railPane = a.Pane
	}
}
func (a *App) mainView(c *ui.Context) {
	if a.Pane == "Branches" {
		a.branchesView(c)
		return
	}
	if a.Pane == "Status" {
		a.statusView(c)
		return
	}
	a.historyView(c)
}
func (a *App) statusBar(c *ui.Context) {
	ui.Row(c).Padding(6).Gap(8).Children(func() {
		left := "No repository"
		if a.Repository.Repository.RepositoryID != "" {
			branch := ""
			for _, b := range a.Branches {
				if b.Current {
					branch = strings.TrimPrefix(b.Name, "refs/heads/")
					break
				}
			}
			if branch == "" {
				branch = "branches not loaded"
			}
			left = fmt.Sprintf("%s · %d changes", branch, len(a.Entries))
		}
		ui.Text(c, left).TextColor(c.Theme().TextMuted).Grow(1).NoWrap()
		if a.Stale {
			ui.Text(c, "Stale").TextColor(c.Theme().Danger)
		}
		if host := a.Repository.SSHHost; host != "" {
			ui.Textf(c, "SSH Connected · %s", host).TextColor(c.Theme().TextMuted)
		} else if a.Repository.Repository.RepositoryID != "" {
			ui.Text(c, "Local").TextColor(c.Theme().TextMuted)
		}
	})
}
