package local

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"gitviewer/internal/application"
	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/storage"
	"gitviewer/internal/testgit"
)

func newDiffStageTestApp(t *testing.T) (*App, chan func()) {
	t.Helper()
	a := New(application.LocalService{Recent: storage.RecentRepositories{Path: filepath.Join(t.TempDir(), "recent.json")}})
	queue := make(chan func(), 32)
	a.Dispatch = func(apply func()) { queue <- apply }
	a.Confirm = func(string, string) bool { return true }
	t.Cleanup(func() { a.Close(); a.Wait() })
	return a, queue
}

func settleDiffStage(t *testing.T, a *App, queue chan func()) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for a.Loading || a.DialogOpen {
		select {
		case apply := <-queue:
			apply()
		case <-deadline:
			t.Fatal("background operation did not finish")
		}
	}
}

func numberedLines(changed map[int]string) string {
	var b strings.Builder
	for i := 1; i <= 20; i++ {
		if text, ok := changed[i]; ok {
			fmt.Fprintf(&b, "%s\n", text)
			continue
		}
		fmt.Fprintf(&b, "line %02d\n", i)
	}
	return b.String()
}

func selectDiffEntry(t *testing.T, a *App, queue chan func(), scope domain.DiffScope, path string) {
	t.Helper()
	for i, entry := range a.Entries {
		if entry.Scope == scope && entry.Change.Path == path {
			a.Select(i)
			settleDiffStage(t, a, queue)
			return
		}
	}
	t.Fatalf("missing %s entry for %s: %+v", scope, path, a.Entries)
}

func TestStageDiffHunkStagesOnlySelectedHunk(t *testing.T) {
	dir := testgit.Repo(t)
	path := filepath.Join(dir, "main.go")
	testgit.Write(t, path, numberedLines(nil))
	testgit.Run(t, dir, "add", "main.go")
	testgit.Run(t, dir, "commit", "-m", "base")
	testgit.Write(t, path, numberedLines(map[int]string{2: "line 02 changed", 18: "line 18 changed"}))

	a, queue := newDiffStageTestApp(t)
	a.PathInput = dir
	a.Open(dir)
	settleDiffStage(t, a, queue)
	selectDiffEntry(t, a, queue, domain.WorktreeDiff, "main.go")

	state := a.nativeDiffState()
	if state == nil {
		t.Fatal("diff did not use the native viewer")
	}
	hunks := state.Hunks()
	if len(hunks) != 2 {
		t.Fatalf("hunks = %d, want 2", len(hunks))
	}
	a.stageDiffHunk(hunks[0])
	settleDiffStage(t, a, queue)
	cached := string(testgit.Run(t, dir, "diff", "--cached", "--", "main.go"))
	worktree := string(testgit.Run(t, dir, "diff", "--", "main.go"))
	if !strings.Contains(cached, "+line 02 changed") || strings.Contains(cached, "+line 18 changed") {
		t.Fatalf("cached diff staged the wrong hunk:\n%s", cached)
	}
	if strings.Contains(worktree, "+line 02 changed") || !strings.Contains(worktree, "+line 18 changed") {
		t.Fatalf("worktree diff lost the wrong hunk:\n%s", worktree)
	}
}

func TestUnstageDiffHunkUnstagesOnlySelectedHunk(t *testing.T) {
	dir := testgit.Repo(t)
	path := filepath.Join(dir, "main.go")
	testgit.Write(t, path, numberedLines(nil))
	testgit.Run(t, dir, "add", "main.go")
	testgit.Run(t, dir, "commit", "-m", "base")
	testgit.Write(t, path, numberedLines(map[int]string{2: "line 02 changed", 18: "line 18 changed"}))
	testgit.Run(t, dir, "add", "main.go")

	a, queue := newDiffStageTestApp(t)
	a.PathInput = dir
	a.Open(dir)
	settleDiffStage(t, a, queue)
	selectDiffEntry(t, a, queue, domain.StagedDiff, "main.go")

	state := a.nativeDiffState()
	if state == nil {
		t.Fatal("diff did not use the native viewer")
	}
	hunks := state.Hunks()
	if len(hunks) != 2 {
		t.Fatalf("hunks = %d, want 2", len(hunks))
	}
	a.stageDiffHunk(hunks[0])
	settleDiffStage(t, a, queue)
	cached := string(testgit.Run(t, dir, "diff", "--cached", "--", "main.go"))
	worktree := string(testgit.Run(t, dir, "diff", "--", "main.go"))
	if strings.Contains(cached, "+line 02 changed") || !strings.Contains(cached, "+line 18 changed") {
		t.Fatalf("cached diff unstaged the wrong hunk:\n%s", cached)
	}
	if !strings.Contains(worktree, "+line 02 changed") || strings.Contains(worktree, "+line 18 changed") {
		t.Fatalf("worktree diff did not restore the unstaged hunk:\n%s", worktree)
	}
}

