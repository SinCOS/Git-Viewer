package local

import (
	"fmt"

	"github.com/egoist/mygo/ui"
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
			ui.Text(c, label).Height(32).Padding(4).NoWrap()
		}).Grow(1).FillWidth()
		if list.Changed() {
			a.Select(a.Selected)
		}
	})
}
