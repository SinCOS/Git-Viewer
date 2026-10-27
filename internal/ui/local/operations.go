package local

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
	"strings"
)

// operations.go owns the unified top toolbar: mode switch, workspace and
// repository context, panel toggles and the global git actions. All writes
// use the existing confirmation and serialization flow.

// modeSwitch renders the primary navigation as a segmented group in the
// toolbar: Commit first, Git Log second, Worktree third.
func (a *App) modeSwitch(c *ui.Context) {
	modes := []struct {
		id, label string
		icon      *ui.SVG
	}{
		{"commit", "Commit", commitIcon},
		{"log", "Git Log", logIcon},
		{"worktree", "Worktree", wtIcon},
	}
	for _, m := range modes {
		active := a.Mode == m.id
		button := ui.ButtonBase(c).Label(m.label).Tooltip(m.label).
			Height(28).Padding(4, 10).Gap(6).Shrink(0).
			Background(ui.Transparent).Cursor(ui.CursorPointer).
			OnClick(func() { a.setMode(m.id) })
		if active {
			button.Background(c.Theme().SurfacePressed)
		}
		button.Children(func() {
			ui.Icon(c, m.icon).Size(16, 16).Shrink(0).TextColor(c.Theme().TextMuted)
			ui.Text(c, m.label).FontSize(12)
			if m.id == "commit" {
				if n := len(a.Entries); n > 0 {
					ui.Badge(c, fmt.Sprintf("%d", n))
				}
			}
		})
	}
}

func (a *App) featureToolbar(c *ui.Context) {
	ui.Row(c).Label("Main toolbar").Gap(6).Padding(4).Children(func() {
		a.modeSwitch(c)
		ui.Box(c).Width(1).Height(20).Shrink(0).Background(c.Theme().Border)
		ui.Popover(c, ui.Button(c, "Workspace: "+workspaceLabel(a.Workspace.FilePath)+" ▾"), &a.WorkspaceMenuOpen, func() {
			ui.Column(c).Width(300).Padding(8).Gap(6).Children(func() { a.workspaceSidebar(c) })
		})
		a.repoMenu(c)
		ui.Popover(c, ui.Button(c, a.currentBranch()+" ▾"), &a.BranchPopOpen, func() {
			ui.Column(c).Width(260).Padding(8).Children(func() { a.logBranches(c) })
		})
		ui.Text(c, "").Grow(1)
		iconButton(c, "Fetch", "Fetch", fetchIcon).Disabled(a.syncDisabled()).OnClick(func() { a.RunWrite(domain.Fetch) })
		iconButton(c, "Pull (fast-forward)", "Pull (fast-forward only)", pullIcon).Disabled(a.syncDisabled()).OnClick(func() { a.RunWrite(domain.PullFFOnly) })
		iconButton(c, "Push", "Push", pushIcon).Disabled(a.syncDisabled()).OnClick(func() { a.RunWrite(domain.Push) })
		ui.Popover(c, ui.Button(c, "Connection / Git ▾"), &a.ConnectionMenuOpen, func() {
			ui.Column(c).Width(380).Padding(8).Gap(6).Children(func() {
				ui.TextInput(c, &a.PathInput).Label("Repository path").Placeholder("Local directory path").Width(260)
				ui.Button(c, "Open path").OnClick(func() { a.Open(a.PathInput) })
				ui.Combobox(c, &a.SSHHost, a.SSHHosts).Label("SSH host").Placeholder("SSH Host alias").Width(150)
				ui.TextInput(c, &a.SSHPath).Label("Remote repository path").Placeholder("/absolute/remote/path").Grow(1)
				ui.Button(c, "Connect SSH").Disabled(a.Loading).OnClick(a.OpenSSH)
				ui.Button(c, "Status").OnClick(func() { a.Pane = "Status" })
				ui.Button(c, "History / Graph").Disabled(a.Loading).OnClick(func() { a.LoadHistory(false) })
				ui.Button(c, "Branches / Worktrees").Disabled(a.Loading).OnClick(a.LoadBranches)

			})
		})
	})
}

// CommitAndPush confirms each write separately and only offers Push after a
// successful commit and status verification.
func (a *App) CommitAndPush() { a.runWrite(domain.Commit, true) }

// syncDisabled reports whether repository writes are currently impossible.
func (a *App) syncDisabled() bool {
	return a.Writing || a.DialogOpen || a.Stale || a.Repository.Repository.RepositoryID == ""
}

// commitSidebar mounts the form below the changes/diff split.
func (a *App) commitSidebar(c *ui.Context) {
	staged := 0
	for _, entry := range a.Entries {
		if entry.Scope == domain.StagedDiff {
			staged++
		}
	}
	unavailable := a.syncDisabled() || staged == 0 || strings.TrimSpace(a.MessageInput) == ""
	ui.Column(c).FillWidth().Gap(6).Padding(8).Children(func() {
		ui.Text(c, "Commit Message").Bold().FontSize(12)
		ui.TextArea(c, &a.MessageInput).Label("Commit message").Placeholder("Commit message").Height(72).FillWidth()
		ui.Row(c).Gap(6).Children(func() {
			ui.Text(c, fmt.Sprintf("%d files staged", staged)).FontSize(11).TextColor(c.Theme().TextMuted).Grow(1)
			ui.Button(c, "Commit").Disabled(unavailable).OnClick(func() { a.RunWrite(domain.Commit) })
			ui.MenuButton(c, "Commit and Push ▾", func(m *ui.Menu) {
				if m.Item("Commit and Push").Disabled(unavailable).Chosen() {
					a.CommitAndPush()
				}
			}).Disabled(unavailable)
		})
		ui.Text(c, "Push requires a separate confirmation after commit.").FontSize(10).TextColor(c.Theme().TextMuted)
	})
}
