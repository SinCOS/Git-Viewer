package local

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

// operations.go owns the Commit workspace form and its write invocations.
// All writes use the existing confirmation and serialization flow.

// CommitAndPush confirms each write separately and only offers Push after a
// successful commit and status verification.
func (a *App) CommitAndPush() { a.runWrite(domain.Commit, true) }

// syncDisabled reports whether repository writes are currently impossible.
func (a *App) syncDisabled() bool {
	return a.Writing || a.DialogOpen || a.Stale || a.Repository.Repository.RepositoryID == ""
}

// commitSidebar mounts the form below the changes/diff split.
func (a *App) commitSidebar(c *ui.Context) {
	staged, unstaged := 0, 0
	for _, entry := range a.Entries {
		if entry.Scope == domain.StagedDiff {
			staged++
		} else {
			unstaged++
		}
	}
	unavailable := a.syncDisabled() || staged == 0 || strings.TrimSpace(a.MessageInput) == ""
	ui.Column(c).Label("Commit form").FillWidth().Gap(8).Padding(12).Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, "Commit Message").Bold().FontSize(13).Grow(1)
			ui.Text(c, fmt.Sprintf("%d staged", staged)).FontSize(11).TextColor(c.Theme().TextMuted)
		})
		ui.TextArea(c, &a.MessageInput).Label("Commit message").Placeholder("Describe your changes").Height(96).FillWidth()
		switch {
		case a.Repository.Repository.RepositoryID == "":
			ui.Text(c, "Open a repository to commit changes").FontSize(11).TextColor(c.Theme().TextMuted)
		case a.Stale:
			ui.Text(c, "Repository status is out of date. Refresh changes before committing.").FontSize(11).TextColor(c.Theme().Danger)
		case staged == 0:
			ui.Text(c, "Stage at least one file to commit").FontSize(11).TextColor(c.Theme().TextMuted)
		case strings.TrimSpace(a.MessageInput) == "":
			ui.Text(c, "Enter a commit message").FontSize(11).TextColor(c.Theme().TextMuted)
		}
		ui.Button(c, fmt.Sprintf("Commit %d file(s)", staged)).Label("Commit").FillWidth().Background(c.Theme().Accent).TextColor(c.Theme().AccentText).Disabled(unavailable).OnClick(func() { a.RunWrite(domain.Commit) })
		ui.Row(c).Gap(8).Children(func() {
			ui.Button(c, "Stage all").Disabled(a.syncDisabled() || unstaged == 0).OnClick(func() { a.runBatchChanges(domain.Stage) })
			ui.Button(c, "Unstage all").Disabled(a.syncDisabled() || staged == 0).OnClick(func() { a.runBatchChanges(domain.Unstage) })
		})
		ui.MenuButton(c, "Commit and Push ▾", func(m *ui.Menu) {
			if m.Item("Commit and Push").Disabled(unavailable).Chosen() {
				a.CommitAndPush()
			}
		}).Disabled(unavailable)
		ui.Text(c, "Push requires a separate confirmation after commit.").FontSize(10).TextColor(c.Theme().TextMuted)
	})
}
