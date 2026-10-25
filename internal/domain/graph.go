package domain

// GraphLayout maintains unresolved parent lanes across history pages. Edges end
// on the next row, including intermediate rows, rather than skipping commits.
type GraphEdge struct {
	From, To int
	Parent   string
}
type GraphRow struct {
	Commit CommitRecord
	Lane   int
	Width  int
	Edges  []GraphEdge
}
type GraphLayout struct {
	Lanes []string
	seen  map[string]bool
}

func (g *GraphLayout) Append(commits []CommitRecord) []GraphRow {
	if g.seen == nil {
		g.seen = map[string]bool{}
	}
	rows := make([]GraphRow, 0, len(commits))
	for _, c := range commits {
		if g.seen[c.OID] {
			continue
		}
		g.seen[c.OID] = true
		lane := -1
		for i, id := range g.Lanes {
			if id == c.OID {
				lane = i
				break
			}
		}
		if lane < 0 {
			lane = len(g.Lanes)
			g.Lanes = append(g.Lanes, c.OID)
		}
		before := append([]string(nil), g.Lanes...)
		g.Lanes = append(g.Lanes[:lane], g.Lanes[lane+1:]...)
		at := lane
		for _, parent := range c.Parents {
			found := false
			for _, id := range g.Lanes {
				if id == parent {
					found = true
					break
				}
			}
			if !found {
				g.Lanes = append(g.Lanes, "")
				copy(g.Lanes[at+1:], g.Lanes[at:])
				g.Lanes[at] = parent
				at++
			}
		}
		row := GraphRow{Commit: c, Lane: lane, Width: max(len(before), len(g.Lanes))}
		for i, id := range before {
			if i == lane {
				continue
			}
			for j, next := range g.Lanes {
				if next == id {
					row.Edges = append(row.Edges, GraphEdge{i, j, id})
					break
				}
			}
		}
		for _, parent := range c.Parents {
			for j, id := range g.Lanes {
				if id == parent {
					row.Edges = append(row.Edges, GraphEdge{lane, j, parent})
					break
				}
			}
		}
		rows = append(rows, row)
	}
	return rows
}
