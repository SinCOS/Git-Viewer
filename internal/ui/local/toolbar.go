package local

import (
	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

// toolbar.go owns the unified top toolbar: workspace and repository context,
// branch switcher and the global git actions. Mode navigation lives in rail.go.
// All writes use the existing confirmation and serialization flow.

func (a *App) toolbarView(c *ui.Context) {
	ui.Row(c).Label("Main toolbar").Gap(6).Padding(4, 8).Height(46).FillWidth().AlignItems(ui.Center).Children(func() {
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
