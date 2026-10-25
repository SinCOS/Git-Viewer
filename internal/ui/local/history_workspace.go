package local

import "github.com/egoist/mygo/ui"

// Keep the history table under the same parent when toggling details. Replacing
// the parent with a Split used to discard the table's native widget identity.
func (a *App) historyWorkspace(c *ui.Context) {
	row := ui.Row(c).Label("History panels").Fill()
	row.Children(func() {
		history := ui.Column(c).Key("history").MinWidth(0).Shrink(1)
		width := a.HistoryWidth
		total := row.Bounds().W
		if total > 0 {
			width = min(width, max(40, total-265))
		}
		width = max(40, width)
		if a.HistoryDetailsOpen {
			history.Width(width).FillHeight()
		} else {
			history.Fill()
		}
		history.Children(func() { a.historyView(c) })
		if !a.HistoryDetailsOpen {
			return
		}
		divider := ui.Box(c).Key("details-divider").Label("Resize commit details").Role(ui.RoleSplitter).
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
		if total > 0 {
			next = min(next, max(40, total-265))
		}
		next = max(40, next)
		if next != width {
			a.HistoryWidth = next
			history.Width(next)
		}
		divider.Draw(func(p *ui.Painter, r ui.Rect) {
			r.X += 2
			r.W = 1
			p.Fill(r, c.Theme().Border, 0)
		})
		ui.Column(c).Key("inspector").Grow(1).MinWidth(40).FillHeight().Children(func() { a.historyInspector(c) })
	})
}
