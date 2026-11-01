package local

import (
	"path"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

var changeFileIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M4 2.5h7l5 5v10H4zM11 2.5v5h5M7 11h6M7 14h6"/></svg>`))
var changeMinusIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M4 10h12"/></svg>`))
var changeDiscardIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3"><path d="M5 3 2 6l3 3M2 6h7a4 4 0 0 1 0 8H6"/></svg>`))
var changeTrashIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M3 5h14M7 5V3h6v2M5 5l1 12h8l1-12M8 8v6m4-6v6"/></svg>`))

// A virtual row is a group heading, a directory, or a file. entry always refers
// to Entries, never to the rendered row index (headings change those indices).
type changeRow struct {
	group    string
	count    int
	dir      bool
	dirPath  string
	dirGroup string
	depth    int
	entry    int
}

func changeGroupLabel(group string) string {
	switch group {
	case "Staged":
		return "Staged Changes"
	case "Unstaged":
		return "Changes"
	case "Untracked":
		return "Untracked Files"
	default:
		return "Merge Changes"
	}
}

func belongsToChangeGroup(entry domain.StatusEntry, group string) bool {
	return entry.Group == group || group == "Unstaged" && entry.Group == "Untracked"
}

func (a *App) statusView(c *ui.Context) {
	unavailable := a.syncDisabled()
	ui.Column(c).Label("Local Changes").Fill().Gap(2).Children(func() {
		if a.Repository.Repository.RepositoryID == "" {
			ui.Text(c, "Open a repository to view changes").Padding(12).TextColor(c.Theme().TextMuted)
			return
		}
		if len(a.Entries) == 0 {
			ui.Row(c).Height(26).Padding(0, 8).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, commitIcon).Size(14, 14).TextColor(c.Theme().TextMuted).Shrink(0)
				ui.Text(c, "No changes").FontSize(12).TextColor(c.Theme().TextMuted)
			})
			return
		}
		rows := a.changeRows()
		controlsGroup := ""
		for _, group := range []string{"Unstaged", "Staged"} {
			for _, row := range rows {
				if row.group == group {
					controlsGroup = group
					break
				}
			}
			if controlsGroup != "" {
				break
			}
		}
		if a.Selected != a.statusRenderedSelection {
			a.StatusRowSelected = -1
			for i, row := range rows {
				if row.entry == a.Selected && row.entry >= 0 {
					a.StatusRowSelected = i
					break
				}
			}
			a.statusRenderedSelection = a.Selected
		}
		a.StatusList.Selected = &a.StatusRowSelected
		a.StatusList.Key = func(i int) any {
			row := rows[i]
			if row.group != "" {
				return "group\x00" + row.group
			}
			if row.dir {
				return "dir\x00" + row.dirGroup + "\x00" + row.dirPath
			}
			return entryKey(a.Entries[row.entry])
		}
		a.StatusList.Label = func(i int) string {
			row := rows[i]
			if row.group != "" {
				return changeGroupLabel(row.group)
			}
			if row.dir {
				return row.dirPath
			}
			return pathLabel(a.Entries[row.entry].Change.Path)
		}
		// Match Git Log's neutral selection without changing the surrounding UI.
		baseTheme := c.Theme()
		listTheme := *baseTheme
		listTheme.Accent = baseTheme.Surface.Mix(baseTheme.Accent, 0.14)
		listTheme.AccentText = baseTheme.Text
		c.SetTheme(&listTheme)
		list := ui.List(c, &a.StatusList, len(rows), func(i int) {
			row := rows[i]
			if row.group != "" {
				key := "group\x00" + row.group
				ui.Box(c).Label(changeGroupLabel(row.group) + " controls").Height(28).FillWidth().Children(func() {
					ui.Row(c).Height(28).FillWidth().Padding(0, 8).Gap(4).AlignItems(ui.Center).Background(c.Theme().Surface).Children(func() {
						ui.Row(c).Label("Toggle " + changeGroupLabel(row.group)).Gap(5).AlignItems(ui.Center).OnClick(func() { a.toggleChanges(key) }).Children(func() {
							a.changeDisclosure(c, key)
							ui.Text(c, changeGroupLabel(row.group)).Bold().FontSize(12).NoWrap()
							ui.Badge(c, strconv.Itoa(row.count)).Height(16).Padding(0, 5).FontSize(11).
								Background(c.Theme().SurfacePressed).TextColor(c.Theme().TextMuted)
						})
						ui.Box(c).Grow(1)
						if row.group == controlsGroup {
							listButton := sourceControlIconButton(c, "List", "List view", listIcon).OnClick(func() { a.setChangesTree(false) })
							treeButton := sourceControlIconButton(c, "Tree", "Tree view", treeIcon).OnClick(func() { a.setChangesTree(true) })
							if a.ChangesTree {
								treeButton.Background(c.Theme().SurfacePressed)
							} else {
								listButton.Background(c.Theme().SurfacePressed)
							}
						}
						if row.group == "Unstaged" || row.group == "Untracked" {
							discardPaths, discardReason := a.discardAllPaths()
							sourceControlIconButton(c, "Discard all changes", "Discard all tracked changes", changeDiscardIcon).
								Disabled(unavailable || discardReason != "" || len(discardPaths) == 0).OnClick(a.discardAll)
							sourceControlIconButton(c, "Stage all "+changeGroupLabel(row.group), "Stage files in this group", plusIcon).
								Disabled(unavailable).OnClick(func() { a.stageChangeGroup(row.group) })
						}
						if row.group == "Staged" {
							sourceControlIconButton(c, "Unstage all changes", "Unstage all changes", changeMinusIcon).
								Disabled(unavailable).OnClick(func() { a.runBatchChanges(domain.Unstage) })
						}
					})
				})
				return
			}
			if row.dir {
				key := "dir\x00" + row.dirGroup + "\x00" + row.dirPath
				folder := folderOpenIcon
				if a.changesCollapsed[key] {
					folder = folderIcon
				}
				ui.Row(c).Label("Toggle directory "+row.dirGroup+" "+pathLabel(row.dirPath)).OnClick(func() { a.toggleChanges(key) }).Height(24).Padding(0, float32(8+min(row.depth, 8)*12)).Gap(6).Children(func() {
					ui.Text(c, a.changeArrow(key)).FontSize(11).TextColor(c.Theme().TextMuted)
					ui.Icon(c, folder).Size(14, 14).Shrink(0).TextColor(c.Theme().TextMuted)
					ui.Text(c, displayPath(path.Base(row.dirPath))).FontSize(12).NoWrap()
					if row.depth > 0 {
						ui.Badge(c, strconv.Itoa(row.count))
					}
				})
				return
			}
			entry := a.Entries[row.entry]
			staged := entry.Scope != domain.StagedDiff
			actionName := "Stage file"
			if !staged {
				actionName = "Unstage file"
			}
			fileRow := ui.Row(c).Height(26).Padding(0, float32(8+min(row.depth, 8)*12)).Gap(6).Tooltip(pathLabel(entry.Change.Path))
			fileMenu := func(m *ui.Menu) {
				if entry.Change.Conflicted && m.Item("Resolve Conflict…").Chosen() {
					a.OpenConflict(entry.Change.Path)
				}
				if m.Item(actionName).Disabled(unavailable).Chosen() {
					a.stageEntry(row.entry, staged)
				}
				if m.Item("Discard changes").Disabled(unavailable || !canDiscardEntry(entry)).Chosen() {
					a.discardEntry(row.entry)
				}
				m.Separator()
				if m.Item("Copy path").Chosen() {
					c.WriteClipboard(entry.Change.Path)
				}
			}
			fileRow.ContextMenu(fileMenu)
			fileRow.Children(func() {
				status := entry.Change.WorktreeStatus
				if entry.Scope == domain.StagedDiff {
					status = entry.Change.IndexStatus
				}
				color := c.Theme().Warning
				if entry.Change.Untracked {
					status = "U"
					color = c.Theme().Success
				}
				if status == "A" {
					color = c.Theme().Success
				} else if status == "D" {
					color = c.Theme().Danger
				} else if status == "R" || status == "C" {
					color = c.Theme().Accent
				}
				if entry.Change.Conflicted {
					status = "!"
					color = c.Theme().Danger
				}
				ui.Icon(c, changeFileIcon).Size(14, 14).Shrink(0).TextColor(c.Theme().TextMuted)
				ui.Text(c, displayPath(path.Base(entry.Change.Path))).FontSize(12).SingleLine().
					Grow(1).MinWidth(0).Tooltip(pathLabel(entry.Change.Path))
				if !a.ChangesTree && path.Dir(entry.Change.Path) != "." {
					ui.Text(c, displayPath(path.Dir(entry.Change.Path))).FontSize(11).SingleLine().
						MaxWidth(90).TextColor(c.Theme().TextMuted).Tooltip(pathLabel(entry.Change.Path))
				}
				actionIcon := plusIcon
				if entry.Scope == domain.StagedDiff {
					actionIcon = changeMinusIcon
				}
				if entry.Scope == domain.WorktreeDiff && !entry.Change.Untracked && !entry.Change.Conflicted {
					sourceControlIconButton(c, "Discard changes for "+pathLabel(entry.Change.Path), "Discard Changes", changeDiscardIcon).
						Disabled(unavailable || !canDiscardEntry(entry)).OnClick(func() { a.discardEntry(row.entry) })
				}
				sourceControlIconButton(c, actionName+" "+pathLabel(entry.Change.Path), actionName, actionIcon).
					Disabled(unavailable).OnClick(func() { a.stageEntry(row.entry, staged) })
				ui.Text(c, status).Bold().Font("monospace").FontSize(11).TextColor(color).Width(12).NoWrap()
			})
		}).Grow(1).FillWidth()
		c.SetTheme(baseTheme)
		if list.Changed() && a.StatusRowSelected >= 0 && a.StatusRowSelected < len(rows) {
			if entry := rows[a.StatusRowSelected].entry; entry >= 0 {
				a.Select(entry)
				a.statusRenderedSelection = a.Selected
			}
		}
	})
}

