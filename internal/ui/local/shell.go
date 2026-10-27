package local

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

// shell.go owns the window frame: the top toolbar, the narrow mode rail,
// panel mounting and the status bar. Panels are mounted here and must not
// mount each other.
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
		a.toolbarView(c)
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
	// Sync the toolbar from Pane only when Pane changed outside the toolbar, so
	// shortcuts update the mode buttons without fighting them.
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

// setMode is the single entry point for mode switches from the toolbar. The
// reverse direction (Pane changed elsewhere) is reconciled by syncMode.
func (a *App) setMode(mode string) {
	if a.Mode == mode {
		return
	}
	switch mode {
	case "commit":
		a.Pane = "Status"
	case "worktree":
		a.Pane = "Branches"
	default:
		a.Pane = ""
	}
	a.Mode = mode
	a.railPane = a.Pane
	a.ensureHistory()
}
func (a *App) mainView(c *ui.Context) {
	a.workspaceReposBar(c)
	switch a.Mode {
	case "worktree":
		a.branchesView(c)
	case "log":
		a.historyWorkspace(c)
	default:
		ui.Split(c, &a.StatusWidth, func() {
			pane := ui.Column(c).Fill()
			pane.Children(func() {
				ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() { a.statusView(c) })
				formHeight := float32(260)
				if height := pane.Bounds().H; height > 0 {
					formHeight = min(280, max(200, height*0.45))
				}
				ui.Scroll(c).Label("Commit form scroll").MaxHeight(formHeight).FillWidth().Children(func() { a.commitSidebar(c) })
			})
		}, func() { a.diffView(c) }).Grow(1).FillWidth()
	}
}

// workspaceReposBar renders the multi-root workspace like VS Code: one
// switcher row listing every repository in the workspace with its live change
// count, the open one marked. Hidden for single-repository workspaces.
func (a *App) workspaceReposBar(c *ui.Context) {
	if len(a.WorkspaceRepositories) < 2 {
		return
	}
	ui.ScrollHorizontal(c).Label("Workspace repositories").Height(40).FillWidth().Children(func() {
		ui.Row(c).Padding(4, 8).Gap(8).Children(func() {
			for i, repo := range a.WorkspaceRepositories {
				current := a.currentRepositoryPath() == repo.Repository.Path
				ui.Row(c).Gap(4).Children(func() {
					ui.Button(c.Key(i), compactLabel(repo.Repository.DisplayName, 24)).Tooltip(repo.Repository.Path).OnClick(func() { a.Open(repo.Repository.Path) })
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
	})
}
func (a *App) statusBar(c *ui.Context) {
	// Keep all status items accessible when the window is narrow.
	ui.ScrollHorizontal(c).Label("Status bar").Height(30).FillWidth().Children(func() {
		ui.Row(c).Height(30).FillWidth().Padding(6, 8).Gap(12).AlignItems(ui.Center).Background(c.Theme().Surface).Children(func() {
			if a.Repository.Repository.RepositoryID == "" {
				ui.Text(c, "No repository").TextColor(c.Theme().TextMuted).NoWrap()
			} else {
				branch := a.currentBranch()
				staged, unstaged := 0, 0
				for _, entry := range a.Entries {
					if entry.Scope == domain.StagedDiff {
						staged++
					} else {
						unstaged++
					}
				}
				ui.Textf(c, "⎇ %s", compactLabel(branch, 24)).Tooltip(branch).TextColor(c.Theme().TextMuted).NoWrap()
				ui.Textf(c, "%d staged", staged).TextColor(c.Theme().TextMuted).NoWrap()
				ui.Textf(c, "%d unstaged", unstaged).TextColor(c.Theme().TextMuted).NoWrap()
			}
			if a.Loading && !a.quietLoading || a.DialogOpen {
				ui.Spinner(c).Label("Git operation in progress").Size(16, 16).Shrink(0)
			}
			ui.Text(c, compactLabel(a.Message, 48)).Label("Operation status").TextColor(c.Theme().TextMuted).FontSize(11).Grow(1).MinWidth(0).SingleLine().Tooltip(a.Message)
			if a.Stale {
				ui.Text(c, "Stale").TextColor(c.Theme().Danger)
			}
			if a.lastSyncError != "" {
				ui.Textf(c, "Sync failed · %v", a.lastSyncFailedAt).Label("Background sync error").TextColor(c.Theme().TextMuted).FontSize(11).NoWrap().Tooltip(a.lastSyncError)
			}
			if host := a.Repository.SSHHost; host != "" {
				ui.Textf(c, "SSH Connected · %s", compactLabel(host, 20)).Tooltip(host).TextColor(c.Theme().TextMuted).NoWrap()
			} else if a.Repository.Repository.RepositoryID != "" {
				ui.Text(c, "Local").TextColor(c.Theme().TextMuted).NoWrap()
			}
		})
	})
}
