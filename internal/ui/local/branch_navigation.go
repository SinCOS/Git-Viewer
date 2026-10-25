package local

import (
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
)

type branchNode struct {
	key, label string
	branch     *gitexec.BranchRecord
	children   map[string]*branchNode
}
type branchRow struct {
	node  *branchNode
	depth int
}

// Full ref paths keep identically named local/remote branches independent.
func branchTreeRows(branches []gitexec.BranchRecord, query string, collapsed map[string]bool) []branchRow {
	query = strings.ToLower(strings.TrimSpace(query))
	roots := []*branchNode{{key: "Local", label: "Local", children: map[string]*branchNode{}}, {key: "Remote", label: "Remote", children: map[string]*branchNode{}}}
	for _, b := range branches {
		if b.Symbolic != "" {
			continue
		}
		root, name := roots[0], strings.TrimPrefix(b.Name, "refs/heads/")
		if strings.HasPrefix(b.Name, "refs/remotes/") {
			root, name = roots[1], strings.TrimPrefix(b.Name, "refs/remotes/")
		}
		if query != "" && !strings.Contains(strings.ToLower(name), query) {
			continue
		}
		parts := strings.Split(name, "/")
		parent := root
		for i, part := range parts {
			key := parent.key + "/" + part
			node := parent.children[part]
			if node == nil {
				node = &branchNode{key: key, label: part, children: map[string]*branchNode{}}
				parent.children[part] = node
			}
			if i == len(parts)-1 {
				copy := b
				node.branch = &copy
				node.key = b.Name
			}
			parent = node
		}
	}
	var rows []branchRow
	var walk func(*branchNode, int)
	walk = func(node *branchNode, depth int) {
		rows = append(rows, branchRow{node, depth})
		if collapsed[node.key] && query == "" {
			return
		}
		children := make([]*branchNode, 0, len(node.children))
		for _, child := range node.children {
			children = append(children, child)
		}
		sort.Slice(children, func(i, j int) bool {
			if (children[i].branch == nil) != (children[j].branch == nil) {
				return children[i].branch == nil
			}
			return children[i].label < children[j].label
		})
		for _, child := range children {
			walk(child, depth+1)
		}
	}
	for _, root := range roots {
		if query == "" || len(root.children) > 0 {
			walk(root, 0)
		}
	}
	return rows
}

func historyScopeLabel(ref string) string {
	if ref == "" {
		return "All Branches"
	}
	if ref == "HEAD" {
		return "HEAD"
	}
	if strings.HasPrefix(ref, "refs/remotes/") {
		return strings.TrimPrefix(ref, "refs/remotes/") + " (remote)"
	}
	return strings.TrimPrefix(ref, "refs/heads/")
}

func branchTracking(b gitexec.BranchRecord) string {
	if b.Upstream == "" {
		return ""
	}
	track := strings.Trim(b.Tracking, "[]")
	track = strings.ReplaceAll(track, "ahead ", "↑")
	track = strings.ReplaceAll(track, "behind ", "↓")
	track = strings.ReplaceAll(track, ", ", " ")
	return strings.TrimSpace(track + " " + strings.TrimPrefix(b.Upstream, "refs/remotes/"))
}

func (a *App) selectHistoryRef(ref string) {
	if a.Loading || a.Writing || a.DialogOpen || a.Dispatch == nil || a.Repository.Repository.RepositoryID == "" {
		return
	}
	if a.HistoryRef == ref && !a.Stale {
		return
	}
	a.HistoryRef = ref
	a.HistoryPage = gitexec.HistoryPage{}
	a.Graph = domain.GraphLayout{}
	a.GraphRows = nil
	a.historyVisible = nil
	a.HistorySelected = -1
	a.CommitInput = ""
	a.CommitDetails = gitexec.CommitDetails{}
	a.LoadHistory(false)
}

