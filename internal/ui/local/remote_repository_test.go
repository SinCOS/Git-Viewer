package local

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/ssh"
	"gitviewer/internal/infrastructure/storage"
	"gitviewer/internal/testgit"
)

func commitFixture(t *testing.T, dir, file, subject string) string {
	t.Helper()
	testgit.Write(t, filepath.Join(dir, file), subject+"\n")
	testgit.Run(t, dir, "add", file)
	testgit.Run(t, dir, "commit", "-m", subject)
	return strings.TrimSpace(string(testgit.Run(t, dir, "rev-parse", "HEAD")))
}

func newRepositoryApp(t *testing.T) (*App, chan func()) {
	t.Helper()
	a := New(application.LocalService{Recent: storage.RecentRepositories{Path: filepath.Join(t.TempDir(), "recent.json")}})
	queue := make(chan func(), 32)
	a.Dispatch = func(apply func()) { queue <- apply }
	t.Cleanup(func() { a.Close(); a.Wait() })
	return a, queue
}

func settleOperation(t *testing.T, a *App, queue chan func()) {
	t.Helper()
	for a.Loading {
		select {
		case apply := <-queue:
			apply()
		case <-time.After(10 * time.Second):
			t.Fatal("operation did not finish")
		}
	}
}

func applyQueued(t *testing.T, queue chan func()) {
	t.Helper()
	select {
	case apply := <-queue:
		apply()
	case <-time.After(5 * time.Second):
		t.Fatal("queued result did not arrive")
	}
}

func installSSHRepository(a *App) {
	a.Repository = domain.RepositoryContext{Repository: domain.Repository{
		RepositoryID: "ssh-fixture", Location: "ssh", Path: "/srv/project",
	}, SSHHost: "fixture"}
	a.applyStatus(domain.GitStatus{RepositoryID: "ssh-fixture"})
	a.resetRepositoryViews()
}

func TestRepositorySwitchClearsHistoryAndCommitPreview(t *testing.T) {
	first, second := testgit.Repo(t), testgit.Repo(t)
	firstOID := commitFixture(t, first, "first.txt", "first repository")
	secondOID := commitFixture(t, second, "second.txt", "second repository")

	a, queue := newRepositoryApp(t)
	a.Open(first)
	settleOperation(t, a, queue)
	a.Pane = "History"
	tt := ui.NewTester(a.View, 1400, 850)
	tt.Frame()
	settleOperation(t, a, queue)
	tt.Frame()
	if err := tt.Click("first repository"); err != nil {
		t.Fatal(err)
	}
	settleOperation(t, a, queue)
	if a.CommitDetails.Commit.OID != firstOID || a.Preview.Scope != domain.CommitDiff {
		t.Fatal("first commit was not selected")
	}

	a.Open(second)
	settleOperation(t, a, queue)
	if len(a.GraphRows) != 0 || a.CommitDetails.Commit.OID != "" || a.CommitInput != "" || a.Preview.Scope != "" {
		t.Fatal("history or commit preview survived a repository switch")
	}
	tt.Frame()
	settleOperation(t, a, queue)
	tt.Frame()
	if len(a.GraphRows) != 1 || a.GraphRows[0].Commit.OID != secondOID {
		t.Fatal("the Git Log did not load the newly opened repository")
	}
}

func TestLateResultsCannotReplaceCurrentRepositoryViews(t *testing.T) {
	first, second := testgit.Repo(t), testgit.Repo(t)
	commitFixture(t, first, "first.txt", "old repository")
	secondOID := commitFixture(t, second, "second.txt", "new repository")

	a, queue := newRepositoryApp(t)
	a.Open(first)
	settleOperation(t, a, queue)
	a.LoadHistory(false)
	staleHistoryApply := <-queue
	a.Open(second)
	settleOperation(t, a, queue)
	staleHistoryApply()
	if len(a.GraphRows) != 0 {
		t.Fatal("late history overwrote the new repository")
	}

	a.CommitInput = secondOID
	a.CommitDiff()
	staleDiffApply := <-queue
	installSSHRepository(a)
	staleDiffApply()
	if a.CommitDetails.Commit.OID != "" || a.Preview.Scope != "" || len(a.lines) != 0 {
		t.Fatal("late commit diff overwrote the remote repository")
	}
}

func TestHistoryRetriesForNewRepositoryAfterEmptyResult(t *testing.T) {
	a, queue := newRepositoryApp(t)
	a.Mode, a.Pane, a.railPane = "log", "History", "History"
	a.Repository.Repository.RepositoryID = "ssh-empty"
	a.historyAttempted, a.historyAttemptedRepoID = true, "ssh-empty"
	a.Repository.Repository.RepositoryID = "ssh-other"
	a.ensureHistory()
	if !a.Loading {
		t.Fatal("history was not retried for a different repository")
	}
	applyQueued(t, queue)
}

func TestRemoteGitLogFailuresNeverReuseLocalHistory(t *testing.T) {
	local := testgit.Repo(t)
	commitFixture(t, local, "local.txt", "local history")
	a, queue := newRepositoryApp(t)
	// This mirrors the repository context after SSH opens but nobody has set
	// up remote commands. Falling back to the local reader would be a bug.
	a.Open(local)
	settleOperation(t, a, queue)
	a.Pane = "History"
	settleOperation(t, a, queue)
	installSSHRepository(a)
	a.Service.Remote = ssh.Executor{SSHPath: "/usr/bin/false"}
	a.LoadHistory(false)
	applyQueued(t, queue)
	if !a.Stale || len(a.GraphRows) != 0 || a.CommitDetails.Commit.OID != "" || a.Preview.Scope != "" {
		t.Fatalf("remote failure reused local data: stale=%t rows=%d message=%q", a.Stale, len(a.GraphRows), a.Message)
	}
}

func TestRemoteFolderOpenedWithNoGitClearsPreviousPanels(t *testing.T) {
	a := New(application.LocalService{
		Recent: storage.RecentRepositories{Path: filepath.Join(t.TempDir(), "recent.json")},
		Remote: ssh.Executor{SSHPath: filepath.Join(t.TempDir(), "missing-ssh")},
	})
	queue := make(chan func(), 2)
	a.Dispatch = func(apply func()) { queue <- apply }
	t.Cleanup(func() { a.Close(); a.Wait() })
	a.Repository = domain.RepositoryContext{Repository: domain.Repository{
		RepositoryID: "local-old", Location: "local", Path: t.TempDir(),
	}}
	a.CommitDetails.Commit.Subject = "old commit"
	a.Preview = domain.DiffPreview{Scope: domain.CommitDiff, Text: "old diff"}
	a.GraphRows = a.Graph.Append([]domain.CommitRecord{{OID: strings.Repeat("a", 40), Subject: "old history"}})
	folder := t.TempDir()
	a.applyRemoteWorkspace(application.RemoteWorkspaceOpenResult{
		Roots: []domain.EditorRoot{{Name: filepath.Base(folder), Location: domain.LocationSSH, SSHHost: "fixture", Path: folder}},
	}, "fixture", "", nil)
	if a.Repository.Repository.RepositoryID != "" || len(a.GraphRows) != 0 || a.CommitDetails.Commit.Subject != "" || a.Preview.Scope != "" {
		t.Fatal("a non-Git remote folder retained previous repository panels")
	}
}
