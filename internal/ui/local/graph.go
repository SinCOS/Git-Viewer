package local

import (
	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

// graph.go owns the commit graph drawing: lanes, edges and the commit dot for
// one history row. history.go positions rows; this file must stay the only
// writer of the graph painting code.

// Frozen UL005 layout parameters (see
// docs/features/git-viewer/evidence/parallel/contract-ui.md): rows are 48px
// tall, lanes are pitched 16px apart and the first lane center sits 12px from
// the row's left edge. graphHitTest must stay in sync with these values.
const (
	graphRowHeight = 48
	graphLanePitch = 16
	graphLaneInset = 12
	graphHitRadius = 6
)

// secondParentEdge returns the index of the row's second parent edge, i.e. the
// second edge starting at the commit lane. domain.GraphLayout appends
// pass-through edges first, then parent edges in parent order, so edges with
// From == row.Lane are exactly this commit's parent edges. It returns -1 when
// the commit has fewer than two parents.
func secondParentEdge(row domain.GraphRow) int {
	parents := 0
	for i, e := range row.Edges {
		if e.From == row.Lane {
			parents++
			if parents == 2 {
				return i
			}
		}
	}
	return -1
}

func (a *App) graphRow(c *ui.Context, i int) {
	row := a.GraphRows[i]
	accent, muted, danger := c.Theme().Accent, c.Theme().TextMuted, c.Theme().Danger
	second := secondParentEdge(row)
	ui.Row(c).Height(graphRowHeight).Children(func() {
		box := ui.Box(c).Size(float32(max(2, a.HistoryLanes))*graphLanePitch+graphLaneInset, graphRowHeight)
		box.Draw(func(p *ui.Painter, r ui.Rect) {
			x := func(l int) float32 { return r.X + graphLaneInset + float32(l)*graphLanePitch }
			for idx, e := range row.Edges {
				// Visual: the second parent edge of a merge commit is drawn in
				// the theme Danger color so it is distinguishable from the
				// muted main line; every other edge stays muted.
				color := muted
				if idx == second {
					color = danger
				}
				p.Line(x(e.From), r.Y+graphRowHeight/2, x(e.To), r.Y+graphRowHeight, 1, color)
			}
			if i > 0 {
				prev := a.GraphRows[i-1]
				prevSecond := secondParentEdge(prev)
				for idx, e := range prev.Edges {
					// Visual: the arrival stub of the previous row's
					// second-parent edge keeps the same Danger color as its
					// diagonal half so one edge renders as a single color.
					color := muted
					if idx == prevSecond {
						color = danger
					}
					p.Line(x(e.To), r.Y, x(e.To), r.Y+graphRowHeight/2, 1, color)
				}
			}
			// Visual: the commit dot itself stays in the theme Accent color.
			p.Fill(ui.Rect{X: x(row.Lane) - 4, Y: r.Y + graphRowHeight/2 - 4, W: 8, H: 8}, accent, 4)
		})
		ui.Column(c).Children(func() {
			ui.Text(c, row.Commit.Subject).NoWrap()
			ui.Textf(c, "%.10s · %s · %s · %s", row.Commit.OID, row.Commit.Author, row.Commit.AuthoredAt.Format("2006-01-02"), row.Commit.Refs).NoWrap()
		})
	})
}

// graphHitTest reports whether x, expressed in row content coordinates, falls
// within graphHitRadius of the commit dot center of row i. The dot center is
// graphLaneInset + lane*graphLanePitch horizontally, matching graphRow's
// painting; callers resolve the row before hit testing, so only x is needed.
func (a *App) graphHitTest(i int, x float32) bool {
	if i < 0 || i >= len(a.GraphRows) {
		return false
	}
	center := float32(graphLaneInset) + float32(a.GraphRows[i].Lane)*float32(graphLanePitch)
	d := x - center
	if d < 0 {
		d = -d
	}
	return d <= graphHitRadius
}
