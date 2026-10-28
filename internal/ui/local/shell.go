package local

import (
	"github.com/egoist/mygo/ui"
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
	c.SetTheme(vscodeTheme(c.Theme()))
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
