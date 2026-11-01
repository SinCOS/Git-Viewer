package local

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
	"gitviewer/internal/testgit"
)

func commitPerformanceApp(t *testing.T) (*App, chan func()) {
	t.Helper()
	a := checkboxApp(t)
	queue := make(chan func(), 32)
	a.Dispatch = func(apply func()) { queue <- apply }
	return a, queue
}

func TestStageKeepsUnrelatedPreviewAndHistory(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Write(t, filepath.Join(dir, "selected.txt"), "base\n")
	testgit.Write(t, filepath.Join(dir, "other.txt"), "base\n")
	testgit.Run(t, dir, "add", ".")
	testgit.Run(t, dir, "commit", "-m", "base")
	testgit.Write(t, filepath.Join(dir, "selected.txt"), "selected\n")
	testgit.Write(t, filepath.Join(dir, "other.txt"), "other\n")
	a, queue := commitPerformanceApp(t)
	a.Open(dir)
	drainDialog(t, a, queue)
	a.LoadHistory(false)
	drainDialog(t, a, queue)
	a.Pane = "Status"
	for i, e := range a.Entries {
		if e.Change.Path == "selected.txt" {
			a.Select(i)
			break
		}
	}
	drainDialog(t, a, queue)
	before := a.Preview
	a.StatusList.ScrollTo(1, ui.Start)
	a.DiffList.ScrollTo(1, ui.Start)
	for i, e := range a.Entries {
		if e.Change.Path == "other.txt" {
			a.stageEntry(i, true)
			break
		}
	}
	drainDialog(t, a, queue)
	if a.Preview != before || a.Selected < 0 || a.Entries[a.Selected].Change.Path != "selected.txt" {
		t.Fatalf("staging another row cleared the active diff: selected=%d preview=%+v", a.Selected, a.Preview)
	}
	if len(a.GraphRows) != 1 {
		t.Fatal("index-only write invalidated immutable history")
	}
}

func TestStageRemovesAffectedPreview(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Write(t, filepath.Join(dir, "file"), "new\n")
	a, queue := commitPerformanceApp(t)
	a.Open(dir)
	drainDialog(t, a, queue)
	a.Select(0)
	drainDialog(t, a, queue)
	a.stageEntry(0, true)
	drainDialog(t, a, queue)
	if a.Preview.Scope != "" || a.Selected != -1 {
		t.Fatal("staged file retained outdated worktree preview", a.Preview, a.Selected)
	}
}

func TestSlowStageDoesNotBlockFrames(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	reader := gitexec.LocalReader{Transport: func(ctx context.Context, _ string, _ int, _ []byte, args ...string) (domain.GitResult, error) {
		switch args[0] {
		case "status":
			return domain.GitResult{Stdout: []byte(" M file\x00")}, nil
		case "add":
			close(started)
			select {
			case <-release:
			case <-ctx.Done():
				return domain.GitResult{}, ctx.Err()
			}
			return domain.GitResult{}, nil
		default:
			t.Errorf("unexpected query %v", args)
			return domain.GitResult{}, nil
		}
	}}
	a := checkboxApp(t)
	a.Service = application.LocalService{Reader: reader, Recent: a.Service.Recent}
	a.Repository.CommonDir = "/tmp/mock/.git"
	a.Entries = []domain.StatusEntry{{Change: domain.Change{Path: "file", IndexStatus: " ", WorktreeStatus: "M"}, Scope: domain.WorktreeDiff, Group: "Unstaged"}}
	queue := make(chan func(), 16)
	a.Dispatch = func(apply func()) { queue <- apply }
	a.stageEntry(0, true)
	<-started
	// Always release the worker even if a frame assertion fails.
	defer func() { close(release); drainDialog(t, a, queue) }()
	tt := ui.NewTester(a.View, 1000, 650)
	tt.Frame()
	if !a.Writing || !strings.Contains(a.Message, "waiting for Git") || !tt.HasText("SOURCE CONTROL") {
		t.Fatal("slow Git hid or blocked source control")
	}
}