func (a *App) branchNavigation(c *ui.Context) {
	a.branchRowsBuilt = 0
	ui.Column(c).Label("Branch navigation").Fill().Gap(4).Children(func() {
		ui.Row(c).Padding(8).FillWidth().Children(func() {
			ui.TextInput(c, &a.BranchQuery).Label("Search branches").Placeholder("Search branches…").Width(224).MaxWidth(224).MinWidth(0).Shrink(1)
		})
		disabled := a.Loading || a.Writing || a.DialogOpen || a.Repository.Repository.RepositoryID == ""
		ui.Row(c).Padding(0, 8).Children(func() {
			button := iconButton(c, "All Branches", "All Branches", treeIcon).Disabled(disabled).OnClick(func() { a.selectHistoryRef("") })
			if a.HistoryRef == "" {
				button.Background(c.Theme().Surface).TextColor(c.Theme().Accent)
			}
		})
		headLabel := "HEAD (Current Branch)"
		if a.historyAttempted && !a.Loading {
			current := false
			for _, b := range a.Branches {
				current = current || b.Current
			}
			if !current {
				headLabel = "HEAD"
			}
		}
		head := ui.ButtonBase(c).Label("HEAD (Current Branch)").Tooltip(headLabel).
			FillWidth().Height(30).Padding(4, 8, 4, 24).Gap(6).
			Background(ui.Transparent).Cursor(ui.CursorPointer).Disabled(disabled).
			OnClick(func() { a.selectHistoryRef("HEAD") })
		if a.HistoryRef == "HEAD" {
			head.Background(c.Theme().Surface)
		}
		head.Children(func() {
			ui.Icon(c, refIcon).Size(16, 16).Shrink(0).TextColor(c.Theme().Warning)
			ui.Text(c, headLabel).FontSize(12).SingleLine().Grow(1).MinWidth(0)
		})
		rows := branchTreeRows(a.Branches, a.BranchQuery, a.branchCollapsed)
		a.branchList.Key = func(i int) any { return rows[i].node.key }
		a.branchList.Label = func(i int) string { return rows[i].node.label }
		ui.List(c, &a.branchList, len(rows), func(i int) {
			a.branchRowsBuilt++
			row := rows[i]
			node := row.node
			folder := node.branch == nil
			label := "Browse " + node.key
			if folder {
				label = "Toggle branch folder " + node.key
			}
			element := ui.Row(c).Label(label).Height(30).Padding(4, 8, 4, 8+float32(row.depth)*16).Gap(6).Tooltip(node.key).Cursor(ui.CursorPointer)
			if folder {
				element.OnClick(func() {
					if strings.TrimSpace(a.BranchQuery) != "" {
						return
					}
					if a.branchCollapsed == nil {
						a.branchCollapsed = map[string]bool{}
					}
					a.branchCollapsed[node.key] = !a.branchCollapsed[node.key]
				})
			} else {
				element.Disabled(disabled).OnClick(func() { a.selectHistoryRef(node.branch.Name) })
				if a.HistoryRef == node.branch.Name {
					element.Background(c.Theme().Surface)
				}
				element.Tooltip(node.branch.Name + "\n" + branchTracking(*node.branch))
			}
			element.Children(func() {
				if folder {
					arrow, icon := "▾", folderOpenIcon
					if a.branchCollapsed[node.key] && strings.TrimSpace(a.BranchQuery) == "" {
						arrow, icon = "▸", folderIcon
					}
					ui.Text(c, arrow).Width(10).FontSize(11).TextColor(c.Theme().TextMuted)
					ui.Icon(c, icon).Size(16, 16).Shrink(0).TextColor(c.Theme().TextMuted)
				} else {
					icon, color := refIcon, c.Theme().Accent
					if node.branch.Current {
						icon, color = starIcon, c.Theme().Warning
					}
					ui.Icon(c, icon).Size(16, 16).Shrink(0).TextColor(color)
				}
				text := ui.Text(c, node.label).FontSize(12).SingleLine().Grow(1).MinWidth(0)
				if node.branch != nil && node.branch.Current {
					text.Bold()
				}
				if node.branch != nil && node.branch.Upstream != "" {
					ui.Text(c, branchTracking(*node.branch)).FontSize(10).SingleLine().MaxWidth(130).TextColor(c.Theme().TextMuted)
				}
			})
		}).Label("Branch tree").Grow(1).FillWidth()
		if len(rows) == 0 {
			ui.Text(c, "No matching branches").Padding(8).TextColor(c.Theme().TextMuted)
		}
	})
}
