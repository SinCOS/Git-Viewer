package local

import (
	"github.com/egoist/mygo/ui"
)

// graph.go owns the commit graph drawing: lanes, edges and the commit dot for
// one history row. history.go positions rows; this file must stay the only
// writer of the graph painting code.
func (a *App) graphRow(c *ui.Context, i int) {
	row := a.GraphRows[i]
	accent, muted := c.Theme().Accent, c.Theme().TextMuted
	ui.Row(c).Height(48).Children(func() {
		box := ui.Box(c).Size(float32(max(2, a.HistoryLanes))*16+12, 48)
		box.Draw(func(p *ui.Painter, r ui.Rect) {
			x := func(l int) float32 { return r.X + 12 + float32(l)*16 }
			for _, e := range row.Edges {
				p.Line(x(e.From), r.Y+24, x(e.To), r.Y+48, 1, muted)
			}
			if i > 0 {
				for _, e := range a.GraphRows[i-1].Edges {
					p.Line(x(e.To), r.Y, x(e.To), r.Y+24, 1, muted)
				}
			}
			p.Fill(ui.Rect{X: x(row.Lane) - 4, Y: r.Y + 20, W: 8, H: 8}, accent, 4)
		})
		ui.Column(c).Children(func() {
			ui.Text(c, row.Commit.Subject).NoWrap()
			ui.Textf(c, "%.10s · %s · %s · %s", row.Commit.OID, row.Commit.Author, row.Commit.AuthoredAt.Format("2006-01-02"), row.Commit.Refs).NoWrap()
		})
	})
}
