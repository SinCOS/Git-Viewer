package local

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// diff.go owns the Inspector: unified diff preview, commit details header and
// copy support. It is mounted by both the Commit and Git Log modes.
func (a *App) diffView(c *ui.Context) {
	ui.Column(c).Fill().Padding(8).Gap(6).Children(func() {
		a.writeControls(c)
		ui.Text(c, "Unified Diff").Bold()
		ui.Row(c).Gap(6).Children(func() {
			ui.TextInput(c, &a.CommitInput).Label("Commit object ID").Placeholder("Full commit object ID").Grow(1)
			ui.Button(c, "View commit diff").Disabled(a.Repository.Repository.RepositoryID == "").OnClick(a.CommitDiff)
		})
		if a.Preview.Scope == "" {
			ui.Text(c, "Select a changed file or enter a commit object ID")
			return
		}
		if a.CommitDetails.Commit.OID != "" {
			ui.Textf(c, "%s · %s", a.CommitDetails.Commit.Author, a.CommitDetails.Commit.AuthoredAt.Format("2006-01-02 15:04:05")).Selectable()
			ui.Scroll(c).MaxHeight(100).FillWidth().Children(func() {
				ui.Text(c, a.CommitDetails.Message).Selectable()
			})
			ui.Textf(c, "Changed files: %d", len(a.CommitDetails.Files))
			for i, file := range a.CommitDetails.Files {
				if i == 5 {
					ui.Text(c, "More files are shown in the diff below")
					break
				}
				ui.Text(c, file.Status+" "+pathLabel(file.Path)).Selectable()
			}
		}
		ui.Textf(c, "%s · %s", a.Preview.Scope, pathLabel(a.Preview.Path)).Selectable()
		if a.Preview.Limited {
			ui.Text(c, "Large diff · preview limit exceeded (512 KiB / 10,000 lines)")
			return
		}
		if a.Preview.Binary {
			ui.Text(c, "Binary file · textual preview unavailable")
		}
		if a.Preview.Text == "" {
			ui.Text(c, "No textual changes")
			return
		}
		if ui.Button(c, "Copy diff").Clicked() {
			c.WriteClipboard(a.Preview.Text)
		}
		a.DiffBuiltRows = 0
		ui.ScrollHorizontal(c).TrackScroll(&a.DiffScroll).Grow(1).FillWidth().Children(func() {
			ui.List(c, &a.DiffList, len(a.lines), func(i int) {
				a.DiffBuiltRows++
				line := a.lines[i]
				text := ui.Text(c, line).Font("monospace").NoWrap().Selectable().Height(20)
				if strings.HasPrefix(line, "+") {
					text.TextColor(c.Theme().Accent)
				} else if strings.HasPrefix(line, "-") {
					text.TextColor(c.Theme().Danger)
				} else if strings.HasPrefix(line, "@@") {
					text.TextColor(c.Theme().TextMuted)
				}
			}).Width(a.diffWidth).FillHeight()
		})
	})
}
