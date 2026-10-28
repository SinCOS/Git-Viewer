package local

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

// operations.go owns the Commit workspace form and its write invocations.
// Writes use the existing serialization flow; only risky operations confirm.

// CommitAndPush only starts Push after a successful commit or amend and status
// verification.
func (a *App) CommitAndPush() { a.runWrite(a.commitOperation(), true) }

func (a *App) commitOperation() domain.Operation {
	if a.Amend {
		return domain.Amend
	}
	return domain.Commit
}

// syncDisabled reports whether repository writes are currently impossible.
func (a *App) syncDisabled() bool {
	return a.Writing || a.DialogOpen || a.Stale || a.Repository.Repository.RepositoryID == ""
}

// discardAllPaths rejects the entire request if any visible worktree change
// cannot be passed safely to the existing discard writer.
func (a *App) discardAllPaths() ([]string, string) {
	var paths []string
	for _, change := range a.Status.Changes {
		if change.Untracked || change.Conflicted || change.IndexStatus == "?" || change.WorktreeStatus == "?" || change.IndexStatus == "!" || change.WorktreeStatus == "!" {
			return nil, "Discard All unavailable: untracked, ignored, or conflicted changes require separate handling."
		}
		if change.WorktreeStatus == " " {
			continue
		}
		if change.Path == "" || change.OldPath != nil ||
			(change.IndexStatus != " " && change.IndexStatus != "M" && change.IndexStatus != "A") ||
			(change.WorktreeStatus != "M" && change.WorktreeStatus != "D") {
			return nil, "Discard All unavailable: a worktree change has a rename, type change, or other unsupported state."
		}
		paths = append(paths, change.Path)
	}
	return paths, ""
}

func (a *App) discardAll() {
	if a.syncDisabled() {
		return
	}
	paths, reason := a.discardAllPaths()
	if reason != "" {
		a.commitFormError = reason
		return
	}
	if len(paths) == 0 {
		return
	}
	a.confirmWrite(domain.GitOperation{
		RepositoryID: a.Repository.Repository.RepositoryID,
		Operation:    domain.Discard,
		Paths:        paths,
	}, false)
}

// commitSidebar mounts the Source Control commit form.
func (a *App) commitSidebar(c *ui.Context) {
	staged := 0
	for _, entry := range a.Entries {
		if entry.Scope == domain.StagedDiff {
			staged++
		}
	}
	discardPaths, discardReason := a.discardAllPaths()
	ui.Column(c).Label("Commit form").FillWidth().Gap(6).Padding(8).Children(func() {
		ui.TextArea(c, &a.MessageInput).Label("Commit message").Placeholder("Message (⌘Enter to commit)").
			Height(64).FillWidth().Background(c.Theme().Background).Border(1, c.Theme().Border).Radius(2)
		if a.commitFormError != "" {
			ui.Text(c, a.commitFormError).Label("Commit form error").FontSize(11).TextColor(c.Theme().Danger)
		}
		switch {
		case a.Repository.Repository.RepositoryID == "":
			ui.Text(c, "Open a repository to commit changes").FontSize(11).TextColor(c.Theme().TextMuted)
		case a.Stale:
			ui.Text(c, "Repository status is out of date. Refresh changes before committing.").FontSize(11).TextColor(c.Theme().Danger)
		case staged == 0 && !a.Amend:
			ui.Text(c, "Stage at least one file to commit").FontSize(11).TextColor(c.Theme().TextMuted)
		case strings.TrimSpace(a.MessageInput) == "":
			ui.Text(c, "Enter a commit message").FontSize(11).TextColor(c.Theme().TextMuted)
		}
		ui.Checkbox(c, &a.Amend, "Amend last commit").Disabled(a.Writing || a.DialogOpen)
		unavailable := a.syncDisabled() || (!a.Amend && staged == 0) || strings.TrimSpace(a.MessageInput) == ""
		buttonLabel := "Commit"
		if staged > 0 {
			buttonLabel = fmt.Sprintf("Commit (%d)", staged)
		}
		if a.Amend {
			buttonLabel = "Amend"
			if staged > 0 {
				buttonLabel = fmt.Sprintf("Amend (%d)", staged)
			}
		}
		ui.Row(c).FillWidth().Gap(4).Children(func() {
			ui.Button(c, buttonLabel).Label("Commit").Grow(1).Height(26).Padding(0, 8).Radius(2).
				Background(c.Theme().Accent).TextColor(c.Theme().AccentText).Disabled(unavailable).
				OnClick(func() { a.RunWrite(a.commitOperation()) })
			ui.MenuButton(c, "▾", func(m *ui.Menu) {
				if m.Item("Commit and Push").Disabled(unavailable).Chosen() {
					a.CommitAndPush()
				}
				m.Separator()
				if m.Item("Reset form").Disabled(a.Writing || a.DialogOpen || (a.MessageInput == "" && !a.Amend)).Chosen() {
					a.MessageInput = ""
					a.Amend = false
					a.commitFormError = ""
				}
				if m.Item("Discard all changes").Disabled(a.syncDisabled() || discardReason != "" || len(discardPaths) == 0).Chosen() {
					a.discardAll()
				}
			}).Height(26).Width(24).Padding(0).Gap(0).Radius(2)
		})
	})
}
