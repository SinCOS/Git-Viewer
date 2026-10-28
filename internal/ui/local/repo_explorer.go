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
	ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
		mount()
		ui.Text(c, "●").TextColor(c.Theme().Accent)
	})
}

// repoMenu mounts the repository/workspace popover anchored in the toolbar.
// It carries the recent repositories, the current path and the workspace
// controls, so the mode sidebars stay focused on their mode.
func (a *App) repoMenu(c *ui.Context) {
	name := a.Repository.Repository.DisplayName
	ui.Popover(c, ui.Button(c, "Repository: "+compactLabel(name, 24)+" ▾").Height(34).Tooltip(name).OnClick(func() { a.RepoMenuOpen = !a.RepoMenuOpen }), &a.RepoMenuOpen, func() {
		ui.Scroll(c).Width(260).MaxHeight(380).Children(func() {
			ui.Column(c).FillWidth().Padding(8).Gap(8).Children(func() {
				ui.Button(c, "Choose repository…").OnClick(a.Choose)
				ui.Button(c, "Add current repository to workspace").Disabled(a.Repository.Repository.RepositoryID == "").
					OnClick(func() { a.AddToWorkspace(a.Repository.Repository.Path) })
				if len(a.WorkspaceRepositories) > 0 {
					ui.Text(c, "Workspace folders").Bold()
					current := a.currentRepositoryPath()
					for i, repo := range a.WorkspaceRepositories {
						marked := current != "" && repo.Repository.Path == current
						ui.Row(c.Key(fmt.Sprintf("workspace-repo-%d", i))).FillWidth().Gap(6).AlignItems(ui.Center).Children(func() {
							markCurrent(c, marked, func() {
								ui.Button(c, "Open "+pathLabel(repo.Repository.Path)).Tooltip(repo.Repository.Path).Grow(1).MinWidth(0).OnClick(func() {
									a.RepoMenuOpen = false
									a.Open(repo.Repository.Path)
								})
							})
							if n, ok := a.workspaceChanges[repo.Repository.RepositoryID]; ok && n >= 0 {
								ui.Badge(c, fmt.Sprintf("%d", n)).Label("Workspace changes " + repo.Repository.RepositoryID)
							}
						})
					}
					iconButton(c, "Refresh workspace statuses", "Refresh workspace statuses", refreshIcon).
						Disabled(a.Writing || a.DialogOpen).OnClick(func() { a.loadWorkspaceStatuses(false) })
				}
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
				ui.TextInput(c, &a.PathInput).Label("Repository path").FillWidth()
				ui.Button(c, "Open path").OnClick(func() { a.Open(a.PathInput) })
			})
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

// branchesView is the Worktree mode's entry: side bar list plus editor details.
func (a *App) branchesView(c *ui.Context) {
	a.worktreeSidebar(c)
	a.worktreeEditor(c)
}

// worktreeSidebar mounts the Worktree mode's primary side bar content: the
// creation controls and the worktree list.
func (a *App) worktreeSidebar(c *ui.Context) {
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().Gap(8).Padding(8).Children(func() {
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, "Worktrees").Bold().FontSize(12).Grow(1)
				ui.Button(c, "Load branches").Disabled(a.Loading || a.Repository.Repository.RepositoryID == "").OnClick(a.LoadBranches)
			})
			ui.Row(c).Gap(8).Children(func() {
				ui.TextInput(c, &a.WorktreePathInput).Label("New worktree path").Placeholder("New absolute path").Grow(1).MinWidth(80)
				ui.TextInput(c, &a.WorktreeBranchInput).Label("New worktree branch").Placeholder("Existing local branch").Width(180)
				ui.Button(c, "Create worktree").Disabled(a.syncDisabled() || a.Loading).OnClick(func() { a.worktreeAction(true) })
			})
			ui.Column(c).FillWidth().Gap(8).Children(func() {
				ui.Text(c, "Worktree List").Bold().FontSize(12)
				if len(a.Worktrees) == 0 {
					ui.Text(c, "Not loaded")
				}
				for i, w := range a.Worktrees {
					label := strings.TrimPrefix(w.Branch, "refs/heads/")
					if label == "" {
						label = "Detached HEAD"
					}
					if i == a.SelectedWorktree {
						label = "● " + label
					}
					ui.Button(c.Key(i), label).OnClick(func() { a.SelectedWorktree = i; a.WorktreeStatusText = "" })
					ui.Text(c, pathLabel(w.Path)).Selectable()
				}
			})
		})
	})
}

// worktreeEditor mounts the Worktree mode's editor area: the details of the
// selected worktree.
func (a *App) worktreeEditor(c *ui.Context) {
	ui.Column(c).Fill().Children(func() { a.worktreeDetails(c) })
}

func (a *App) workspaceSidebar(c *ui.Context) {
	ui.Text(c, "Workspace").Bold()
	if a.Workspace.FilePath != "" {
		ui.Button(c, "Close workspace").Disabled(a.Loading).OnClick(a.CloseWorkspace)
	}
	for _, loc := range a.Locations {
		label := loc.Path
		if loc.Kind == "ssh" {
			label = loc.Host + ":" + label
			ui.Button(c, "Recent "+pathLabel(label)).OnClick(func() {
				a.SSHHost = loc.Host
				a.SSHPath = loc.Path
				a.OpenSSH()
			})
			continue
		}
		ui.Row(c).Gap(4).Children(func() {
			ui.Button(c, "Recent "+pathLabel(label)).Grow(1).OnClick(func() { a.OpenWorkspace(loc.Path) })
			iconButton(c, "Add "+pathLabel(label), "Add to workspace", plusIcon).OnClick(func() { a.AddToWorkspace(loc.Path) })
		})
	}
	ui.Button(c, "Add current repository").Disabled(a.Repository.Repository.RepositoryID == "").
		OnClick(func() { a.AddToWorkspace(a.Repository.Repository.Path) })
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
