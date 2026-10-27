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
	staged := 0
	for _, entry := range a.Entries {
		if entry.Scope == domain.StagedDiff {
			staged++
		}
	}
	unavailable := a.syncDisabled() || staged == 0 || strings.TrimSpace(a.MessageInput) == ""
	ui.Column(c).Label("Commit form").FillWidth().Gap(8).Padding(8).Children(func() {
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
		if a.Repository.Repository.RepositoryID != "" {
			if staged == 0 {
				ui.Text(c, "Stage at least one file to commit").FontSize(11).TextColor(c.Theme().TextMuted)
			} else if strings.TrimSpace(a.MessageInput) == "" {
				ui.Text(c, "Enter a commit message").FontSize(11).TextColor(c.Theme().TextMuted)
			}
		}
		ui.Text(c, "Push requires a separate confirmation after commit.").FontSize(10).TextColor(c.Theme().TextMuted)
	})
}
