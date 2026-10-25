package local

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
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
		ui.Split(c, &a.SidebarWidth, func() { a.modeExplorer(c) }, func() {
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

// modeExplorer mounts the sidebar content for the active mode, as IDEA swaps
// tool-window sidebars: the commit form in Commit mode, nothing in Git Log
// (its filters live in the log header), and worktree/branch sections in
// Worktree mode.
func (a *App) modeExplorer(c *ui.Context) {
	switch a.Mode {
	case "log":
		ui.Column(c).Fill().Children(func() {})
	case "worktree":
		ui.Scroll(c).Fill().Children(func() {
			ui.Column(c).FillWidth().Padding(8).Gap(2).Children(func() {
				a.worktreeSections(c)
			})
		})
	default:
		ui.Column(c).Fill().Children(func() {
			a.commitSidebar(c)
		})
	}
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
			ui.SidebarItem(c, "commit", commitIcon, "Commit").Children(func() {
				if n := len(a.Entries); n > 0 {
					ui.Badge(c, fmt.Sprintf("%d", n))
				}
			})
			ui.SidebarItem(c, "log", logIcon, "Git Log")
			ui.SidebarItem(c, "worktree", wtIcon, "Worktree")
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
	// The outer Split pairs this main pane with the Diff inspector; modes
	// only swap the left side, as IDEA swaps tool-window content.
	switch a.Mode {
	case "worktree":
		a.branchesView(c)
		return
	case "log":
		a.historyView(c)
		return
	default:
		// IDEA Commit tool window: Local Changes fills the main pane; the
		// commit form lives in the explorer sidebar beside it.
		ui.Column(c).FillWidth().Children(func() { a.statusView(c) }).Grow(1)
	}
}
func (a *App) statusBar(c *ui.Context) {
	ui.Row(c).Padding(6).Gap(8).Children(func() {
		if a.Repository.Repository.RepositoryID == "" {
			ui.Text(c, "No repository").TextColor(c.Theme().TextMuted).Grow(1).NoWrap()
		} else {
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
			// IDEA-style branch widget: the current branch opens a popover
			// listing branches; choosing one goes through the safe switch flow.
			ui.Popover(c, ui.Textf(c, "⎇ %s ▾", branch).TextColor(c.Theme().TextMuted), &a.BranchPopOpen, func() {
				ui.Column(c).Width(240).Padding(8).Gap(2).Children(func() {
					if len(a.Branches) == 0 {
						ui.Text(c, "branches not loaded")
					}
					for _, b := range a.Branches {
						name := strings.TrimPrefix(b.Name, "refs/heads/")
						label := name
						if b.Current {
							label = "✓ " + name
						}
						ui.Text(c, label).OnClick(func() {
							a.BranchInput = name
							a.RunWrite(domain.SwitchBranch)
						})
					}
				})
			})
			ui.Textf(c, "· %d changes", len(a.Entries)).TextColor(c.Theme().TextMuted)
		}
		ui.Text(c, "").Grow(1)
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
