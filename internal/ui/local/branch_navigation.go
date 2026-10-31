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

// treeRow is one rendered row of the Git Log side panel: a section header, a
// branch folder, a branch, a tag, or a stash entry.
type treeRow struct {
	key, label string
	depth      int
	count      int
	folder     bool
	section    bool
	branch     *gitexec.BranchRecord
	tag        *gitexec.TagRecord
	stash      *gitexec.StashRecord
}

// treeRows builds the four side-panel sections — Local, Remote, Tags and
// Stashes. The query filters every section and ignores collapse state; an
// empty query honors it. Local and Remote keep the folder hierarchy of full
// ref paths, so identically named branches stay independent.
func treeRows(branches []gitexec.BranchRecord, tags []gitexec.TagRecord, stashes []gitexec.StashRecord, query string, collapsed map[string]bool) []treeRow {
	query = strings.ToLower(strings.TrimSpace(query))
	local := &branchNode{key: "Local", label: "Local", children: map[string]*branchNode{}}
	remote := &branchNode{key: "Remote", label: "Remote", children: map[string]*branchNode{}}
	for _, b := range branches {
		if b.Symbolic != "" {
			continue
		}
		root, name := local, strings.TrimPrefix(b.Name, "refs/heads/")
		if strings.HasPrefix(b.Name, "refs/remotes/") {
			root, name = remote, strings.TrimPrefix(b.Name, "refs/remotes/")
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
	matchedTags, matchedStashes := []gitexec.TagRecord{}, []gitexec.StashRecord{}
	for _, t := range tags {
		if query == "" || strings.Contains(strings.ToLower(strings.TrimPrefix(t.Name, "refs/tags/")), query) {
			matchedTags = append(matchedTags, t)
		}
	}
	for _, s := range stashes {
		if query == "" || strings.Contains(strings.ToLower(s.Subject), query) {
			matchedStashes = append(matchedStashes, s)
		}
	}
	var rows []treeRow
	var emitNode func(*branchNode, int)
	emitNode = func(node *branchNode, depth int) {
		rows = append(rows, treeRow{key: node.key, label: node.label, depth: depth, folder: node.branch == nil, branch: node.branch})
		if node.branch == nil && collapsed[node.key] && query == "" {
			return
		}
		for _, child := range sortedChildren(node) {
			emitNode(child, depth+1)
		}
	}
	section := func(key, label string, count int, emit func()) {
		if query != "" && count == 0 {
			return
		}
		rows = append(rows, treeRow{key: "section:" + key, label: label, count: count, folder: true, section: true})
		if count == 0 {
			return
		}
		if query == "" && collapsed["section:"+key] {
			return
		}
		emit()
	}
	section("Local", "Local", leaves(local), func() {
		for _, child := range sortedChildren(local) {
			emitNode(child, 1)
		}
	})
	section("Remote", "Remote", leaves(remote), func() {
		for _, child := range sortedChildren(remote) {
			emitNode(child, 1)
		}
	})
	section("Tags", "Tags", len(matchedTags), func() {
		for i := range matchedTags {
			tag := matchedTags[i]
			rows = append(rows, treeRow{key: tag.Name, label: strings.TrimPrefix(tag.Name, "refs/tags/"), depth: 1, tag: &tag})
		}
	})
	section("Stashes", "Stashes", len(matchedStashes), func() {
		for i := range matchedStashes {
			stash := matchedStashes[i]
			rows = append(rows, treeRow{key: stash.Ref, label: stash.Subject, depth: 1, stash: &stash})
		}
	})
	return rows
}

func sortedChildren(node *branchNode) []*branchNode {
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
	return children
}

func leaves(node *branchNode) int {
	if node.branch != nil {
		return 1
	}
	count := 0
	for _, child := range node.children {
		count += leaves(child)
	}
	return count
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
	if a.Writing || a.DialogOpen || a.Dispatch == nil || a.Repository.Repository.RepositoryID == "" {
		return
	}
	if a.HistoryRef == ref && !a.Stale {
		return
	}
	a.loadHistory(false, ref)
}

func (a *App) branchNavigation(c *ui.Context) {
	a.branchRowsBuilt = 0
	ui.Column(c).Label("Branch navigation").Fill().Gap(4).Children(func() {
		ui.Row(c).Padding(8).FillWidth().Children(func() {
			ui.TextInput(c, &a.BranchQuery).Label("Search branches").Placeholder("Search branches…").Width(224).MaxWidth(224).MinWidth(0).Shrink(1)
		})
		disabled := a.Writing || a.DialogOpen || a.Repository.Repository.RepositoryID == ""
		ui.Row(c).Padding(0, 8).Children(func() {
			button := iconButton(c, "All Branches", "All Branches", treeIcon).Disabled(disabled).OnClick(func() { a.selectHistoryRef("") })
			if a.HistoryRef == "" {
				button.Background(c.Theme().SurfacePressed).TextColor(c.Theme().Accent)
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
			FillWidth().Height(30).Padding(4, 8, 4, 8).Gap(6).
			Background(ui.Transparent).Cursor(ui.CursorPointer).Disabled(disabled).
			OnClick(func() { a.selectHistoryRef("HEAD") })
		if a.HistoryRef == "HEAD" {
			head.Background(c.Theme().SurfacePressed)
		}
		head.Children(func() {
			ui.Icon(c, refIcon).Size(16, 16).Shrink(0).TextColor(c.Theme().Warning)
			ui.Text(c, headLabel).FontSize(12).SingleLine().Grow(1).MinWidth(0)
		})
		rows := treeRows(a.Branches, a.Tags, a.Stashes, a.BranchQuery, a.branchCollapsed)
		a.branchList.Key = func(i int) any { return rows[i].key }
		a.branchList.Label = func(i int) string { return rows[i].label }
		ui.List(c, &a.branchList, len(rows), func(i int) {
			a.branchRowsBuilt++
			row := rows[i]
			element := ui.Row(c).Label(a.treeRowLabel(row)).Height(30).Padding(4, 8, 4, 8+float32(row.depth)*16).Gap(6).Tooltip(a.treeRowTooltip(row)).Cursor(ui.CursorPointer)
			switch {
			case row.section:
				key := "section:" + row.key
				element.OnClick(func() { a.toggleTreeRow(key) })
			case row.folder:
				element.OnClick(func() { a.toggleTreeRow(row.key) })
			case row.branch != nil:
				a.mountBranchRow(c, element, row, disabled)
			case row.tag != nil:
				// Tags have no write operations; the context menu only copies.
				element.ContextMenu(func(m *ui.Menu) {
					if m.Item("Copy tag name").Chosen() {
						c.WriteClipboard(row.tag.Name)
					}
				})
			case row.stash != nil:
				element.ContextMenu(func(m *ui.Menu) {
					if m.Item("Copy stash message").Chosen() {
						c.WriteClipboard(row.stash.Subject)
					}
				})
			}
			element.Children(func() { a.treeRowContent(c, row) })
		}).Label("Branch tree").Grow(1).FillWidth()
		if len(rows) == 0 {
			ui.Text(c, "No matching branches").Padding(8).TextColor(c.Theme().TextMuted)
		}
	})
}

func (a *App) treeRowLabel(row treeRow) string {
	switch {
	case row.section:
		return "Toggle section " + row.label
	case row.folder:
		return "Toggle branch folder " + row.key
	case row.branch != nil:
		return "Browse " + row.key
	case row.tag != nil:
		return "Tag " + row.key
	default:
		return "Stash " + row.key
	}
}

func (a *App) treeRowTooltip(row treeRow) string {
	switch {
	case row.branch != nil:
		return row.branch.Name + "\n" + branchTracking(*row.branch)
	case row.tag != nil:
		return row.tag.Name + " · " + row.tag.OID
	case row.stash != nil:
		return row.stash.Ref + "\n" + row.stash.Subject
	default:
		return row.key
	}
}

func (a *App) toggleTreeRow(key string) {
	if strings.TrimSpace(a.BranchQuery) != "" {
		return
	}
	if a.branchCollapsed == nil {
		a.branchCollapsed = map[string]bool{}
	}
	a.branchCollapsed[key] = !a.branchCollapsed[key]
}

// mountBranchRow wires the branch row's click (scope the history) and its
// context menu. Switching branches goes through the confirmed write flow;
// remote branches stay read-only as in the branch popover.
func (a *App) mountBranchRow(c *ui.Context, element ui.Element, row treeRow, disabled bool) {
	branch := row.branch
	name := strings.TrimPrefix(strings.TrimPrefix(branch.Name, "refs/heads/"), "refs/remotes/")
	element.Disabled(disabled).OnClick(func() { a.selectHistoryRef(branch.Name) })
	if a.HistoryRef == branch.Name {
		element.Background(c.Theme().SurfacePressed)
	}
	element.ContextMenu(func(m *ui.Menu) {
		if m.Item("Show history for " + name).Chosen() {
			a.selectHistoryRef(branch.Name)
		}
		if m.Item("Reveal HEAD in Graph").Chosen() {
			a.selectBranchAndReveal(branch.Name, branch.OID)
		}
		switchItem := m.Item("Switch to " + name)
		if strings.HasPrefix(branch.Name, "refs/remotes/") {
			switchItem.Disabled(true)
		}
		if switchItem.Chosen() {
			a.BranchInput = name
			a.RunWrite(domain.SwitchBranch)
		}
		if m.Item("Create branch here…").Chosen() {
			a.openCommitBranchPrompt(branch.OID, branch.Name)
		}
		m.Separator()
		if m.Item("Copy branch name").Chosen() {
			c.WriteClipboard(name)
		}
		if m.Item("Copy full ref").Chosen() {
			c.WriteClipboard(branch.Name)
		}
		if m.Item("Copy commit hash").Chosen() {
			c.WriteClipboard(branch.OID)
		}
	})
}

func (a *App) treeRowContent(c *ui.Context, row treeRow) {
	switch {
	case row.section || row.folder:
		arrow, icon := "▾", folderOpenIcon
		if a.branchCollapsed[row.key] {
			arrow, icon = "▸", folderIcon
		}
		ui.Text(c, arrow).Width(10).FontSize(11).TextColor(c.Theme().TextMuted)
		ui.Icon(c, icon).Size(16, 16).Shrink(0).TextColor(c.Theme().TextMuted)
		label := ui.Text(c, row.label).FontSize(12).SingleLine().Grow(1).MinWidth(0)
		if row.section {
			label.Bold()
		}
		if row.section && row.count > 0 {
			ui.Textf(c, "%d", row.count).FontSize(10).TextColor(c.Theme().TextMuted)
		}
	case row.branch != nil:
		icon, color := refIcon, c.Theme().Accent
		if row.branch.Current {
			icon, color = starIcon, c.Theme().Warning
		}
		ui.Icon(c, icon).Size(16, 16).Shrink(0).TextColor(color)
		text := ui.Text(c, row.label).FontSize(12).SingleLine().Grow(1).MinWidth(0)
		if row.branch.Current {
			text.Bold()
		}
		if row.branch.Upstream != "" {
			ui.Text(c, branchTracking(*row.branch)).FontSize(10).SingleLine().MaxWidth(130).TextColor(c.Theme().TextMuted)
		}
	case row.tag != nil:
		ui.Icon(c, refIcon).Size(16, 16).Shrink(0).TextColor(c.Theme().Success)
		ui.Text(c, row.label).FontSize(12).SingleLine().Grow(1).MinWidth(0)
	default:
		ui.Icon(c, stashIcon).Size(16, 16).Shrink(0).TextColor(c.Theme().TextMuted)
		ui.Text(c, row.label).FontSize(12).SingleLine().Grow(1).MinWidth(0)
	}
}
