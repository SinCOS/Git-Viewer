package local

import (
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

// diff.go owns the Inspector: unified diff preview, commit details header,
// copy support and the diff find bar. It is mounted by both the Commit and
// Git Log modes.

// diffFind holds the FindBar state for one App. The App field set is a frozen
// shared contract, so the state lives beside the App, keyed by pointer.
type diffFind struct {
	open    bool
	query   string
	current int
}

var diffFinds sync.Map // *App -> *diffFind

func (a *App) findState() *diffFind {
	if v, ok := diffFinds.Load(a); ok {
		return v.(*diffFind)
	}
	v, _ := diffFinds.LoadOrStore(a, &diffFind{})
	return v.(*diffFind)
}

func (a *App) diffView(c *ui.Context) {
	ui.Column(c).Fill().Padding(8).Gap(6).Children(func() {
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
			a.commitDetailsView(c)
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
		a.diffControlsView(c)
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

// commitDetailsView groups the commit header: author, time, message and the
// first changed files. All data is read-only state already loaded on App.
func (a *App) commitDetailsView(c *ui.Context) {
	ui.Column(c).Gap(2).FillWidth().Children(func() {
		ui.Text(c, "COMMIT DETAILS").Bold()
		ui.Textf(c, "Author: %s", a.CommitDetails.Commit.Author).Selectable()
		ui.Textf(c, "Authored: %s", a.CommitDetails.Commit.AuthoredAt.Format("2006-01-02 15:04:05")).Selectable()
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
	})
}

// diffControlsView mounts copy, the find toggle and the FindBar. The FindBar
// state lives in a.findState(); a.lines is searched case-insensitively and a
// match move scrolls the diff list to that line.
func (a *App) diffControlsView(c *ui.Context) {
	find := a.findState()
	if ui.Button(c, "Copy diff").Clicked() {
		c.WriteClipboard(a.Preview.Text)
	}
	if ui.Button(c, "Find").Clicked() {
		find.open = true
	}
	var matches []int
	if q := strings.ToLower(find.query); q != "" {
		for i, line := range a.lines {
			if strings.Contains(strings.ToLower(line), q) {
				matches = append(matches, i)
			}
		}
	}
	bar := ui.FindBar(c, &find.open, &find.query, len(matches), &find.current)
	if bar.Changed() && len(matches) > 0 {
		a.DiffList.ScrollTo(matches[min(find.current, len(matches)-1)], ui.Center)
	}
}
