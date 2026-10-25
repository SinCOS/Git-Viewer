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
	a.syncMode()
	a.ensureHistory()
	ui.Column(c).Fill().Children(func() {
		a.featureToolbar(c)
		if a.StoreWarning {
			ui.Text(c, "Recent repositories could not be read or saved. Check application configuration.").TextColor(c.Theme().Danger)
		}
		ui.Row(c).Label("Workspace body").Grow(1).FillWidth().Children(func() {
			a.modeRail(c)
			ui.Column(c).Fill().Children(func() { a.mainView(c) }).Grow(1)
		})
		a.statusBar(c)
	})
}

// syncMode derives the active mode before any panels are mounted.
func (a *App) syncMode() {
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
}

func (a *App) modeRail(c *ui.Context) {
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
	}).Label("Mode navigation").Width(180).FillHeight().Changed()
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
		a.ensureHistory()
	}
}
func (a *App) mainView(c *ui.Context) {
	a.workspaceReposBar(c)
	switch a.Mode {
	case "worktree":
		a.branchesView(c)
	case "log":
		ui.Split(c, &a.BranchTreeWidth, func() { a.branchNavigation(c) }, func() {
			a.historyWorkspace(c)
		}).Fill()
	default:
		ui.Column(c).Fill().Children(func() {
			ui.Text(c, "Commit").Bold().FontSize(16).Padding(14)
			ui.Split(c, &a.StatusWidth, func() { a.statusView(c) }, func() { a.diffView(c) }).Grow(1).FillWidth()
			a.commitSidebar(c)
		})
	}
}

// workspaceReposBar renders the multi-root workspace like VS Code: one
// switcher row listing every repository in the workspace with its live change
// count, the open one marked. Hidden for single-repository workspaces.
func (a *App) workspaceReposBar(c *ui.Context) {
	if len(a.WorkspaceRepositories) < 2 {
		return
	}
	ui.Row(c).Label("Workspace repositories").Padding(4, 8).Gap(6).Children(func() {
		for i, repo := range a.WorkspaceRepositories {
			current := a.currentRepositoryPath() == repo.Repository.Path
			ui.Row(c).Gap(4).Children(func() {
				ui.Button(c.Key(i), repo.Repository.DisplayName).Tooltip(repo.Repository.Path).OnClick(func() { a.Open(repo.Repository.Path) })
				if current {
					ui.Text(c, "●").TextColor(c.Theme().Accent)
				}
				if n, ok := a.workspaceChanges[repo.Repository.RepositoryID]; ok && n >= 0 {
					ui.Badge(c, fmt.Sprintf("%d", n))
				}
			})
		}
		iconButton(c, "Refresh workspace statuses", "Refresh workspace statuses", refreshIcon).
			Disabled(a.Writing || a.DialogOpen).OnClick(func() { a.loadWorkspaceStatuses(false) })
	})
}
func (a *App) statusBar(c *ui.Context) {
	// Fixed height so the spinner never resizes the bar and shifts the panes above.
	ui.Row(c).Height(28).Padding(6).Gap(8).Children(func() {
		if a.Repository.Repository.RepositoryID == "" {
			ui.Text(c, "No repository").TextColor(c.Theme().TextMuted).NoWrap()
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
			ui.Textf(c, "⎇ %s", branch).TextColor(c.Theme().TextMuted)
			ui.Textf(c, "· %d changes", len(a.Entries)).TextColor(c.Theme().TextMuted)
		}
		if a.Loading && !a.quietLoading || a.DialogOpen {
			ui.Spinner(c).Label("Git operation in progress").Size(16, 16).Shrink(0)
		}
		ui.Text(c, a.Message).Label("Operation status").TextColor(c.Theme().TextMuted).FontSize(11).Grow(1).MinWidth(0).SingleLine().Tooltip(a.Message)
		if a.Stale {
			ui.Text(c, "Stale").TextColor(c.Theme().Danger)
		}
		if a.lastSyncError != "" {
			ui.Textf(c, "Sync failed · %v", a.lastSyncFailedAt).Label("Background sync error").TextColor(c.Theme().TextMuted).FontSize(11).NoWrap().Tooltip(a.lastSyncError)
		}
		if host := a.Repository.SSHHost; host != "" {
			ui.Textf(c, "SSH Connected · %s", host).TextColor(c.Theme().TextMuted)
		} else if a.Repository.Repository.RepositoryID != "" {
			ui.Text(c, "Local").TextColor(c.Theme().TextMuted)
		}
	})
}
