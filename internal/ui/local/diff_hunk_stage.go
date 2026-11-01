package local

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"gitviewer/internal/domain"
)

// canStageHunk reports whether the current full-file diff can be written to
// the index by hunk. Commit history and partial or binary previews stay
// read-only. Local and SSH repositories share the same bounded patch writer.
func (a *App) canStageHunk() bool {
	return a.Preview.Path != "" && a.Preview.FullText && !a.Preview.Limited && !a.Preview.Binary &&
		(a.Preview.Scope == domain.WorktreeDiff || a.Preview.Scope == domain.StagedDiff)
}

// stageDiffHunk builds the single-hunk patch for the change the user chose and
// sends it through the same validated writer as the file-level Stage/Unstage
// actions. The patch is derived from the already displayed Git diff, so the
// index update cannot widen beyond the selected hunk.
func (a *App) stageDiffHunk(hunk editor.DiffHunk) {
	if a.syncDisabled() || !a.canStageHunk() {
		return
	}
	patch, err := a.hunkPatch(hunk)
	if err != nil {
		a.Message = err.Error()
		return
	}
	op := domain.StageHunk
	if a.Preview.Scope == domain.StagedDiff {
		op = domain.UnstageHunk
	}
	a.confirmWrite(domain.GitOperation{
		RepositoryID: a.Repository.Repository.RepositoryID,
		Operation:    op,
		Paths:        []string{a.Preview.Path},
		Patch:        patch,
	}, false)
}

// canDiscardHunk reports whether the displayed worktree change can be
// reverted by hunk. Staged and historical diffs have no worktree side to
// restore, so they stay unchanged.
func (a *App) canDiscardHunk() bool {
	return a.canStageHunk() && a.Preview.Scope == domain.WorktreeDiff
}

// discardDiffHunk reverts one changed run in the worktree, leaving the index
// and every other change untouched.
func (a *App) discardDiffHunk(hunk editor.DiffHunk) {
	if a.syncDisabled() || !a.canDiscardHunk() {
		return
	}
	patch, err := a.hunkPatch(hunk)
	if err != nil {
		a.Message = err.Error()
		return
	}
	a.confirmWrite(domain.GitOperation{
		RepositoryID: a.Repository.Repository.RepositoryID,
		Operation:    domain.DiscardHunk,
		Paths:        []string{a.Preview.Path},
		Patch:        patch,
	}, false)
}

// hunkPatch formats one changed run from the full texts the native viewer
// compares, with the unchanged lines around it as context. Context stops at
// the next change, so staging one run cannot carry an adjacent one with it.
func (a *App) hunkPatch(hunk editor.DiffHunk) (string, error) {
	path := a.Preview.Path
	if path == "" || strings.ContainsAny(path, "\x00\r\n\t\"") {
		return "", fmt.Errorf("This file path cannot be staged by hunk")
	}
	oldLines, oldEOL := splitDiffText(a.Preview.OldText)
	newLines, newEOL := splitDiffText(a.Preview.NewText)
	if hunk.OldStart < 0 || hunk.OldEnd < hunk.OldStart || hunk.OldEnd > len(oldLines) ||
		hunk.NewStart < 0 || hunk.NewEnd < hunk.NewStart || hunk.NewEnd > len(newLines) {
		return "", fmt.Errorf("This change is outside the displayed file")
	}
	if hunk.OldStart == hunk.OldEnd && hunk.NewStart == hunk.NewEnd {
		return "", fmt.Errorf("This change has no lines to stage")
	}
	before, after := 0, 0
	for before < 3 && hunk.OldStart-before-1 >= 0 && hunk.NewStart-before-1 >= 0 &&
		oldLines[hunk.OldStart-before-1] == newLines[hunk.NewStart-before-1] {
		before++
	}
	for after < 3 && hunk.OldEnd+after < len(oldLines) && hunk.NewEnd+after < len(newLines) &&
		oldLines[hunk.OldEnd+after] == newLines[hunk.NewEnd+after] {
		after++
	}
	oldStart := hunk.OldStart - before + 1
	oldCount := before + hunk.OldEnd - hunk.OldStart + after
	if oldCount == 0 {
		oldStart = hunk.OldStart
	}
	newStart := hunk.NewStart - before + 1
	newCount := before + hunk.NewEnd - hunk.NewStart + after
	if newCount == 0 {
		newStart = hunk.NewStart
	}
	var b strings.Builder
	fmt.Fprintf(&b, "diff --git a/%s b/%s\n", path, path)
	fmt.Fprintf(&b, "--- a/%s\n+++ b/%s\n", path, path)
	fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", oldStart, oldCount, newStart, newCount)
	writeLine := func(prefix byte, text string, eof, eol bool) {
		b.WriteByte(prefix)
		b.WriteString(text)
		b.WriteByte('\n')
		if eof && !eol {
			b.WriteString("\\ No newline at end of file\n")
		}
	}
	for i := hunk.OldStart - before; i < hunk.OldStart; i++ {
		writeLine(' ', oldLines[i], i == len(oldLines)-1, oldEOL)
	}
	for i := hunk.OldStart; i < hunk.OldEnd; i++ {
		writeLine('-', oldLines[i], i == len(oldLines)-1, oldEOL)
	}
	for i := hunk.NewStart; i < hunk.NewEnd; i++ {
		writeLine('+', newLines[i], i == len(newLines)-1, newEOL)
	}
	for i := hunk.OldEnd; i < hunk.OldEnd+after; i++ {
		writeLine(' ', oldLines[i], i == len(oldLines)-1, oldEOL)
	}
	return b.String(), nil
}

