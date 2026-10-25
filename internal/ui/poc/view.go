package poc

import (
	"fmt"
	"math"
	"strings"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
)

const CommitCount = 10000
const GraphCount = 200

type App struct {
	SidebarWidth  float32
	HistoryWidth  float32
	Selected      int
	Repository    int
	GraphMode     bool
	Binary        bool
	HealthChecks  int
	List          ui.ListState
	GraphScroll   ui.ScrollState
	DiffScroll    ui.ScrollState
	GraphSelected int
	Tasks         Tasks
	// BuiltRows measures one view pass, never the whole data collection.
	BuiltRows int
}

func New() *App {
	return &App{SidebarWidth: 200, HistoryWidth: 450, Selected: -1, GraphSelected: -1, Tasks: Tasks{Connection: "idle", Message: "Ready"}}
}
func (a *App) Health() { a.HealthChecks++; a.Tasks.Start(application.Health) }
func (a *App) View(c *ui.Context) {
	c.OnShortcut(ui.Cmd, ui.KeyR, a.Health)
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(8).Gap(8).Children(func() {
			ui.Text(c, "Git Viewer · Phase 0").Bold()
			ui.Button(c, "Health / Version").OnClick(a.Health)
			ui.Button(c, "Cancel task").Disabled(!a.Tasks.Loading).OnClick(a.Tasks.Cancel)
			ui.Button(c, "Simulate disconnect").OnClick(a.Tasks.Disconnect)
			ui.Button(c, "Graph / Diff").OnClick(func() { a.GraphMode = !a.GraphMode })
		})
		ui.Textf(c, "%s · %s · stale=%t · %s", application.Version, a.Tasks.Connection, a.Tasks.Stale, a.Tasks.Message).Selectable().Padding(4)
		ui.Split(c, &a.SidebarWidth, func() { a.sidebar(c) }, func() {
			ui.Split(c, &a.HistoryWidth, func() { a.HistoryView(c) }, func() {
				if a.GraphMode {
					a.GraphView(c)
				} else {
					a.DiffView(c)
				}
			}).Fill()
		}).Grow(1).FillWidth()
	})
}
func (a *App) sidebar(c *ui.Context) {
	ui.Column(c).Fill().Padding(8).Gap(8).Children(func() {
		ui.Text(c, "Mock repositories").Bold()
		for i, name := range []string{"Local fixture", "SSH fixture"} {
			ui.Button(c.Key(i), name).OnClick(func() { a.Repository = i })
		}
		ui.Textf(c, "Selected fixture: %d", a.Repository)
		ui.Text(c, "Technical POCs use mock commits and graph nodes.")
	})
}
func (a *App) HistoryView(c *ui.Context) {
	a.BuiltRows = 0
	a.List.Selected = &a.Selected
	a.List.Key = func(i int) any { return i }
	a.List.Label = func(i int) string { return fmt.Sprintf("Commit %05d", i) }
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Gap(6).Padding(4).Children(func() {
			ui.Text(c, "10,000 mock commits")
			ui.Button(c, "Jump to 9999").OnClick(func() { a.List.ScrollTo(9999, ui.Center) })
		})
		ui.List(c, &a.List, CommitCount, func(i int) { a.BuiltRows++; ui.Textf(c, "Commit %05d", i).Height(28).Padding(4) }).Grow(1).FillWidth()
	})
}
func (a *App) DiffView(c *ui.Context) {
	ui.Column(c).Fill().Padding(8).Gap(8).Children(func() {
		ui.Text(c, "Static unified diff POC").Bold()
		ui.Button(c, "Binary / text").OnClick(func() { a.Binary = !a.Binary })
		if a.Binary {
			ui.Text(c, "Binary file · preview unavailable")
			return
		}
		ui.ScrollBoth(c).TrackScroll(&a.DiffScroll).Grow(1).FillWidth().Children(func() {
			ui.Column(c).Selectable().Children(func() {
				ui.Text(c, "@@ -1,2 +1,2 @@").Font("monospace").NoWrap()
				ui.Text(c, "- old text 旧内容").TextColor(c.Theme().Danger).Font("monospace").NoWrap()
				ui.Text(c, "+ new text 新内容").TextColor(c.Theme().Accent).Font("monospace").NoWrap()
				ui.Text(c, "+ "+strings.Repeat("中文 long line ", 100)).Font("monospace").NoWrap()
			})
		})
	})
}

// GraphView draws a synthetic 20x10 node layout, not a commit DAG algorithm.
// 200 nodes fit at 840x420; a narrower pane scrolls in both directions.
func (a *App) GraphView(c *ui.Context) {
	ui.ScrollBoth(c).TrackScroll(&a.GraphScroll).Fill().Children(func() {
		box := ui.Box(c).Size(840, 420).Label("Mock graph")
		if box.Clicked() {
			x, y, _ := box.PointerPosition()
			a.GraphSelected = hitNode(x, y)
		}
		selected := a.GraphSelected
		accent, muted := c.Theme().Accent, c.Theme().TextMuted
		box.Draw(func(p *ui.Painter, r ui.Rect) {
			for i := 0; i < GraphCount; i++ {
				x, y := nodePosition(i)
				if i%20 < 19 {
					p.Line(r.X+x, r.Y+y, r.X+x+40, r.Y+y, 1, muted)
				}
				if i%20 == 0 && i+20 < GraphCount {
					p.Line(r.X+x, r.Y+y, r.X+x, r.Y+y+38, 1, muted)
				}
				radius := float32(4)
				if i == selected {
					radius = 7
				}
				p.Fill(ui.Rect{X: r.X + x - radius, Y: r.Y + y - radius, W: radius * 2, H: radius * 2}, accent, radius)
				if i%20 == 0 {
					p.Text(r.X+x+8, r.Y+y-16, fmt.Sprintf("branch-%d", i/20), 10, muted)
				}
			}
		})
	})
}
func nodePosition(i int) (float32, float32) { return 20 + float32(i%20)*40, 30 + float32(i/20)*38 }
func hitNode(x, y float32) int {
	for i := 0; i < GraphCount; i++ {
		nx, ny := nodePosition(i)
		if math.Hypot(float64(x-nx), float64(y-ny)) <= 10 {
			return i
		}
	}
	return -1
}
