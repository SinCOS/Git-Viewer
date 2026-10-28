package local

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

func workspaceLabel(path string) string {
	if path == "" {
		return "None"
	}
	return filepath.Base(path)
}

func (a *App) currentBranch() string {
	for _, b := range a.Branches {
		if b.Current {
			return strings.TrimPrefix(b.Name, "refs/heads/")
		}
	}
	return "Branch not loaded"
}

func (a *App) logBranches(c *ui.Context) {
	ui.Column(c).FillWidth().Padding(12).Gap(6).Children(func() {
		ui.Text(c, "Branches").Bold().FontSize(12)
		if len(a.Branches) == 0 {
			ui.Text(c, "Not loaded")
			ui.Button(c, "Load branches").Disabled(a.Loading || a.Repository.Repository.RepositoryID == "").OnClick(a.loadBranches)
		}
		for _, remote := range []bool{false, true} {
			label := "Local"
			if remote {
				label = "Remote"
			}
			ui.Text(c, label).Bold().FontSize(10).TextColor(c.Theme().TextMuted).Padding(8, 0, 4, 0)
			for _, b := range a.Branches {
				if strings.HasPrefix(b.Name, "refs/remotes/") != remote {
					continue
				}
				name := strings.TrimPrefix(strings.TrimPrefix(b.Name, "refs/heads/"), "refs/remotes/")
				if b.Current {
					name = "✓ " + name
				}
				button := ui.Button(c, name).FontSize(12).FillWidth().Background(ui.Transparent).Disabled(remote || a.syncDisabled()).OnClick(func() {
					a.BranchInput = strings.TrimPrefix(b.Name, "refs/heads/")
					a.RunWrite(domain.SwitchBranch)
				})
				if b.Current {
					button.Background(c.Theme().Surface).TextColor(c.Theme().Accent)
				}
			}
		}
	})
}

func (a *App) historyInspector(c *ui.Context) {
	ui.Row(c).Label("History inspector").Fill().Gap(0).Children(func() {
		ui.Column(c).Key("history-details").Width(300).MinWidth(240).Shrink(0).FillHeight().Children(func() {
			a.historyDetailsSummary(c)
		})
		ui.Box(c).Label("History inspector divider").Width(1).FillHeight().Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
			p.Fill(r, c.Theme().Border, 0)
		})
		ui.Column(c).Key("history-diff").Grow(1).MinWidth(0).FillHeight().Children(func() {
			a.historyDiffView(c)
		})
	})
}

func (a *App) historyDetailsSummary(c *ui.Context) {
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().Padding(14).Gap(8).Children(func() {
			ui.Text(c, "COMMIT DETAILS").Bold().FontSize(11).TextColor(c.Theme().TextMuted)
			if a.CommitDetails.Commit.OID == "" {
				ui.Text(c, "Changed Files").Bold().FontSize(12)
				ui.Text(c, "Select a commit to view details").FontSize(12).TextColor(c.Theme().TextMuted)
				return
			}
			commit := a.CommitDetails.Commit
			ui.Text(c, commit.Subject).FontSize(14).Bold().SingleLine().Tooltip(commit.Subject)
			ui.Textf(c, "%s · %s", commit.Author, commit.AuthoredAt.Format("02 Jan 2006 15:04:05")).FontSize(11).
				TextColor(c.Theme().TextMuted).SingleLine().Tooltip(commit.Author)
			ui.Textf(c, "%.12s", commit.OID).Font("monospace").FontSize(10).
				TextColor(c.Theme().Accent).Tooltip(commit.OID).Selectable()
			if message := strings.TrimSpace(a.CommitDetails.Message); message != "" {
				ui.Scroll(c).MaxHeight(70).FillWidth().Children(func() { ui.Text(c, message).FontSize(12).Selectable() })
			}
			ui.Row(c).Gap(8).Padding(8, 0, 4, 0).Children(func() {
				changedFiles := "files"
				if len(a.CommitDetails.Files) == 1 {
					changedFiles = "file"
				}
				ui.Textf(c, "%d changed %s", len(a.CommitDetails.Files), changedFiles).Bold().FontSize(12).Grow(1)
				ui.Textf(c, "+%d", diffFileAdditions(a.Preview.Text)).FontSize(10).TextColor(c.Theme().Success)
				ui.Textf(c, "−%d", diffFileRemovals(a.Preview.Text)).FontSize(10).TextColor(c.Theme().Danger)
			})
			ui.Column(c).FillWidth().Gap(2).Children(func() {
				for _, file := range a.CommitDetails.Files {
					row := ui.Row(c).Label("Commit file "+pathLabel(file.Path)).Height(30).Padding(0, 6).Gap(8).
						Cursor(ui.CursorPointer).OnClick(func() { a.loadDiff(domain.DiffRequest{Scope: domain.CommitDiff, Commit: commit.OID, Path: file.Path}) })
					if a.Preview.Scope == domain.CommitDiff && a.Preview.Path == file.Path {
						row.Background(c.Theme().SurfacePressed)
					}
					row.Children(func() {
						ui.Text(c, file.Status).Font("monospace").FontSize(11).TextColor(commitStatusColor(c, file.Status)).Shrink(0)
						ui.Text(c, displayPath(file.Path)).FontSize(12).SingleLine().MinWidth(0).Grow(1).Tooltip(file.Path)
					})
				}
			})
		})
	})
}

func diffFileAdditions(text string) int {
	n := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			n++
		}
	}
	return n
}

func diffFileRemovals(text string) int {
	n := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			n++
		}
	}
	return n
}
