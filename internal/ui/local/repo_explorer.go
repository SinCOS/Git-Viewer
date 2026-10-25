package local

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

// repo_explorer.go owns the repository context: the repository/workspace
// popover in the toolbar, and the VS Code-style worktree/branch sections
// shown in the Worktree mode sidebar.

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

// repoMenu mounts the repository/workspace popover anchored in the toolbar.
// It carries the recent repositories, the current path and the workspace
// controls, so the mode sidebars stay focused on their mode.
func (a *App) repoMenu(c *ui.Context) {
	ui.Popover(c, ui.Button(c, "Repository ▾"), &a.RepoMenuOpen, func() {
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
}

// vscHead renders a VS Code-style collapsible section header: uppercase
// label, fold arrow and entry count.
func (a *App) vscHead(c *ui.Context, label string, count int, open *bool, body func()) {
	mark := "▸"
	if *open {
		mark = "▾"
	}
	ui.Row(c).Gap(6).OnClick(func() { *open = !*open }).Children(func() {
		ui.Text(c, mark).TextColor(c.Theme().TextMuted)
		ui.Text(c, strings.ToUpper(label)).Bold()
		if count > 0 {
			ui.Textf(c, "(%d)", count).TextColor(c.Theme().TextMuted)
		}
	})
	if *open {
		body()
	}
}

// worktreeSections mounts the worktree/branch sections for the Worktree mode
// sidebar. Choosing a branch goes through the safe switch flow.
func (a *App) worktreeSections(c *ui.Context) {
	a.vscHead(c, "worktrees", len(a.Worktrees), &a.WtOpen, func() {
		if len(a.Worktrees) == 0 {
			ui.Text(c, "Not loaded").TextColor(c.Theme().TextMuted)
		}
		for _, w := range a.Worktrees {
			ui.Text(c, pathLabel(w.Path)+" · "+w.Branch).Selectable()
		}
	})
	a.vscHead(c, "branches", len(a.Branches), &a.BrOpen, func() {
		if len(a.Branches) == 0 {
			ui.Text(c, "Not loaded").TextColor(c.Theme().TextMuted)
		}
		for _, b := range a.Branches {
			label := strings.TrimPrefix(b.Name, "refs/heads/")
			if b.Current {
				label = "✓ " + label
			}
			ui.Text(c, label).OnClick(func() {
				a.BranchInput = strings.TrimPrefix(b.Name, "refs/heads/")
				a.RunWrite(domain.SwitchBranch)
			})
		}
	})
	if len(a.Worktrees) == 0 || len(a.Branches) == 0 {
		ui.Button(c, "Load branches").Disabled(a.Loading).OnClick(a.LoadBranches)
	}
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
