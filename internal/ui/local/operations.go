package local

import (
	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

// operations.go owns the Operation Panel: connection controls, Git write
// operations and the commit form. All writes go through App.RunWrite.
func (a *App) featureToolbar(c *ui.Context) {
	ui.Row(c).Gap(6).Padding(4).Children(func() {
		ui.TextInput(c, &a.PathInput).Label("Repository path").Placeholder("Local directory path").Width(260)
		ui.Button(c, "Open path").OnClick(func() { a.Open(a.PathInput) })
		ui.Text(c, "Connect").TextColor(c.Theme().TextMuted)
		ui.Combobox(c, &a.SSHHost, a.SSHHosts).Label("SSH host").Placeholder("SSH Host alias").Width(150)
		ui.TextInput(c, &a.SSHPath).Label("Remote repository path").Placeholder("/absolute/remote/path").Grow(1)
		ui.Button(c, "Connect SSH").Disabled(a.Loading).OnClick(a.OpenSSH)
		ui.Button(c, "Status").OnClick(func() { a.Pane = "Status" })
		ui.Button(c, "History / Graph").Disabled(a.Loading).OnClick(func() { a.LoadHistory(false) })
		ui.Button(c, "Branches / Worktrees").Disabled(a.Loading).OnClick(a.LoadBranches)
	})
}
func (a *App) writeControls(c *ui.Context) {
	unavailable := a.Loading || a.DialogOpen || a.Stale || a.Repository.Repository.RepositoryID == ""
	ui.Row(c).Gap(4).Children(func() {
		for _, op := range []domain.Operation{domain.Stage, domain.Unstage, domain.Fetch, domain.PullFFOnly, domain.Push} {
			ui.Button(c, operationLabel(op)).Disabled(unavailable).OnClick(func() { a.RunWrite(op) })
		}
	})
	ui.Row(c).Gap(4).Children(func() {
		ui.TextArea(c, &a.MessageInput).Label("Commit message").Placeholder("Commit message").Height(64).Grow(1)
		ui.Button(c, "Commit staged").Disabled(unavailable).OnClick(func() { a.RunWrite(domain.Commit) })
		// start() drops any new work while a write is in flight, so a combined
		// commit+push cannot run as one nested action; this button starts the
		// commit step and Push stays a separate confirmed write afterwards.
		ui.Button(c, "Commit and Push").Disabled(unavailable).OnClick(func() { a.RunWrite(domain.Commit) })
		ui.Text(c, "Commit and Push runs as two steps · confirm Push after the commit completes").TextColor(c.Theme().TextMuted)
	})
	ui.Row(c).Gap(4).Children(func() {
		ui.TextInput(c, &a.BranchInput).Label("Local branch").Placeholder("Local branch name").Grow(1)
		ui.Button(c, "Switch branch").Disabled(unavailable).OnClick(func() { a.RunWrite(domain.SwitchBranch) })
	})
}
