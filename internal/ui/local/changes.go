package local

import (
	"fmt"
	"path"
	"strings"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

// changes.go owns the Local Changes panel (IDEA Commit tool window body):
// grouped counts, the file list with stage checkboxes, and the VS Code-style
// list/tree view toggle.

// changeRow is one rendered row of the Local Changes view: a directory
// header in tree mode, or a file entry.
type changeRow struct {
	dir     bool
	dirPath string
	entry   int // index into a.Entries, -1 for directory rows
}

func (a *App) statusView(c *ui.Context) {
	unavailable := a.Loading || a.Writing || a.DialogOpen || a.Stale || a.Repository.Repository.RepositoryID == ""
	ui.Column(c).Fill().Padding(6).Gap(6).Children(func() {
		ui.Row(c).Gap(6).Children(func() {
			ui.Text(c, "Local Changes").Bold()
			if n := len(a.Entries); n > 0 {
				ui.Textf(c, "· %d", n).TextColor(c.Theme().TextMuted)
			}
			ui.Text(c, "").Grow(1)
			ui.Button(c, operationLabel(domain.Stage)).Disabled(unavailable || a.Selected < 0).OnClick(func() { a.RunWrite(domain.Stage) })
			ui.Button(c, operationLabel(domain.Unstage)).Disabled(unavailable || a.Selected < 0).OnClick(func() { a.RunWrite(domain.Unstage) })
			ui.Button(c, "List").OnClick(func() { a.ChangesTree = false })
			ui.Button(c, "Tree").OnClick(func() { a.ChangesTree = true })
		})
		if a.Repository.Repository.RepositoryID == "" {
			ui.Text(c, "Open a repository to view changes")
			return
		}
		if len(a.Entries) == 0 {
			ui.Text(c, "Working tree clean")
			return
		}
		for _, group := range []string{"Staged", "Unstaged", "Untracked", "Conflicted"} {
			count := 0
			for _, entry := range a.Entries {
				if entry.Group == group {
					count++
				}
			}
			if count > 0 {
				ui.Textf(c, "%s: %d", group, count)
			}
		}
		rows := a.changeRows()
		a.StatusList.Selected = &a.Selected
		a.StatusList.Key = func(i int) any {
			if rows[i].dir {
				return "dir\x00" + rows[i].dirPath
			}
			e := a.Entries[rows[i].entry]
			return e.Group + "\x00" + e.Change.Path
		}
		a.StatusList.Label = func(i int) string {
			if rows[i].dir {
				return rows[i].dirPath
			}
			e := a.Entries[rows[i].entry]
			return e.Group + " " + pathLabel(e.Change.Path)
		}
		// The list owns pointer and keyboard selection for each file row.
		list := ui.List(c, &a.StatusList, len(rows), func(i int) {
			row := rows[i]
			if row.dir {
				depth := strings.Count(row.dirPath, "/")
				ui.Row(c).Children(func() {
					ui.Text(c, strings.Repeat("    ", depth)+"📁 "+path.Base(row.dirPath)).Bold()
				})
				return
			}
			entry := a.Entries[row.entry]
			label := fmt.Sprintf("%s [%s%s] %s", entry.Group, entry.Change.IndexStatus, entry.Change.WorktreeStatus, pathLabel(entry.Change.Path))
			if entry.Change.OldPath != nil {
				label += " ← " + pathLabel(*entry.Change.OldPath)
			}
			depth := 0
			if a.ChangesTree {
				depth = strings.Count(path.Dir(entry.Change.Path), "/") + 1
			}
			indent := strings.Repeat("    ", depth)
			ui.Row(c).Gap(6).Children(func() {
				// The box mirrors the entry's group: checking stages the row's
				// path, unchecking unstages it, both through RunWrite's confirm
				// flow. The state never follows the click, only Git's answer.
				checked := entry.Group == "Staged"
				ui.Checkbox(c, &checked, "").Label("Stage " + pathLabel(entry.Change.Path)).
					Disabled(a.Loading || a.Writing || a.DialogOpen || a.Stale).
					OnChange(func() { a.stageEntry(row.entry, checked) })
				ui.Text(c, indent+label).Height(32).Padding(4).NoWrap().Grow(1)
			})
		}).Grow(1).FillWidth()
		if list.Changed() {
			if a.ChangesTree {
				// The list selection is a row index; map it back to the entry.
				if a.Selected >= 0 && a.Selected < len(rows) && !rows[a.Selected].dir {
					a.Select(rows[a.Selected].entry)
				}
			} else {
				a.Select(a.Selected)
			}
		}
	})
}

// changeRows builds the rendered row model. List mode is one row per entry;
// tree mode groups entries by their directory, as VS Code's Source Control
// tree does. Entries with no directory share the root row.
func (a *App) changeRows() []changeRow {
	if !a.ChangesTree {
		rows := make([]changeRow, len(a.Entries))
		for i := range a.Entries {
			rows[i] = changeRow{entry: i}
		}
		return rows
	}
	var rows []changeRow
	last := ""
	for i := range a.Entries {
		dir := path.Dir(a.Entries[i].Change.Path)
		if dir == "." {
			dir = "/"
		}
		if dir != last {
			rows = append(rows, changeRow{dir: true, dirPath: dir, entry: -1})
			last = dir
		}
		rows = append(rows, changeRow{entry: i})
	}
	return rows
}

// stageEntry routes a checkbox change through RunWrite, which stages or
// unstages the entry a.Selected points at: aim the selection at the row for
// the request, then restore it so row clicks keep owning the diff selection.
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
