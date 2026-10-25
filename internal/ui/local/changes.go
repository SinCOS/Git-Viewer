package local

import (
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
			if count > 0 {
				ui.Textf(c, "%s: %d", group, count)
			}
		}
		ui.Text(c, "File rows are listed in the explorer; select one to preview its diff")
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
