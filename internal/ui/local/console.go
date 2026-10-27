package local

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// recordConsole deliberately records only application-generated summaries;
// Git/SSH output can contain credentials and is never added to this view.
func (a *App) recordConsole(text string) {
	const limit = 100
	if len(a.consoleLines) == limit {
		copy(a.consoleLines, a.consoleLines[1:])
		a.consoleLines = a.consoleLines[:limit-1]
	}
	a.consoleLines = append(a.consoleLines, time.Now().Format("15:04:05")+"  "+text)
}

func (a *App) consoleView(c *ui.Context) {
	ui.Column(c).Label("Operation console").FillWidth().Height(144).Padding(8).Gap(4).Background(c.Theme().Surface).Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, "Console").Bold().Grow(1)
			ui.Button(c, "Clear console").Disabled(len(a.consoleLines) == 0).OnClick(func() { a.consoleLines = nil })
			ui.Button(c, "Close console").OnClick(func() { a.consoleOpen = false })
		})
		ui.Scroll(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).FillWidth().Gap(4).Children(func() {
				if len(a.consoleLines) == 0 {
					ui.Text(c, "No operations yet").TextColor(c.Theme().TextMuted)
				}
				for _, line := range a.consoleLines {
					ui.Text(c, line).Font("monospace").FontSize(11).Selectable()
				}
			})
		})
	})
}