// displayPath keeps ordinary names readable, but escapes control characters so
// special Git paths cannot create fake file rows or misleading labels.
func displayPath(name string) string {
	for _, r := range name {
		if r < 32 || r == 127 {
			return pathLabel(name)
		}
	}
	return name
}

func (a *App) changeRows() []changeRow {
	var rows []changeRow
	for _, group := range []string{"Conflicted", "Staged", "Unstaged"} {
		count := 0
		for _, e := range a.Entries {
			if belongsToChangeGroup(e, group) {
				count++
			}
		}
		if count == 0 {
			continue
		}
		rows = append(rows, changeRow{group: group, count: count, entry: -1})
		if a.changesCollapsed["group\x00"+group] {
			continue
		}
		if !a.ChangesTree {
			for i, e := range a.Entries {
				if belongsToChangeGroup(e, group) {
					rows = append(rows, changeRow{entry: i})
				}
			}
			continue
		}
		// Build a directory hierarchy per group, preserving Git's entry order.
		type treeItem struct {
			dir   string
			entry int
		}
		children := map[string][]treeItem{}
		seen := map[string]bool{}
		counts := map[string]int{}
		for i, e := range a.Entries {
			if !belongsToChangeGroup(e, group) {
				continue
			}
			dir := path.Dir(e.Change.Path)
			parent := "."
			if dir != "." {
				for _, component := range strings.Split(dir, "/") {
					child := path.Join(parent, component)
					counts[child]++
					if !seen[child] {
						seen[child] = true
						children[parent] = append(children[parent], treeItem{dir: child, entry: -1})
					}
					parent = child
				}
			}
			children[parent] = append(children[parent], treeItem{entry: i})
		}
		var appendTree func(string, int)
		appendTree = func(parent string, depth int) {
			for _, item := range children[parent] {
				if item.dir == "" {
					rows = append(rows, changeRow{entry: item.entry, depth: depth})
					continue
				}
				rows = append(rows, changeRow{dir: true, dirPath: item.dir, dirGroup: group, entry: -1, depth: depth, count: counts[item.dir]})
				if !a.changesCollapsed["dir\x00"+group+"\x00"+item.dir] {
					appendTree(item.dir, depth+1)
				}
			}
		}
		appendTree(".", 0)
	}
	return rows
}

