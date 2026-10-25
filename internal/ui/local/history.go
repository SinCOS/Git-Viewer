package local

import (
	"github.com/egoist/mygo/ui"
)

// history.go owns the History mode panel: paged commit list with graph rows
// drawn by graph.go and the commit summary. Row selection loads a commit diff.
func (a *App) historyView(c *ui.Context) {
	a.HistoryBuiltRows = 0
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Gap(4).Children(func() {
			ui.Textf(c, "History · %d commits", len(a.GraphRows)).Bold()
			if len(a.GraphRows) == 0 {
				ui.Button(c, "Load history").Disabled(a.Loading || a.Repository.Repository.RepositoryID == "").OnClick(func() { a.LoadHistory(false) })
			}
			ui.Button(c, "Load more").Disabled(a.Loading || !a.HistoryPage.More).OnClick(func() { a.LoadHistory(true) })
		})
		if len(a.GraphRows) == 0 {
			ui.Text(c, "No history loaded yet")
			return
		}
		a.HistoryList.Selected = &a.HistorySelected
		a.HistoryList.Key = func(i int) any { return a.GraphRows[i].Commit.OID }
		a.HistoryList.Label = func(i int) string { return a.GraphRows[i].Commit.Subject }
		ui.ScrollHorizontal(c).TrackScroll(&a.HistoryScroll).Grow(1).FillWidth().Children(func() {
			list := ui.List(c, &a.HistoryList, len(a.GraphRows), func(i int) {
				a.HistoryBuiltRows++
				a.graphRow(c, i)
			}).Width(float32(max(2, a.HistoryLanes))*16 + 700).FillHeight()
			if list.Changed() && a.HistorySelected >= 0 && a.HistorySelected < len(a.GraphRows) {
				a.CommitInput = a.GraphRows[a.HistorySelected].Commit.OID
				a.CommitDiff()
			}
		})
	})
}
