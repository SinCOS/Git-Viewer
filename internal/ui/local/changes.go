package local

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

// changes.go owns the Changes mode panel: grouped counts and the file list.
// Row actions (stage checkboxes) also live here.
func (a *App) statusView(c *ui.Context) {
	ui.Column(c).Fill().Padding(6).Children(func() {
		ui.Text(c, "Status").Bold()
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
			ui.Textf(c, "%s: %d", group, count)
		}
		a.StatusList.Selected = &a.Selected
		a.StatusList.Key = func(i int) any { e := a.Entries[i]; return e.Group + "\x00" + e.Change.Path }
		a.StatusList.Label = func(i int) string { e := a.Entries[i]; return e.Group + " " + pathLabel(e.Change.Path) }
		// The list owns pointer and keyboard selection for each file row.
		list := ui.List(c, &a.StatusList, len(a.Entries), func(i int) {
			entry := a.Entries[i]
			label := fmt.Sprintf("%s [%s%s] %s", entry.Group, entry.Change.IndexStatus, entry.Change.WorktreeStatus, pathLabel(entry.Change.Path))
			if entry.Change.OldPath != nil {
				label += " ← " + pathLabel(*entry.Change.OldPath)
			}
			ui.Row(c).Gap(6).Children(func() {
				// The box mirrors the entry's group: checking stages the row's
				// path, unchecking unstages it, both through RunWrite's confirm
				// flow. The state never follows the click, only Git's answer.
				checked := entry.Group == "Staged"
				ui.Checkbox(c, &checked, "").Label("Stage " + pathLabel(entry.Change.Path)).
					Disabled(a.Loading || a.Writing || a.DialogOpen || a.Stale).
					OnChange(func() { a.stageEntry(i, checked) })
				ui.Text(c, label).Height(32).Padding(4).NoWrap().Grow(1)
			})
		}).Grow(1).FillWidth()
		if list.Changed() {
			a.Select(a.Selected)
		}
	})
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