// splitDiffText returns the lines of a full text and whether it ends with a
// newline. Empty texts have no lines and count as newline-terminated.
func splitDiffText(text string) ([]string, bool) {
	if text == "" {
		return nil, true
	}
	lines := strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		return lines[:len(lines)-1], true
	}
	return lines, false
}

// diffHunkActions renders the Stage/Unstage button on the change under the
// pointer, next to the line it changes, as VS Code does.
func (a *App) diffHunkActions(c *ui.Context, bar ui.Element, actions *editor.DiffHunkActions) {
	if !a.canStageHunk() {
		return
	}
	label, tooltip := "Stage Hunk", "Stage this change"
	if a.Preview.Scope == domain.StagedDiff {
		label, tooltip = "Unstage Hunk", "Unstage this change"
	}
	bar.Children(func() {
		hunkActionButton(c, label, tooltip, func() { a.stageDiffHunk(actions.Hunk) })
		if a.canDiscardHunk() {
			hunkActionButton(c, "Discard Hunk", "Discard this change", func() { a.discardDiffHunk(actions.Hunk) })
		}
	})
}

func hunkActionButton(c *ui.Context, label, tooltip string, action func()) {
	btn := ui.ButtonBase(c).Label(label).Tooltip(tooltip).Height(22).Padding(0, 8).Radius(4).
		Background(c.Theme().Surface).Border(1, c.Theme().Border).Cursor(ui.CursorPointer)
	btn.Children(func() {
		ui.Text(c, label).FontSize(11).TextColor(c.Theme().Text)
	})
	if btn.Clicked() {
		action()
	}
}

// configureHunkStageActions enables the native viewer's change actions only
// for full local worktree/staged diffs that can be written to the index.
func (a *App) configureHunkStageActions(state *editor.DiffState) {
	if !a.canStageHunk() {
		state.HunkActions = false
		state.RenderHunkActions = nil
		state.OnGutterButton = nil
		state.RenderGutterButton = nil
		return
	}
	state.HunkActions = true
	state.RenderHunkActions = a.diffHunkActions
	state.OnGutterButton = func(line editor.DiffLineRange) {
		if hunk, ok := diffHunkAtLine(state, line); ok {
			a.stageDiffHunk(hunk)
		}
	}
	state.RenderGutterButton = func(c *ui.Context, btn ui.Element, line editor.DiffLineRange) {
		if _, ok := diffHunkAtLine(state, line); !ok {
			return
		}
		if a.Preview.Scope != domain.StagedDiff {
			editor.DefaultGutterButton(c, btn, line)
			return
		}
		// The same gutter action removes the change from the index when the
		// preview shows the staged side.
		t := c.Theme()
		btn.Radius(4).Background(t.Accent).Label("Unstage line")
		btn.Draw(func(p *ui.Painter, r ui.Rect) {
			cx, cy := r.X+r.W/2, r.Y+r.H/2
			p.Line(cx-4, cy, cx+4, cy, 1.5, t.AccentText)
		})
	}
}

// diffHunkAtLine returns the changed run whose side contains the gutter line.
// Deleted lines use the old side; added and unchanged lines use the new side.
func diffHunkAtLine(state *editor.DiffState, line editor.DiffLineRange) (editor.DiffHunk, bool) {
	for _, hunk := range state.Hunks() {
		if line.Side == editor.DiffOld {
			if line.Start >= hunk.OldStart && line.Start < hunk.OldEnd {
				return hunk, true
			}
			continue
		}
		if line.Start >= hunk.NewStart && line.Start < hunk.NewEnd {
			return hunk, true
		}
	}
	return editor.DiffHunk{}, false
}

// diffHunkStageControl adds the same action to the diff title bar so the
// change at the caret can be staged without hovering over it.
func (a *App) diffHunkStageControl(c *ui.Context, state *editor.DiffState) {
	if !a.canStageHunk() {
		return
	}
	hunks := state.Hunks()
	if len(hunks) == 0 {
		return
	}
	label, tooltip, icon := "Stage Hunk", "Stage the change at the caret", stageIcon
	if a.Preview.Scope == domain.StagedDiff {
		label, tooltip, icon = "Unstage Hunk", "Unstage the change at the caret", unstageIcon
	}
	stage := iconButton(c, label, tooltip, icon).Size(26, 26).Radius(3)
	stage.Disabled(a.syncDisabled())
	if stage.Hovered() && !a.syncDisabled() {
		stage.Background(c.Theme().SurfaceHover)
	}
	if stage.Clicked() {
		current, _ := state.CurrentHunk()
		if current < 0 {
			current = 0
		}
		current = min(current, len(hunks)-1)
		a.stageDiffHunk(hunks[current])
	}
	if !a.canDiscardHunk() {
		return
	}
	discard := iconButton(c, "Discard Hunk", "Discard the change at the caret", changeTrashIcon).Size(26, 26).Radius(3)
	discard.Disabled(a.syncDisabled())
	if discard.Hovered() && !a.syncDisabled() {
		discard.Background(c.Theme().SurfaceHover)
	}
	if discard.Clicked() {
		current, _ := state.CurrentHunk()
		if current < 0 {
			current = 0
		}
		current = min(current, len(hunks)-1)
		a.discardDiffHunk(hunks[current])
	}
}