func TestStageHunkToolbarStagesCurrentChange(t *testing.T) {
	dir := testgit.Repo(t)
	path := filepath.Join(dir, "main.go")
	testgit.Write(t, path, numberedLines(nil))
	testgit.Run(t, dir, "add", "main.go")
	testgit.Run(t, dir, "commit", "-m", "base")
	testgit.Write(t, path, numberedLines(map[int]string{2: "line 02 changed", 18: "line 18 changed"}))

	a, queue := newDiffStageTestApp(t)
	a.PathInput = dir
	a.Open(dir)
	settleDiffStage(t, a, queue)
	selectDiffEntry(t, a, queue, domain.WorktreeDiff, "main.go")
	a.Mode = "commit"
	a.Pane = "Status"
	tt := ui.NewTester(a.View, 1400, 850)
	tt.Frame()
	if err := tt.Click("Stage Hunk"); err != nil {
		t.Fatal(err)
	}
	settleDiffStage(t, a, queue)

	cached := string(testgit.Run(t, dir, "diff", "--cached", "--", "main.go"))
	if !strings.Contains(cached, "+line 02 changed") || strings.Contains(cached, "+line 18 changed") {
		t.Fatalf("toolbar staged the wrong hunk:\n%s", cached)
	}
}

func TestStageDiffHunkWithoutTrailingNewline(t *testing.T) {
	dir := testgit.Repo(t)
	path := filepath.Join(dir, "main.go")
	testgit.Write(t, path, "one\ntwo\nthree")
	testgit.Run(t, dir, "add", "main.go")
	testgit.Run(t, dir, "commit", "-m", "base")
	testgit.Write(t, path, "one\ntwo changed\nthree")

	a, queue := newDiffStageTestApp(t)
	a.PathInput = dir
	a.Open(dir)
	settleDiffStage(t, a, queue)
	selectDiffEntry(t, a, queue, domain.WorktreeDiff, "main.go")

	state := a.nativeDiffState()
	if state == nil {
		t.Fatal("diff did not use the native viewer")
	}
	hunks := state.Hunks()
	if len(hunks) != 1 {
		t.Fatalf("hunks = %d, want 1", len(hunks))
	}
	a.stageDiffHunk(hunks[0])
	settleDiffStage(t, a, queue)

	cached := string(testgit.Run(t, dir, "diff", "--cached", "--", "main.go"))
	if !strings.Contains(cached, "+two changed") || !strings.Contains(cached, "\\ No newline at end of file") {
		t.Fatalf("cached diff lost the no-newline change:\n%s", cached)
	}
}

func TestStageDiffHunkWithSpacedPath(t *testing.T) {
	dir := testgit.Repo(t)
	path := filepath.Join(dir, "main file.go")
	testgit.Write(t, path, "one\ntwo\nthree\n")
	testgit.Run(t, dir, "add", "main file.go")
	testgit.Run(t, dir, "commit", "-m", "base")
	testgit.Write(t, path, "one\ntwo changed\nthree\n")

	a, queue := newDiffStageTestApp(t)
	a.PathInput = dir
	a.Open(dir)
	settleDiffStage(t, a, queue)
	selectDiffEntry(t, a, queue, domain.WorktreeDiff, "main file.go")

	state := a.nativeDiffState()
	if state == nil {
		t.Fatal("diff did not use the native viewer")
	}
	a.stageDiffHunk(state.Hunks()[0])
	settleDiffStage(t, a, queue)

	cached := string(testgit.Run(t, dir, "diff", "--cached", "--", "main file.go"))
	if !strings.Contains(cached, "+two changed") {
		t.Fatalf("spaced path was not staged:\n%s", cached)
	}
}

func TestDiffGutterButtonStagesHunkAtLine(t *testing.T) {
	dir := testgit.Repo(t)
	path := filepath.Join(dir, "main.go")
	testgit.Write(t, path, numberedLines(nil))
	testgit.Run(t, dir, "add", "main.go")
	testgit.Run(t, dir, "commit", "-m", "base")
	testgit.Write(t, path, numberedLines(map[int]string{2: "line 02 changed", 18: "line 18 changed"}))

	a, queue := newDiffStageTestApp(t)
	a.PathInput = dir
	a.Open(dir)
	settleDiffStage(t, a, queue)
	selectDiffEntry(t, a, queue, domain.WorktreeDiff, "main.go")

	state := a.nativeDiffState()
	if state == nil {
		t.Fatal("diff did not use the native viewer")
	}
	a.configureHunkStageActions(state)
	if state.OnGutterButton == nil || state.RenderGutterButton == nil {
		t.Fatal("gutter stage button was not configured")
	}
	hunks := state.Hunks()
	if _, ok := diffHunkAtLine(state, editor.DiffLineRange{Side: editor.DiffNew, Start: hunks[0].NewStart, End: hunks[0].NewStart}); !ok {
		t.Fatal("changed line was not mapped to a hunk")
	}
	if _, ok := diffHunkAtLine(state, editor.DiffLineRange{Side: editor.DiffNew, Start: 10, End: 10}); ok {
		t.Fatal("unchanged line was mapped to a hunk")
	}
	state.OnGutterButton(editor.DiffLineRange{Side: editor.DiffNew, Start: hunks[0].NewStart, End: hunks[0].NewStart})
	settleDiffStage(t, a, queue)

	cached := string(testgit.Run(t, dir, "diff", "--cached", "--", "main.go"))
	if !strings.Contains(cached, "+line 02 changed") || strings.Contains(cached, "+line 18 changed") {
		t.Fatalf("gutter button staged the wrong hunk:\n%s", cached)
	}
}

