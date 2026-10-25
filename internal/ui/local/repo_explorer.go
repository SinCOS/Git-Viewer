package local

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

// repo_explorer.go owns the Repository Explorer: changes entry counts live in
// changes.go; this file mounts recent repositories, workspace controls and the
// worktree/branch summaries.
// currentRepositoryPath is the open repository path used for selection
// markers; empty when no repository is open so nothing is marked.
func (a *App) currentRepositoryPath() string {
	if a.Repository.Repository.RepositoryID == "" {
		return ""
	}
	return a.Repository.Repository.Path
}

// markCurrent wraps current in a row carrying the selection marker.
func markCurrent(c *ui.Context, current bool, mount func()) {
	if !current {
		mount()
		return
	}
	ui.Row(c).Gap(4).Children(func() {
		mount()
		ui.Text(c, "●").TextColor(c.Theme().Accent)
	})
}

func (a *App) sidebar(c *ui.Context) {
	ui.Column(c).Fill().Padding(8).Gap(8).Children(func() {
		ui.Text(c, "Changes").Bold()
		if a.Repository.Repository.RepositoryID == "" {
			ui.Text(c, "Open a repository to view changes")
		} else if len(a.Entries) == 0 {
			ui.Text(c, "Working tree clean")
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
		// The changes list owns pointer and keyboard selection for each file row.
		a.StatusList.Selected = &a.Selected
		a.StatusList.Key = func(i int) any { e := a.Entries[i]; return e.Group + "\x00" + e.Change.Path }
		a.StatusList.Label = func(i int) string { e := a.Entries[i]; return e.Group + " " + pathLabel(e.Change.Path) }
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
		ui.Scroll(c).MaxHeight(280).FillWidth().Children(func() {
			ui.Column(c).FillWidth().Padding(8).Gap(8).Children(func() {
				ui.Text(c, "Recent local repositories").Bold()
				if len(a.Recent) == 0 {
					ui.Text(c, "No recent repositories")
				}
				current := a.currentRepositoryPath()
				for i, repo := range a.Recent {
					marked := current != "" && repo.Path == current
					markCurrent(c, marked, func() {
						ui.Button(c.Key(i), pathLabel(repo.Path)).OnClick(func() { a.Open(repo.Path) })
					})
				}
				if a.Repository.Repository.RepositoryID != "" {
					ui.Text(c, pathLabel(a.Repository.Repository.Path)).Selectable()
				}
				a.workspaceSidebar(c)
				ui.Text(c, "Worktrees").Bold()
				if len(a.Worktrees) == 0 {
					ui.Text(c, "Not loaded")
				}
				for _, w := range a.Worktrees {
					ui.Text(c, pathLabel(w.Path)+" · "+w.Branch).Selectable()
				}
				ui.Text(c, "Branches").Bold()
				if len(a.Branches) == 0 {
					ui.Text(c, "Not loaded")
				}
				if len(a.Worktrees) == 0 || len(a.Branches) == 0 {
					ui.Button(c, "Load branches").Disabled(a.Loading).OnClick(a.LoadBranches)
				}
				for _, b := range a.Branches {
					label := b.Name
					if b.Current {
						label = "HEAD · " + label
					}
					ui.Text(c, label).Selectable()
				}
				ui.Text(c, "Cmd+R refresh")
			})
		})
	})
}

func (a *App) branchesView(c *ui.Context) {
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			ui.Text(c, "Branches").Bold()
			for _, b := range a.Branches {
				label := b.Name
				if b.Current {
					label = "HEAD · " + label
				}
				ui.Button(c, label+" → "+b.Upstream).OnClick(func() { a.BranchInput = strings.TrimPrefix(b.Name, "refs/heads/") })
			}
			ui.Text(c, "Worktrees").Bold()
			for _, w := range a.Worktrees {
				ui.Textf(c, "%s · %s · detached=%t · locked=%t %s · prunable=%t %s", pathLabel(w.Path), w.Branch, w.Detached, w.Locked, w.LockReason, w.Prunable, w.PruneReason).Selectable()
			}
		})
	})
}
func (a *App) workspaceSidebar(c *ui.Context) {
	ui.Text(c, "Workspace").Bold()
	if a.Workspace.FilePath != "" {
		ui.Button(c, "Close workspace").Disabled(a.Loading).OnClick(func() {
			a.Workspace = domain.WorkspaceView{}
			a.WorkspaceFolders = nil
			a.WorkspaceRepositories = nil
			a.WorkspaceInput = ""
		})
	}
	for _, loc := range a.Locations {
		label := loc.Path
		if loc.Kind == "ssh" {
			label = loc.Host + ":" + label
		}
		ui.Button(c, "Recent "+pathLabel(label)).OnClick(func() {
			if loc.Kind == "ssh" {
				a.SSHHost = loc.Host
				a.SSHPath = loc.Path
				a.OpenSSH()
			} else {
				a.OpenWorkspace(loc.Path)
			}
		})
	}
	ui.TextInput(c, &a.WorkspaceInput).Label("Workspace path").Placeholder(".code-workspace")
	ui.Row(c).Gap(4).Children(func() {
		ui.Button(c, "Open workspace").Disabled(a.Loading || a.DialogOpen).OnClick(a.ChooseWorkspaceFile)
		ui.Button(c, "Save").Disabled(a.Loading || a.Workspace.FilePath == "").OnClick(func() { a.SaveWorkspace(false) })
	})
	ui.Button(c, "New / Save workspace as…").Disabled(a.Loading).OnClick(func() { a.SaveWorkspace(true) })
	ui.TextInput(c, &a.FolderInput).Label("Workspace folder").Placeholder("Folder path")
	ui.Button(c, "Add folder").OnClick(func() {
		if a.FolderInput != "" {
			a.WorkspaceFolders = append(a.WorkspaceFolders, domain.WorkspaceFolder{Path: a.FolderInput})
			a.FolderInput = ""
		}
	})
	for i, f := range a.WorkspaceFolders {
		ui.Row(c).Children(func() {
			ui.Text(c, pathLabel(f.Path)).Grow(1)
			ui.Button(c, fmt.Sprintf("Remove %d", i+1)).OnClick(func() { a.WorkspaceFolders = append(a.WorkspaceFolders[:i], a.WorkspaceFolders[i+1:]...) })
		})
	}
	for _, repo := range a.WorkspaceRepositories {
		current := a.currentRepositoryPath()
		marked := current != "" && repo.Repository.Path == current
		markCurrent(c, marked, func() {
			ui.Button(c, "Open "+pathLabel(repo.Repository.Path)).OnClick(func() { a.Open(repo.Repository.Path) })
		})
	}
	for _, f := range a.Workspace.Folders {
		for _, diagnostic := range f.Diagnostics {
			ui.Text(c, pathLabel(f.Path)+": "+diagnostic)
		}
	}
}
