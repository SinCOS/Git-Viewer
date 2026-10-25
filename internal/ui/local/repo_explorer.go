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
		// IDEA-like: repository and workspace entry points collapse into a
		// popover anchored to one button instead of a long column of forms.
		ui.Popover(c, ui.Button(c, "Repository ▾").Width(220), &a.RepoMenuOpen, func() {
			ui.Column(c).Width(260).Padding(8).Gap(8).Children(func() {
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
			})
		})
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
		// The commit group lives in the explorer, as IDEA parks the commit
		// form inside its tool window beside the change list.
		ui.Divider(c)
		ui.Column(c).FillWidth().Children(func() { a.writeControls(c) }).Grow(1)
		ui.Text(c, "Cmd+R refresh")
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
