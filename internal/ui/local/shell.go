package local

import (
	"github.com/egoist/mygo/ui"
)

// shell.go owns the window frame: the top toolbar, the narrow mode rail,
// panel mounting and the status bar. Panels are mounted here and must not
// mount each other.
// Modes map onto Pane values shared with the toolbar buttons:
// "commit"→"Status", "log"→"" (history), "editor"→"Editor".

// UIState is layout-only UI state. Business data stays on App.
type UIState struct {
	SidebarWidth   float32
	InspectorWidth float32
}

func (a *App) View(c *ui.Context) {
	c.SetTheme(vscodeTheme(c.Theme()))
	c.OnShortcut(ui.Cmd, ui.KeyR, a.Refresh)
	c.OnShortcut(ui.Cmd, ui.KeyO, a.OpenFolderDialog)
	c.OnShortcut(ui.Cmd, ui.Key1, func() { a.Pane = "Editor" })
	c.OnShortcut(ui.Cmd, ui.Key2, func() { a.Pane = "" })
	c.OnShortcut(ui.Cmd, ui.Key3, func() { a.Pane = "Status" })
	c.OnShortcut(ui.Cmd, ui.KeyS, func() { a.Message = message(a.SaveActiveEditorFile()) })
	c.OnShortcut(ui.Cmd, ui.KeyP, a.OpenQuickOpen)
	a.syncMode()
	a.ensureHistory()
	ui.Column(c).Fill().Children(func() {
		a.toolbarView(c)
		if a.StoreWarning {
			ui.Text(c, "Recent repositories could not be read or saved. Check application configuration.").TextColor(c.Theme().Danger)
		}
		ui.Row(c).Label("Workspace body").Grow(1).FillWidth().Children(func() {
			a.modeRail(c)
			a.primarySidebar(c)
			ui.Column(c).Fill().Children(func() { a.mainView(c) }).Grow(1)
		})
	})
	a.quickOpenView(c)
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
			a.requestBackgroundRefresh()
		case "Editor":
			a.Mode = "editor"
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
		a.requestBackgroundRefresh()
	case "editor":
		a.Pane = "Editor"
	default:
		a.Pane = ""
	}
	a.Mode = mode
	a.railPane = a.Pane
	a.ensureHistory()
}
func (a *App) mainView(c *ui.Context) {
	switch a.Mode {
	case "editor":
		a.editorView(c)
	case "log":
		a.historyWorkspace(c)
	default:
		ui.Column(c).Fill().Children(func() { a.diffView(c) }).Grow(1)
	}
}

// primarySidebar mounts the VS Code-style primary side bar: the mode's
// navigation panel followed by a shared resize divider.
func (a *App) primarySidebar(c *ui.Context) {
	width := clampSidebarWidth(a.SidebarWidth)
	ui.Column(c).Key("primary-sidebar").Width(width).FillHeight().Shrink(0).
		Background(c.Theme().Surface).Children(func() {
		ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
			switch a.Mode {
			case "log":
				a.branchNavigation(c)
			case "editor":
				a.editorSidebar(c)
			default:
				a.commitSidebarPane(c)
			}
		})
	})
	a.sidebarDivider(c, width)
}

func (a *App) commitSidebarPane(c *ui.Context) {
	pane := ui.Column(c).Fill()
	pane.Children(func() {
		ui.Row(c).Height(30).FillWidth().Padding(0, 8).AlignItems(ui.Center).
			BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Children(func() {
			ui.Text(c, "SOURCE CONTROL").Bold().FontSize(10).TextColor(c.Theme().TextMuted).Grow(1).NoWrap()
			iconButton(c, "Refresh source control", "Refresh source control", refreshIcon).
				Size(20, 20).Disabled(a.Writing || a.DialogOpen).OnClick(a.Refresh)
		})
		formHeight := float32(260)
		if height := pane.Bounds().H; height > 0 {
			formHeight = min(210, max(150, height*0.34))
		}
		ui.Scroll(c).Label("Commit form scroll").MaxHeight(formHeight).FillWidth().Shrink(0).Children(func() { a.commitSidebar(c) })
		ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() { a.statusView(c) })
	})
}

func clampSidebarWidth(width float32) float32 {
	return max(220, min(480, width))
}

// sidebarDivider is the side bar's resize handle, mirroring the manual
// splitter used for the history details panel.
func (a *App) sidebarDivider(c *ui.Context, width float32) {
	divider := ui.Box(c).Key("tree-divider").Label("Resize branch tree").Role(ui.RoleSplitter).
		Width(5).FillHeight().Shrink(0).Focusable().Cursor(ui.CursorResizeEW)
	divider.Dragged() // Register drag input before applying this frame's events.
	left, right := divider.Shortcut(0, ui.KeyLeft), divider.Shortcut(0, ui.KeyRight)
	dx, _, held := divider.Dragged()
	next := width
	if left {
		next -= 10
	}
	if right {
		next += 10
	}
	if held {
		divider.Focus()
		next += dx
	}
	next = clampSidebarWidth(next)
	if next != width {
		a.SidebarWidth = next
	}
	divider.Draw(func(p *ui.Painter, r ui.Rect) {
		r.X += 2
		r.W = 1
		p.Fill(r, c.Theme().Border, 0)
	})
}
