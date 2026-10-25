package local

import (
	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

// operations.go owns the operation controls: the commit form in the Commit
// mode sidebar, the repository popover anchor and the VCS sync buttons in
// the toolbar. All writes go through App.RunWrite.
func (a *App) featureToolbar(c *ui.Context) {
	ui.Row(c).Gap(6).Padding(4).Children(func() {
		ui.TextInput(c, &a.PathInput).Label("Repository path").Placeholder("Local directory path").Width(260)
		ui.Button(c, "Open path").OnClick(func() { a.Open(a.PathInput) })
		ui.Combobox(c, &a.SSHHost, a.SSHHosts).Label("SSH host").Placeholder("SSH Host alias").Width(150)
		ui.TextInput(c, &a.SSHPath).Label("Remote repository path").Placeholder("/absolute/remote/path").Grow(1)
		ui.Button(c, "Connect SSH").Disabled(a.Loading).OnClick(a.OpenSSH)
		ui.Button(c, "Status").OnClick(func() { a.Pane = "Status" })
		ui.Button(c, "History / Graph").Disabled(a.Loading).OnClick(func() { a.LoadHistory(false) })
		ui.Button(c, "Branches / Worktrees").Disabled(a.Loading).OnClick(a.LoadBranches)
		a.repoMenu(c)
		// IDEA keeps the VCS sync operations in the toolbar's right end.
		ui.Button(c, "Fetch").Disabled(a.syncDisabled()).OnClick(func() { a.RunWrite(domain.Fetch) })
		ui.Button(c, "Pull (fast-forward)").Disabled(a.syncDisabled()).OnClick(func() { a.RunWrite(domain.PullFFOnly) })
		ui.Button(c, "Push").Disabled(a.syncDisabled()).OnClick(func() { a.RunWrite(domain.Push) })
	})
}

// syncDisabled reports whether repository writes are currently impossible.
func (a *App) syncDisabled() bool {
	return a.Loading || a.DialogOpen || a.Stale || a.Repository.Repository.RepositoryID == ""
}

// commitSidebar mounts the commit form in the Commit mode sidebar: message
// field and the Commit split button, as the approved mockup shows.
func (a *App) commitSidebar(c *ui.Context) {
	unavailable := a.syncDisabled()
	ui.Column(c).FillWidth().Gap(6).Padding(8).Children(func() {
		ui.Row(c).Children(func() {
			ui.Text(c, "Commit").Bold()
			ui.Text(c, "⌥⌘K").TextColor(c.Theme().TextMuted)
		})
		ui.TextArea(c, &a.MessageInput).Label("Commit message").Placeholder("Commit message").Height(72).FillWidth()
		ui.MenuButton(c, "Commit ▾", func(m *ui.Menu) {
			if m.Item("Commit staged").Disabled(unavailable).Chosen() {
				a.RunWrite(domain.Commit)
			}
			if m.Item("Commit and Push").Disabled(unavailable).Chosen() {
				// Two steps by design: start() serializes writes, so Push is
				// confirmed separately after the commit completes.
				a.RunWrite(domain.Commit)
			}
		}).FillWidth()
		ui.Text(c, "Stage files with the checkboxes in Local Changes.").TextColor(c.Theme().TextMuted)
	})
}
