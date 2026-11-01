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

// stagedCount is the number of staged entries shown in the commit form.
func (a *App) stagedCount() int {
	staged := 0
	for _, entry := range a.Entries {
		if entry.Scope == domain.StagedDiff {
			staged++
		}
	}
	return staged
}

// commitUnavailable mirrors the Commit button's enabled state so the source
// control header and the form cannot disagree.
func (a *App) commitUnavailable() bool {
	return a.syncDisabled() || (!a.Amend && a.stagedCount() == 0) || strings.TrimSpace(a.MessageInput) == ""
}

// selectedChangePath is the file the Source Control panels currently target.
func (a *App) selectedChangePath() string {
	if a.Selected >= 0 && a.Selected < len(a.Entries) {
		return a.Entries[a.Selected].Change.Path
	}
	return a.Preview.Path
}

// commitSidebar mounts the Source Control commit form.
func (a *App) commitSidebar(c *ui.Context) {
	staged := a.stagedCount()
	discardPaths, discardReason := a.discardAllPaths()
	ui.Column(c).Label("Commit form").FillWidth().Gap(6).Padding(8, 12).Children(func() {
		input := ui.TextArea(c, &a.MessageInput).Label("Commit message").Placeholder("Message (⌘Enter to commit)").
			MinHeight(52).Height(52).FillWidth().FontSize(13).Disabled(a.Writing || a.DialogOpen).Background(c.Theme().Background).
			Border(1, c.Theme().Border.Mix(c.Theme().Text, 0.12)).Radius(4)
		if input.Shortcut(ui.Cmd, ui.KeyEnter) && !a.commitUnavailable() {
			a.RunWrite(a.commitOperation())
		}
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
		unavailable := a.commitUnavailable()
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
		ui.Row(c).FillWidth().Gap(6).Children(func() {
			ui.ButtonBase(c).Label("Commit").Grow(1).Height(28).Padding(0, 10).Gap(6).Radius(3).
				Background(c.Theme().Accent).TextColor(c.Theme().AccentText).Disabled(unavailable).
				OnClick(func() { a.RunWrite(a.commitOperation()) }).Children(func() {
				ui.Icon(c, commitIcon).Size(14, 14).Shrink(0)
				ui.Text(c, buttonLabel).FontSize(12).NoWrap()
			})
			actions := ui.ButtonBase(c).Label("Commit actions").Tooltip("Commit actions").
				Height(28).Width(30).Padding(0).Gap(0).Radius(3).
				Background(c.Theme().Surface).Border(1, c.Theme().Border).
				Cursor(ui.CursorPointer)
			actions.Menu(func(m *ui.Menu) {
				if m.Item("Commit (Amend)").Disabled(a.syncDisabled()).Chosen() {
					a.Amend = true
				}
				if m.Item("Commit and Push").Disabled(unavailable).Chosen() {
					a.CommitAndPush()
				}
				m.Separator()
				if m.Item("Undo Last Commit").Disabled(a.syncDisabled()).Chosen() {
					a.RunWrite(domain.UndoCommit)
				}
				if m.Item("Reset form").Disabled(a.Writing || a.DialogOpen || (a.MessageInput == "" && !a.Amend)).Chosen() {
					a.MessageInput = ""
					a.Amend = false
					a.commitFormError = ""
				}
				if m.Item("Discard all changes").Disabled(a.syncDisabled() || discardReason != "" || len(discardPaths) == 0).Chosen() {
					a.discardAll()
				}
			})
			if actions.Hovered() {
				actions.Background(c.Theme().SurfaceHover)
			}
			actions.Children(func() {
				ui.Icon(c, chevronDownIcon).Size(14, 14).Shrink(0).TextColor(c.Theme().TextMuted)
			})
		})
	})
}
