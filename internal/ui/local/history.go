package local

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// Read once on entering Git Log. A failed/cancelled read waits for an explicit
// retry; rendering an empty or failed repository must not loop Git commands.
func (a *App) ensureHistory() {
	if a.Mode != "log" || a.historyAttempted || len(a.GraphRows) != 0 || a.Dispatch == nil {
		return
	}
	a.LoadHistory(false)
}

func (a *App) filteredHistory() []int {
	query := strings.ToLower(strings.TrimSpace(a.HistoryQuery))
	if a.historyVisible != nil && a.historyFilter == query && a.historyFilterCount == len(a.GraphRows) {
		return a.historyVisible
	}
	filterChanged := a.historyFilter != query
	a.historyVisible = make([]int, 0, len(a.GraphRows))
	for i, row := range a.GraphRows {
		commit := row.Commit
		if query == "" || strings.Contains(strings.ToLower(commit.Subject+" "+commit.Author+" "+commit.OID+" "+commit.Refs), query) {
			a.historyVisible = append(a.historyVisible, i)
		}
	}
	a.historyFilter, a.historyFilterCount = query, len(a.GraphRows)
	if filterChanged {
		a.HistoryList = ui.ListState{}
		a.historyListSelected = -1
	}
	return a.historyVisible
}

func (a *App) historyView(c *ui.Context) {
	a.HistoryBuiltRows = 0
	ui.Column(c).Fill().Gap(6).Children(func() {
		ui.Row(c).Padding(12).Gap(8).Children(func() {
			ui.Text(c, "Git Log").Bold().FontSize(16)
			ui.Textf(c, "History · %d commits", len(a.GraphRows)).FontSize(11).TextColor(c.Theme().TextMuted).Grow(1)
			ui.Button(c, "Refresh history").Disabled(a.Loading || a.Repository.Repository.RepositoryID == "").OnClick(func() { a.LoadHistory(false) })
		})
		ui.TextInput(c, &a.HistoryQuery).Label("Search commits").Placeholder("Message, author, hash or branch…").FillWidth()
		if len(a.GraphRows) == 0 {
			label := "No history loaded yet"
			switch {
			case a.Repository.Repository.RepositoryID == "":
				label = "Open a repository to browse history"
			case a.Loading:
				label = "Loading history…"
			case a.historyAttempted && a.Stale:
				label = "History unavailable · refresh to retry"
			case a.historyAttempted:
				label = "No commits yet"
			}
			ui.Text(c, label).Padding(16).TextColor(c.Theme().TextMuted)
			return
		}
		visible := a.filteredHistory()
		if len(visible) == 0 {
			ui.Text(c, "No matching commits").Padding(16).TextColor(c.Theme().TextMuted)
			return
		}
		a.HistoryList.Selected = &a.historyListSelected
		a.HistoryList.Key = func(i int) any { return a.GraphRows[visible[i]].Commit.OID }
		a.HistoryList.Label = func(i int) string { return a.GraphRows[visible[i]].Commit.Subject }
		list := ui.List(c, &a.HistoryList, len(visible), func(i int) {
			a.HistoryBuiltRows++
			a.graphRow(c, visible[i])
		}).Label("Commit history list").Grow(1).FillWidth()
		if list.Changed() && a.historyListSelected >= 0 && a.historyListSelected < len(visible) {
			a.HistorySelected = visible[a.historyListSelected]
			a.CommitInput = a.GraphRows[a.HistorySelected].Commit.OID
			a.CommitDiff()
		}
		ui.Row(c).Padding(8).Gap(8).Children(func() {
			ui.Textf(c, "%d commits", len(visible)).FontSize(10).TextColor(c.Theme().TextMuted).Grow(1)
			if a.HistoryPage.More {
				ui.Button(c, "Load more").Disabled(a.Loading).OnClick(func() { a.LoadHistory(true) })
			}
		})
	})
}