func TestDiscardDiffHunkDiscardsOnlySelectedHunk(t *testing.T) {
	dir := testgit.Repo(t)
	path := filepath.Join(dir, "main.go")
	testgit.Write(t, path, numberedLines(nil))
	testgit.Run(t, dir, "add", "main.go")
	testgit.Run(t, dir, "commit", "-m", "base")
	testgit.Write(t, path, numberedLines(map[int]string{2: "line 02 changed", 18: "line 18 changed"}))

	a, queue := newDiffStageTestApp(t)
	a.PathInput = dir
	a.Open(dir)
	settleDiffStage(t, a, queue)
	selectDiffEntry(t, a, queue, domain.WorktreeDiff, "main.go")

	state := a.nativeDiffState()
	if state == nil {
		t.Fatal("diff did not use the native viewer")
	}
	hunks := state.Hunks()
	if len(hunks) != 2 {
		t.Fatalf("hunks = %d, want 2", len(hunks))
	}
	a.discardDiffHunk(hunks[0])
	settleDiffStage(t, a, queue)

	worktree := string(testgit.Run(t, dir, "diff", "--", "main.go"))
	cached := string(testgit.Run(t, dir, "diff", "--cached", "--", "main.go"))
	if strings.Contains(worktree, "+line 02 changed") || !strings.Contains(worktree, "+line 18 changed") {
		t.Fatalf("worktree diff discarded the wrong hunk:\n%s", worktree)
	}
	if cached != "" {
		t.Fatalf("discard hunk staged content:\n%s", cached)
	}
}

func TestUndoLastCommitKeepsChangesStaged(t *testing.T) {
	dir := testgit.Repo(t)
	path := filepath.Join(dir, "main.go")
	testgit.Write(t, path, "one\n")
	testgit.Run(t, dir, "add", "main.go")
	testgit.Run(t, dir, "commit", "-m", "base")
	testgit.Write(t, path, "one\ntwo\n")
	testgit.Run(t, dir, "add", "main.go")
	testgit.Run(t, dir, "commit", "-m", "second")

	a, queue := newDiffStageTestApp(t)
	a.PathInput = dir
	a.Open(dir)
	settleDiffStage(t, a, queue)
	a.RunWrite(domain.UndoCommit)
	settleDiffStage(t, a, queue)

	log := string(testgit.Run(t, dir, "log", "--oneline"))
	if strings.Count(strings.TrimSpace(log), "\n") != 0 || !strings.Contains(log, "base") {
		t.Fatalf("undo did not remove the last commit:\n%s", log)
	}
	cached := string(testgit.Run(t, dir, "diff", "--cached", "--", "main.go"))
	if !strings.Contains(cached, "+two") {
		t.Fatalf("undo did not keep the changes staged:\n%s", cached)
	}
}

func TestStashAndPopFromSourceControl(t *testing.T) {
	dir := testgit.Repo(t)
	path := filepath.Join(dir, "main.go")
	testgit.Write(t, path, "one\n")
	testgit.Run(t, dir, "add", "main.go")
	testgit.Run(t, dir, "commit", "-m", "base")
	testgit.Write(t, path, "one\ntwo\n")

	a, queue := newDiffStageTestApp(t)
	a.PathInput = dir
	a.Open(dir)
	settleDiffStage(t, a, queue)
	a.RunWrite(domain.StashPush)
	settleDiffStage(t, a, queue)

	if status := strings.TrimSpace(string(testgit.Run(t, dir, "status", "--porcelain"))); status != "" {
		t.Fatalf("stash left worktree changes:\n%s", status)
	}
	if stashes := string(testgit.Run(t, dir, "stash", "list")); !strings.Contains(stashes, "stash@{0}") {
		t.Fatalf("stash was not created:\n%s", stashes)
	}

	a.RunWrite(domain.StashPop)
	settleDiffStage(t, a, queue)
	worktree := string(testgit.Run(t, dir, "diff", "--", "main.go"))
	if !strings.Contains(worktree, "+two") {
		t.Fatalf("stash pop did not restore changes:\n%s", worktree)
	}
}
