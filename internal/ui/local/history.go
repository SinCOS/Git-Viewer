package local

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
)

func (a *App) ensureHistory() {
	if a.Mode != "log" || a.historyAttempted || len(a.GraphRows) != 0 || a.Dispatch == nil {
		return
	}
	a.LoadHistory(false)
}

func historyBranchLabel(b gitexec.BranchRecord) string {
	if strings.HasPrefix(b.Name, "refs/remotes/") {
		return strings.TrimPrefix(b.Name, "refs/remotes/") + " (remote)"
	}
	return strings.TrimPrefix(b.Name, "refs/heads/")
}

// Reachability is restricted to the loaded page(s); filtering never performs a
// write or an unbounded history read. Parent IDs outside those pages terminate.
func (a *App) historyReachable() map[string]bool {
	all := a.HistoryBranch == "" || a.HistoryBranch == "Show All"
	if all && (a.HistoryShowRemote || len(a.Branches) == 0) {
		return nil
	}
	parents := make(map[string][]string, len(a.GraphRows))
	for _, row := range a.GraphRows {
		parents[row.Commit.OID] = row.Commit.Parents
	}
	pending := []string{}
	for _, b := range a.Branches {
		if !a.HistoryShowRemote && strings.HasPrefix(b.Name, "refs/remotes/") {
			continue
		}
		if (all || historyBranchLabel(b) == a.HistoryBranch) && b.OID != "" {
			pending = append(pending, b.OID)
		}
	}
	reachable := map[string]bool{}
	for len(pending) > 0 {
		oid := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if reachable[oid] {
			continue
		}
		reachable[oid] = true
		pending = append(pending, parents[oid]...)
	}
	return reachable
}

func (a *App) filteredHistory() []int {
	query := strings.ToLower(strings.TrimSpace(a.HistoryQuery))
	if a.historyVisible != nil && a.historyFilter == query && a.historyFilterCount == len(a.GraphRows) && a.historyBranchFilter == a.HistoryBranch && a.historyRemoteFilter == a.HistoryShowRemote {
		return a.historyVisible
	}
	filterChanged := a.historyFilter != query || a.historyBranchFilter != a.HistoryBranch || a.historyRemoteFilter != a.HistoryShowRemote
	reachable := a.historyReachable()
	a.historyVisible = make([]int, 0, len(a.GraphRows))
	commits := make([]domain.CommitRecord, 0, len(a.GraphRows))
	for i, row := range a.GraphRows {
		commit := row.Commit
		if reachable != nil && !reachable[commit.OID] {
			continue
		}
		if query == "" || strings.Contains(strings.ToLower(commit.Subject+" "+commit.Author+" "+commit.OID+" "+commit.Refs), query) {
			a.historyVisible = append(a.historyVisible, i)
			commits = append(commits, commit)
		}
	}
	var graph domain.GraphLayout
	a.historyDisplayGraph = graph.Append(commits)
	a.historyFilter, a.historyFilterCount = query, len(a.GraphRows)
	a.historyBranchFilter, a.historyRemoteFilter = a.HistoryBranch, a.HistoryShowRemote
	if filterChanged {
		columns := a.HistoryList.Columns
		a.HistoryList = ui.ListState{Columns: columns}
		a.historyListSelected = -1
	}
	return a.historyVisible
}

func (a *App) historyView(c *ui.Context) {
	a.HistoryBuiltRows = 0
	ui.Column(c).Label("Git history workspace").Fill().Gap(4).Children(func() {
		ui.Row(c).Padding(8).Gap(8).Children(func() {
			ui.Text(c, "Git Log").Bold().FontSize(14)
			ui.Textf(c, "History · %d commits", len(a.GraphRows)).FontSize(11).TextColor(c.Theme().TextMuted).Grow(1)
			iconButton(c, "Commit details", "Show / hide commit details", logIcon).OnClick(func() { a.HistoryDetailsOpen = !a.HistoryDetailsOpen })
			iconButton(c, "Refresh history", "Refresh history", refreshIcon).Disabled(a.Loading || a.Repository.Repository.RepositoryID == "").OnClick(func() { a.LoadHistory(false) })
		})
		options := []string{"Show All"}
		for _, b := range a.Branches {
			if a.HistoryShowRemote || !strings.HasPrefix(b.Name, "refs/remotes/") {
				options = append(options, historyBranchLabel(b))
			}
		}
		if a.HistoryBranch == "" {
			a.HistoryBranch = "Show All"
		}
		found := false
		for _, option := range options {
			if option == a.HistoryBranch {
				found = true
			}
		}
		if !found {
			a.HistoryBranch = "Show All"
		}
		ui.Row(c).Padding(4, 8).Gap(8).Children(func() {
			ui.Text(c, "Branches:").FontSize(11)
			ui.Combobox(c, &a.HistoryBranch, options).Label("Branches").Width(210)
			ui.Checkbox(c, &a.HistoryShowRemote, "Show Remote Branches").FontSize(11)
			ui.TextInput(c, &a.HistoryQuery).Label("Search commits").Placeholder("Search loaded commits…").Grow(1).MinWidth(100)
		})
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
		lanes := 2
		for _, row := range a.historyDisplayGraph {
			lanes = max(lanes, row.Width)
		}
		graphWidth := max(float32(72), float32(lanes)*graphLanePitch+24)
		columns := []ui.TableColumn{
			{ID: "graph", Title: "Graph", Width: graphWidth, Fixed: true},
			{ID: "description", Title: "Description", MinWidth: 220},
			{ID: "date", Title: "Date", Width: 150, MinWidth: 130},
			{ID: "author", Title: "Author", Width: 110, MinWidth: 75},
			{ID: "commit", Title: "Commit", Width: 100, MinWidth: 85},
		}
		table := ui.Table(c, &a.HistoryList, columns, len(visible), func(i, col int) {
			if col == 0 {
				a.HistoryBuiltRows++
			}
			a.historyCell(c, i, col)
		}).Label("Commit history list").Grow(1).FillWidth()
		if table.Changed() && a.historyListSelected >= 0 && a.historyListSelected < len(visible) {
			a.HistorySelected = visible[a.historyListSelected]
			a.CommitInput = a.GraphRows[a.HistorySelected].Commit.OID
			a.CommitDiff()
		}
		ui.Row(c).Padding(6, 8).Gap(8).Children(func() {
			ui.Textf(c, "%d commits", len(visible)).FontSize(10).TextColor(c.Theme().TextMuted).Grow(1)
			if a.HistoryPage.More {
				ui.Button(c, "Load more").Disabled(a.Loading).OnClick(func() { a.LoadHistory(true) })
			}
		})
	})
}
