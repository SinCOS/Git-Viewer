package local

import "github.com/egoist/mygo/ui"

// modeRail provides the three application modes and their keyboard shortcuts.
func (a *App) modeRail(c *ui.Context) {
	modes := []struct {
		id, label, shortcut string
		icon                *ui.SVG
	}{
		{"commit", "Commit", "⌘1", commitIcon},
		{"log", "Git Log", "⌘2", logIcon},
		{"worktree", "Worktree", "⌘3", wtIcon},
	}
	ui.Column(c).Label("Mode navigation").Width(108).FillHeight().Shrink(0).
		Padding(8, 6).Gap(4).Background(c.Theme().Surface).Children(func() {
		for _, m := range modes {
			active := a.Mode == m.id
			item := ui.ButtonBase(c).Label(m.label).Tooltip(m.label + " (" + m.shortcut + ")").
				Width(96).Height(40).Radius(2).
				Background(ui.Transparent).Cursor(ui.CursorPointer).
				AlignItems(ui.Center).Justify(ui.Center).
				OnClick(func() { a.setMode(m.id) })
			color := c.Theme().Text
			if active {
				color = c.Theme().Accent
				item.Background(c.Theme().Selection)
			} else if item.Hovered() {
				color = c.Theme().Text
			}
			item.Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, m.icon).Size(18, 18).Shrink(0).TextColor(color)
					ui.Text(c, m.label).TextColor(color).NoWrap()
				})
			})
		}
	})
}
