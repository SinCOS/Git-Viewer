package local

import "github.com/egoist/mygo/ui"

// historyWorkspace mounts the Git Log editor area: the commit history with a
// bottom commit-details panel. The branch tree lives in the primary side bar.
// Panels stay siblings under this Row with stable Keys, and toggles never
// replace the history table's parent chain: replacing a parent with a Split
// used to discard the table's native widget identity.
func (a *App) historyWorkspace(c *ui.Context) {
	row := ui.Row(c).Label("History panels").Fill()
	row.Children(func() {
		column := ui.Column(c).Key("history-column").Grow(1).MinWidth(0).FillHeight()
		column.Children(func() {
			a.historyView(c)
			if a.HistoryDetailsOpen {
				a.historyDetails(c, column.Bounds().H)
			}
		})
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
