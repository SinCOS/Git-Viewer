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
				Width(96).Height(68).Radius(5).
				Background(ui.Transparent).Cursor(ui.CursorPointer).
				AlignItems(ui.Center).Justify(ui.Center).
				OnClick(func() { a.setMode(m.id) })
			color := c.Theme().Text
			if active {
				item.Background(c.Theme().Accent)
				color = c.Theme().AccentText
			} else if item.Hovered() {
				item.Background(c.Theme().SurfaceHover)
			}
			item.Children(func() {
				ui.Column(c).Gap(4).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, m.icon).Size(20, 20).Shrink(0).TextColor(color)
					ui.Text(c, m.label).TextColor(color).NoWrap()
				})
			})
		}
	})
}
