package local

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// Column zero starts the graph at the same x offset as graphHitTest. The
// table adds cell padding; painting restores that space so lane segments
// meet at adjacent row boundaries.
const (
	graphLanePitch = 16
	graphLaneInset = 12
	graphHitRadius = 6
)

func (a *App) graphColor(c *ui.Context, oid string) ui.Color {
	if a.graphColorIDs == nil || a.graphColorCount != len(a.GraphRows) {
		a.graphColorIDs = make(map[string]int)
		next := 0
		for _, row := range a.GraphRows {
			color, exists := a.graphColorIDs[row.Commit.OID]
			if !exists {
				color = next
				next++
				a.graphColorIDs[row.Commit.OID] = color
			}
			for i, parent := range row.Commit.Parents {
				if _, exists := a.graphColorIDs[parent]; exists {
					continue
				}
				if i == 0 {
					a.graphColorIDs[parent] = color
				} else {
					a.graphColorIDs[parent] = next
					next++
				}
			}
		}
		a.graphColorCount = len(a.GraphRows)
	}
	palette := []ui.Color{ui.Hex("#3b86d1"), ui.Hex("#cc3696"), ui.Hex("#56b83d"), ui.Hex("#c59637"), ui.Hex("#329eab"), ui.Hex("#8b6ad1")}
	color := palette[a.graphColorIDs[oid]%len(palette)]
	if c.Theme().Dark {
		color = color.Mix(ui.RGB(255, 255, 255), .15)
	}
	return color
}

func (a *App) historyRefs(refs string) []string {
	tags := []string{}
	for _, ref := range strings.Split(refs, ",") {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		remote := false
		for _, b := range a.Branches {
			if strings.HasPrefix(b.Name, "refs/remotes/") && ref == strings.TrimPrefix(b.Name, "refs/remotes/") {
				remote = true
				break
			}
		}
		if !a.HistoryShowRemote && remote {
			continue
		}
		tags = append(tags, ref)
	}
	return tags
}

func (a *App) historyCell(c *ui.Context, i, col int) {
	row := a.historyDisplayGraph[i]
	commit := row.Commit
	switch col {
	case 0:
		color := a.graphColor(c, commit.OID)
		pad := c.Theme().Space(1.5)
		ui.Box(c).FillWidth().Height(c.Theme().Space(8) - pad*2).Draw(func(p *ui.Painter, r ui.Rect) {
			r.X -= c.Theme().Space(2.5)
			r.Y -= pad
			r.H += pad * 2
			x := func(l int) float32 { return r.X + graphLaneInset + float32(l)*graphLanePitch }
			middle, bottom := r.Y+r.H/2, r.Y+r.H
			for _, edge := range row.Edges {
				path := new(ui.Path).MoveTo(x(edge.From), middle).CubeTo(x(edge.From), middle+r.H/4, x(edge.To), bottom-r.H/4, x(edge.To), bottom)
				p.StrokePath(path, 1.5, a.graphColor(c, edge.Parent))
			}
			if i > 0 {
				for _, edge := range a.historyDisplayGraph[i-1].Edges {
					p.Line(x(edge.To), r.Y, x(edge.To), middle, 1.5, a.graphColor(c, edge.Parent))
				}
			}
			p.Fill(ui.Rect{X: x(row.Lane) - 3.5, Y: middle - 3.5, W: 7, H: 7}, color, 3.5)
		})
	case 1:
		color := a.graphColor(c, commit.OID)
		ui.Row(c).Label("Commit description " + commit.OID).Height(20).Gap(6).FillWidth().MinWidth(0).Clip().Children(func() {
			for _, ref := range a.historyRefs(commit.Refs) {
				ui.Row(c).Label("Branch label "+ref).Tooltip(ref).Height(20).MaxWidth(210).Shrink(1).MinWidth(0).Gap(4).Border(1, c.Theme().Border).Radius(3).Clip().Children(func() {
					ui.Box(c).Size(18, 18).Shrink(0).Background(color).Center().Children(func() { ui.Icon(c, logIcon).Size(12, 12).TextColor(ui.RGB(255, 255, 255)) })
					ui.Text(c, ref).FontSize(11).NoWrap().Padding(0, 4, 0, 0)
				})
			}
			ui.Text(c, commit.Subject).FontSize(12).MinWidth(0).Grow(1).Tooltip(commit.Subject).NoWrap()
		})
	case 2:
		ui.Text(c, commit.AuthoredAt.Format("02 Jan 2006 15:04")).Label("Commit date " + commit.OID).FontSize(11).NoWrap()
	case 3:
		ui.Text(c, commit.Author).Label("Commit author " + commit.OID).FontSize(12).NoWrap().Tooltip(commit.Author)
	case 4:
		ui.Textf(c, "%.8s", commit.OID).Label("Commit revision " + commit.OID).Font("monospace").FontSize(11).NoWrap().Tooltip(commit.OID).Selectable()
	}
}

func (a *App) graphHitTest(i int, x float32) bool {
	if i < 0 || i >= len(a.GraphRows) {
		return false
	}
	center := float32(graphLaneInset) + float32(a.GraphRows[i].Lane)*graphLanePitch
	d := x - center
	if d < 0 {
		d = -d
	}
	return d <= graphHitRadius
}
