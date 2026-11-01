package local

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

func (a *App) ensureHistory() {
	repoID := a.Repository.Repository.RepositoryID
	if a.Mode != "log" || repoID == "" || len(a.GraphRows) != 0 || a.Dispatch == nil ||
		(a.historyAttempted && a.historyAttemptedRepoID == repoID) {
		return
	}
	a.LoadHistory(false)
}

func (a *App) filteredHistory() []int {
	query := strings.ToLower(strings.TrimSpace(a.HistoryQuery))
	if a.historyVisible != nil && a.historyFilter == query && a.historyFilterCount == len(a.GraphRows) &&
		a.historyFilterRepoID == a.Repository.Repository.RepositoryID {
		return a.historyVisible
	}
	filterChanged := a.historyFilter != query
	a.historyVisible = make([]int, 0, len(a.GraphRows))
	commits := make([]domain.CommitRecord, 0, len(a.GraphRows))
	for i, row := range a.GraphRows {
		commit := row.Commit
		if query == "" || strings.Contains(strings.ToLower(commit.Subject+" "+commit.Author+" "+commit.OID+" "+commit.Refs), query) {
			a.historyVisible = append(a.historyVisible, i)
			commits = append(commits, commit)
		}
	}
	var graph domain.GraphLayout
	a.historyDisplayGraph = graph.Append(commits)
	a.historyFilter, a.historyFilterCount = query, len(a.GraphRows)
	a.historyFilterRepoID = a.Repository.Repository.RepositoryID
	if filterChanged {
		a.HistoryList.ScrollTo(0, ui.Start)
		a.historyListSelected = -1
	}
	return a.historyVisible
}

func (a *App) historyView(c *ui.Context) {
	a.HistoryBuiltRows = 0
	ui.Column(c).Label("Git history workspace").Grow(1).FillWidth().MinHeight(0).Gap(4).Children(func() {
		ui.Row(c).Padding(8).Gap(8).Children(func() {
			ui.Text(c, "Git Log").Bold().FontSize(12)
			ui.Textf(c, "History · %d commits", len(a.GraphRows)).FontSize(11).TextColor(c.Theme().TextMuted).Grow(1)
			detailsButton := iconButton(c, "Commit details", "Show / hide commit details", logIcon).OnClick(func() { a.HistoryDetailsOpen = !a.HistoryDetailsOpen })
			if a.HistoryDetailsOpen {
				detailsButton.TextColor(c.Theme().Accent)
			}
			iconButton(c, "Refresh history", "Refresh history", refreshIcon).Disabled(a.Writing || a.DialogOpen || a.Repository.Repository.RepositoryID == "").OnClick(func() { a.LoadHistory(false) })
		})
		ui.Row(c).Padding(4, 8).Gap(8).Children(func() {
			ui.Text(c, historyScopeLabel(a.HistoryRef)).FontSize(11).NoWrap().MaxWidth(240).Tooltip(a.HistoryRef)
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
		a.HistoryList.Selection = nil
		a.HistoryList.CellSelect = false
		a.HistoryList.Key = func(i int) any { return a.GraphRows[visible[i]].Commit.OID }
		a.HistoryList.Label = func(i int) string { return a.GraphRows[visible[i]].Commit.Subject }
		lanes := 2
		for _, row := range a.historyDisplayGraph {
			lanes = max(lanes, row.Width)
		}
		graphWidth := max(float32(72), float32(lanes)*graphLanePitch+24)
		columns := []ui.TableColumn{
			{ID: "graph", Title: "Graph", Width: graphWidth, Fixed: true, RowHeader: true},
			{ID: "description", Title: "Description", MinWidth: 180},
			{ID: "branches", Title: "Branches", Width: 230, MinWidth: 100, MaxWidth: 360},
			{ID: "author", Title: "Author", Width: 110, MinWidth: 60, MaxWidth: 220},
			{ID: "date", Title: "Date", Width: 150, MinWidth: 105, MaxWidth: 190},
			{ID: "commit", Title: "Commit", Width: 100, MinWidth: 70, MaxWidth: 140},
		}
		a.HistoryList.CellMenu = func(m *ui.Menu, row, col int) {
			if row < 0 || row >= len(visible) || col == 0 {
				return
			}
			commit := a.GraphRows[visible[row]].Commit
			if m.Item("Open Changes").Chosen() {
				a.historyListSelected = row
				a.HistorySelected = visible[row]
				a.HistoryDetailsOpen = true
				a.CommitInput = commit.OID
				a.CommitDiff()
			}
			if m.Item("Create Branch Here…").Chosen() {
				a.openCommitBranchPrompt(commit.OID, "")
			}
			if m.Item("Cherry-pick Commit").Disabled(a.syncDisabled()).Chosen() {
				a.runCommitAction(domain.CherryPick, commit.OID, "")
			}
			if m.Item("Revert Commit").Disabled(a.syncDisabled()).Chosen() {
				a.runCommitAction(domain.Revert, commit.OID, "")
			}
			m.Separator()
			if m.Item("Copy cell").Chosen() {
				c.WriteClipboard(a.historyCellText(commit, col))
			}
			if m.Item("Copy commit hash").Chosen() {
				c.WriteClipboard(commit.OID)
			}
			if m.Item("Copy short hash").Chosen() {
				c.WriteClipboard(commit.OID[:min(8, len(commit.OID))])
			}
			if m.Item("Copy subject").Chosen() {
				c.WriteClipboard(commit.Subject)
			}
		}
		// Table rows use Accent/AccentText for selection. Scope neutral colors
		// to this table so selected commits stay readable in both appearances.
		baseTheme := c.Theme()
		tableTheme := *baseTheme
		tableTheme.Accent = baseTheme.SurfacePressed
		tableTheme.AccentText = baseTheme.Text
		c.SetTheme(&tableTheme)
		table := ui.Table(c.Key(a.revealGen), &a.HistoryList, columns, len(visible), func(i, col int) {
			if col == 0 {
				a.HistoryBuiltRows++
			}
			a.historyCell(c, i, col)
		}).Label("Commit history list").Grow(1).FillWidth()
		c.SetTheme(baseTheme)
		if table.Changed() && a.historyListSelected >= 0 && a.historyListSelected < len(visible) {
			selected := visible[a.historyListSelected]
			if a.HistorySelected != selected {
				a.HistorySelected = selected
				a.HistoryDetailsOpen = true
				a.CommitInput = a.GraphRows[a.HistorySelected].Commit.OID
				a.CommitDiff()
			}
		}
		ui.Row(c).Padding(6, 8).Gap(8).Children(func() {
			ui.Textf(c, "%d commits", len(visible)).FontSize(10).TextColor(c.Theme().TextMuted).Grow(1)
			if a.HistoryPage.More {
				ui.Button(c, "Load more").Disabled(a.Loading).OnClick(func() { a.LoadHistory(true) })
			}
		})
	})
}

func (a *App) historyCellText(commit domain.CommitRecord, col int) string {
	switch col {
	case 1:
		return commit.Subject
	case 2:
		return strings.Join(a.historyRefs(commit.Refs), ", ")
	case 3:
		return commit.Author
	case 4:
		return commit.AuthoredAt.Format("02 Jan 2006 15:04")
	case 5:
		return commit.OID
	default:
		return ""
	}
}