func (a *App) changeArrow(key string) string {
	if a.changesCollapsed[key] {
		return "›"
	}
	return "⌄"
}

func (a *App) toggleChanges(key string) {
	if a.changesCollapsed == nil {
		a.changesCollapsed = make(map[string]bool)
	}
	a.changesCollapsed[key] = !a.changesCollapsed[key]
	a.resetChangeRows()
}

func (a *App) resetChangeRows() {
	a.StatusList = ui.ListState{}
	a.StatusRowSelected = -1
	a.statusRenderedSelection = -2
}

func (a *App) setChangesTree(tree bool) {
	a.ChangesTree = tree
	a.resetChangeRows()
}

func (a *App) hasUnstagedChanges() bool {
	for _, entry := range a.Entries {
		if entry.Scope != domain.StagedDiff {
			return true
		}
	}
	return false
}

// stageChangeGroup captures the displayed group's exact paths before confirmation.
func (a *App) stageChangeGroup(group string) {
	if a.syncDisabled() || (group != "Unstaged" && group != "Untracked") {
		return
	}
	request := domain.GitOperation{RepositoryID: a.Repository.Repository.RepositoryID, Operation: domain.Stage}
	seen := make(map[string]bool)
	for _, entry := range a.Entries {
		if belongsToChangeGroup(entry, group) && !seen[entry.Change.Path] {
			seen[entry.Change.Path] = true
			request.Paths = append(request.Paths, entry.Change.Path)
		}
	}
	if len(request.Paths) > 0 {
		a.confirmWrite(request, false)
	}
}

