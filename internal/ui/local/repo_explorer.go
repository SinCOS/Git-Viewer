package local

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

// repo_explorer.go owns the VS Code-style worktree/branch sections shown in
// the Worktree mode sidebar.

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
