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
			ui.Button(c, "Load branches").Disabled(a.Loading || a.Repository.Repository.RepositoryID == "").OnClick(func() { a.loadBranches(false) })
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
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().Padding(14).Gap(10).Children(func() {
			ui.Text(c, "COMMIT DETAILS").Bold().FontSize(11).TextColor(c.Theme().TextMuted)
			if a.CommitDetails.Commit.OID == "" {
				ui.Text(c, "Changed Files").Bold().FontSize(12)
				ui.Text(c, "Select a commit to view details").FontSize(12).TextColor(c.Theme().TextMuted)
				return
			}
			commit := a.CommitDetails.Commit
			ui.Text(c, commit.Subject).FontSize(14).Bold()
			ui.Textf(c, "%s · %s", commit.Author, commit.AuthoredAt.Format("2006-01-02 15:04:05")).FontSize(11).TextColor(c.Theme().TextMuted).Selectable()
			ui.Textf(c, "Hash: %.12s", commit.OID).Font("monospace").FontSize(10).TextColor(c.Theme().Accent).Tooltip(commit.OID).Selectable()
			ui.Scroll(c).MaxHeight(90).FillWidth().Children(func() { ui.Text(c, a.CommitDetails.Message).FontSize(12).Selectable() })
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, "Changed Files").Bold().FontSize(12).Grow(1)
				ui.Textf(c, "Changed files: %d", len(a.CommitDetails.Files)).FontSize(10).TextColor(c.Theme().TextMuted)
			})
			ui.Scroll(c).MaxHeight(160).FillWidth().Children(func() {
				ui.Column(c).FillWidth().Gap(3).Children(func() {
					for _, file := range a.CommitDetails.Files {
						row := ui.Row(c).Label("Commit file " + pathLabel(file.Path)).Padding(4).Gap(8).Cursor(ui.CursorPointer).
							OnClick(func() { a.loadDiff(domain.DiffRequest{Scope: domain.CommitDiff, Commit: commit.OID, Path: file.Path}) })
						if a.Preview.Scope == domain.CommitDiff && a.Preview.Path == file.Path {
							row.Background(c.Theme().SurfacePressed)
						}
						row.Children(func() {
							ui.Text(c, file.Status).Font("monospace").FontSize(11).TextColor(c.Theme().Accent)
							ui.Text(c, displayPath(file.Path)).FontSize(12)
						})
					}
				})
			})
			a.diffView(c)
		})
	})
}

func (a *App) worktreeDetails(c *ui.Context) {
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().Padding(8).Gap(8).Children(func() {
			ui.Text(c, "Worktree Details").Bold()
			if a.SelectedWorktree < 0 || a.SelectedWorktree >= len(a.Worktrees) {
				ui.Text(c, "Select a worktree")
				return
			}
			w := a.Worktrees[a.SelectedWorktree]
			ui.Textf(c, "Branch: %s", strings.TrimPrefix(w.Branch, "refs/heads/")).Selectable()
			ui.Textf(c, "Path: %s", pathLabel(w.Path)).Selectable()
			ui.Textf(c, "HEAD: %s", w.Head).Selectable()
			ui.Textf(c, "detached=%t · locked=%t %s · prunable=%t %s", w.Detached, w.Locked, w.LockReason, w.Prunable, w.PruneReason)
			ui.Text(c, "Status: open this worktree to load changes")
			ui.Button(c, "Open").Disabled(a.Loading || w.Bare || w.Prunable).OnClick(func() {
				if a.Repository.SSHHost != "" {
					a.SSHHost = a.Repository.SSHHost
					a.SSHPath = w.Path
					a.OpenSSH()
				} else {
					a.Open(w.Path)
				}
			})
			ui.Button(c, "Remove").Disabled(true)
			ui.Text(c, "Adding and removing worktrees is not available yet.").TextColor(c.Theme().TextMuted)
		})
	})
}
