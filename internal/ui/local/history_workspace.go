package local

import "github.com/egoist/mygo/ui"

// historyWorkspace mounts the Git Log page: a resizable, hideable branch tree
// beside the commit history with a bottom commit-details panel. Every panel is
// a sibling under this Row with a stable Key, and toggles never replace the
// history table's parent chain: replacing a parent with a Split used to
// discard the table's native widget identity.
func (a *App) historyWorkspace(c *ui.Context) {
	row := ui.Row(c).Label("History panels").Fill()
	row.Children(func() {
		total := row.Bounds().W
		if !a.BranchTreeHidden {
			width := clampTreeWidth(a.BranchTreeWidth, total)
			ui.Column(c).Key("branch-tree").Width(width).FillHeight().Shrink(0).
				Children(func() { a.branchNavigation(c) })
			a.treeDivider(c, width, total)
		}
		column := ui.Column(c).Key("history-column").Grow(1).MinWidth(0).FillHeight()
		column.Children(func() {
			a.historyView(c)
			if a.HistoryDetailsOpen {
				a.historyDetails(c, column.Bounds().H)
			}
		})
	})
}

func clampTreeWidth(width, total float32) float32 {
	if total > 0 {
		width = min(width, max(160, total-265))
	}
	return max(160, min(width, 480))
}

// treeDivider is the branch tree's resize handle. It mirrors the manual
// splitter used for the details panel: drag or arrow keys move the shared
// BranchTreeWidth, which ui.Split used to own.
func (a *App) treeDivider(c *ui.Context, width, total float32) {
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
	next = clampTreeWidth(next, total)
	if next != width {
		a.BranchTreeWidth = next
	}
	divider.Draw(func(p *ui.Painter, r ui.Rect) {
		r.X += 2
		r.W = 1
		p.Fill(r, c.Theme().Border, 0)
	})
}

// historyDetails mounts the bottom commit-details panel: metadata, changed
// files and the diff, resized with a horizontal handle along its top edge.
func (a *App) historyDetails(c *ui.Context, total float32) {
	height := a.HistoryDetailsHeight
	if total > 0 {
		height = min(height, max(140, total-200))
	}
	height = max(140, height)
	divider := ui.Box(c).Key("details-divider").Label("Resize commit details").Role(ui.RoleSplitter).
		Height(5).FillWidth().Shrink(0).Focusable().Cursor(ui.CursorResizeNS)
	divider.Dragged()
	up, down := divider.Shortcut(0, ui.KeyUp), divider.Shortcut(0, ui.KeyDown)
	_, dy, held := divider.Dragged()
	next := height
	if up {
		next -= 10
	}
	if down {
		next += 10
	}
	if held {
		divider.Focus()
		next -= dy
	}
	if total > 0 {
		next = min(next, max(140, total-200))
	}
	next = max(140, next)
	if next != height {
		a.HistoryDetailsHeight = next
		height = next
	}
	divider.Draw(func(p *ui.Painter, r ui.Rect) {
		r.Y += 2
		r.H = 1
		p.Fill(r, c.Theme().Border, 0)
	})
	ui.Column(c).Key("details-panel").Height(height).FillWidth().Shrink(0).Children(func() { a.historyInspector(c) })
}