func canDiscardEntry(entry domain.StatusEntry) bool {
	return entry.Scope == domain.WorktreeDiff && !entry.Change.Untracked && !entry.Change.Conflicted
}

func (a *App) discardEntry(index int) {
	if a.syncDisabled() || index < 0 || index >= len(a.Entries) || !canDiscardEntry(a.Entries[index]) {
		return
	}
	a.confirmWrite(domain.GitOperation{
		RepositoryID: a.Repository.Repository.RepositoryID,
		Operation:    domain.Discard,
		Paths:        []string{a.Entries[index].Change.Path},
	}, false)
}

func (a *App) stageEntry(index int, staged bool) {
	if index < 0 || index >= len(a.Entries) {
		return
	}
	op := domain.Unstage
	if staged {
		op = domain.Stage
	}
	selected := a.Selected
	a.Selected = index
	a.RunWrite(op)
	a.Selected = selected
}

func sourceControlIconButton(c *ui.Context, label, tooltip string, icon *ui.SVG) ui.Element {
	button := iconButton(c, label, tooltip, icon).Size(22, 22).Padding(0).Gap(0).
		Radius(3).BorderWidth(0).Background(ui.Transparent).TextColor(c.Theme().TextMuted)
	if button.Hovered() {
		button.Background(c.Theme().SurfaceHover)
	}
	return button
}

func (a *App) changeDisclosure(c *ui.Context, key string) {
	rotation := float32(0)
	if a.changesCollapsed[key] {
		rotation = -90
	}
	ui.Icon(c, chevronDownIcon).Size(12, 12).Shrink(0).
		TextColor(c.Theme().TextMuted).Rotate(rotation)
}
