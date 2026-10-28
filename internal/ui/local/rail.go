package local

import "github.com/egoist/mygo/ui"

// modeRail provides the application modes and their keyboard shortcuts.
func (a *App) modeRail(c *ui.Context) {
	modes := []struct {
		id, label, shortcut string
		icon                *ui.SVG
	}{
		{"editor", "Editor", "⌘1", codeIcon},
		{"log", "Git Log", "⌘2", logIcon},
		{"commit", "Commit", "⌘3", commitIcon},
	}
	ui.Column(c).Label("Mode navigation").Width(48).FillHeight().Shrink(0).
		Padding(4).Gap(6).Background(c.Theme().Surface).Children(func() {
		for _, m := range modes {
			active := a.Mode == m.id
			item := ui.ButtonBase(c).Label(m.label).Tooltip(m.label + " (" + m.shortcut + ")").
				Width(40).Height(40).Radius(2).
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
				ui.Icon(c, m.icon).Size(22, 22).Shrink(0).TextColor(color)
			})
		}
	})
}
