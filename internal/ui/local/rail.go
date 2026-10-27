package local

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// rail.go owns the narrow mode navigation on the left edge: a 52–60px column
// of icon buttons, Commit first, Git Log second, Worktree third. MyGo's
// ui.Sidebar always renders item labels, so the narrow rail is a custom icon
// column; the label stays for accessibility and tests.

func (a *App) modeRail(c *ui.Context) {
	modes := []struct {
		id, label string
		icon      *ui.SVG
	}{
		{"commit", "Commit", commitIcon},
		{"log", "Git Log", logIcon},
		{"worktree", "Worktree", wtIcon},
	}
	rail := ui.Column(c).Label("Mode navigation").Width(56).FillHeight().Shrink(0).Padding(4, 0).Gap(2)
	rail.Children(func() {
		for _, m := range modes {
			active := a.Mode == m.id
			item := ui.ButtonBase(c).Label(m.label).Tooltip(m.label).
				Width(48).Height(44).
				Background(ui.Transparent).Cursor(ui.CursorPointer).
				AlignItems(ui.Center).Justify(ui.Center).
				OnClick(func() { a.setMode(m.id) })
			if active {
				item.Background(c.Theme().SurfacePressed)
			}
			item.Children(func() {
				ui.Icon(c, m.icon).Size(20, 20).Shrink(0)
				if m.id == "commit" {
					if n := len(a.Entries); n > 0 {
						ui.Badge(c, fmt.Sprintf("%d", n))
					}
				}
			})
		}
	})
}
