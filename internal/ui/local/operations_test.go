package local_test

import (
	"gitviewer/internal/domain"
	"gitviewer/internal/testgit"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestCommitFormShowsWriteFailureUntilRetry(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Write(t, filepath.Join(dir, "file"), "staged\n")
	testgit.Run(t, dir, "add", "file")
	hook := filepath.Join(dir, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
		t.Fatal(err)
	}
	a, q := setup(t)
	a.Open(dir)
	settle(t, a, q)
	a.MessageInput = "retry commit"
	a.RunWrite(domain.Commit)
	settle(t, a, q)
	tt := ui.NewTester(a.View, 1600, 1000)
	tt.Frame()
	if !tt.HasText("Commit form error") || !strings.Contains(a.Message, "CommandFailed") || a.MessageInput != "retry commit" {
		t.Fatalf("missing form error or lost message: %s %v", a.Message, tt.Texts())
	}
	a.Refresh()
	settle(t, a, q)
	tt.Frame()
	if !tt.HasText("Commit form error") {
		t.Fatal("refresh cleared the form error")
	}
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	a.RunWrite(domain.Commit)
	settle(t, a, q)
	tt.Frame()
	if tt.HasText("Commit form error") || a.MessageInput != "" {
		t.Fatalf("successful retry did not clear the form: %s %v", a.Message, tt.Texts())
	}
}

func TestAmendAndPushWithoutConfirmation(t *testing.T) {
	for _, scenario := range []string{"success", "hook-failure"} {
		t.Run(scenario, func(t *testing.T) {
			dir := testgit.Repo(t)
			testgit.Write(t, filepath.Join(dir, "file"), "base\n")
			testgit.Run(t, dir, "add", "file")
			testgit.Run(t, dir, "commit", "-m", "original")
			before := string(testgit.Run(t, dir, "rev-parse", "HEAD"))
			if scenario == "hook-failure" {
				hook := filepath.Join(dir, ".git", "hooks", "pre-commit")
				if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
					t.Fatal(err)
				}
			}
			a, q := setup(t)
			a.Open(dir)
			settle(t, a, q)
			a.Amend = true
			a.MessageInput = "amended message"
			confirmations := 0
			a.Confirm = func(_, _ string) bool {
				confirmations++
				return true
			}
			a.CommitAndPush()
			settle(t, a, q)
			after := string(testgit.Run(t, dir, "rev-parse", "HEAD"))
			if confirmations != 0 {
				t.Fatalf("routine amend/push requested %d confirmations", confirmations)
			}
			if scenario == "success" {
				if before == after || a.Amend || a.MessageInput != "" {
					t.Fatalf("successful amend did not clear the form: state=%t message=%q status=%s", a.Amend, a.MessageInput, a.Message)
				}
				if got := strings.TrimSpace(string(testgit.Run(t, dir, "log", "-1", "--format=%s"))); got != "amended message" {
					t.Fatalf("amended subject = %q", got)
				}
			} else if before != after || !a.Amend || a.MessageInput != "amended message" {
				t.Fatalf("failed amend changed state: state=%t message=%q status=%s", a.Amend, a.MessageInput, a.Message)
			}
		})
	}
}

func TestOperationsLayoutLabels(t *testing.T) {
	a, _ := setup(t)
	a.ConnectionMenuOpen = true
	tt := ui.NewTester(a.View, 1600, 1000)
	tt.Frame()
	for _, label := range []string{"Commit", "Local Changes", "Amend last commit", "Diff Preview"} {
		if !tt.HasText(label) {
			t.Fatalf("missing button label %q in %v", label, tt.Texts())
		}
	}
}

func TestOperationsDisabledWhileLoading(t *testing.T) {
	a, queue := setup(t)
	a.Loading = true
	a.ConnectionMenuOpen = true
	tt := ui.NewTester(a.View, 1600, 1000)
	tt.Frame()
	// "Connect to Host…" is Disabled(a.Loading) and OpenSSH has no Loading guard of
	// its own, so a click that fired would start a worker and enqueue a dispatch.
	if err := tt.Click("Connect to Host…"); err != nil {
		t.Fatal(err)
	}
	a.Wait()
	if len(queue) != 0 {
		t.Fatal("button was not disabled while Loading")
	}
}

func TestCommitAndPushWithoutConfirmation(t *testing.T) {
	for _, scenario := range []string{"success", "failed-commit"} {
		t.Run(scenario, func(t *testing.T) {
			dir := testgit.Repo(t)
			testgit.Write(t, filepath.Join(dir, "file"), "base\n")
			testgit.Run(t, dir, "add", ".")
			testgit.Run(t, dir, "commit", "-m", "base")
			remote := filepath.Join(t.TempDir(), "remote.git")
			testgit.Run(t, dir, "init", "--bare", remote)
			testgit.Run(t, dir, "remote", "add", "origin", remote)
			branch := strings.TrimSpace(string(testgit.Run(t, dir, "branch", "--show-current")))
			testgit.Run(t, dir, "push", "-u", "origin", branch)
			before := string(testgit.Run(t, remote, "rev-parse", "refs/heads/"+branch))
			if scenario != "failed-commit" {
				testgit.Write(t, filepath.Join(dir, "file"), "updated\n")
				testgit.Run(t, dir, "add", ".")
			}
			a, q := setup(t)
			a.Open(dir)
			settle(t, a, q)
			a.MessageInput = "commit then push"
			confirmations := 0
			a.Confirm = func(_, _ string) bool {
				confirmations++
				return true
			}
			a.CommitAndPush()
			settle(t, a, q)
			if confirmations != 0 {
				t.Fatalf("routine commit/push requested %d confirmations", confirmations)
			}
			after := string(testgit.Run(t, remote, "rev-parse", "refs/heads/"+branch))
			if (after != before) != (scenario == "success") {
				t.Fatalf("unexpected remote update for %s: %s", scenario, a.Message)
			}
			if scenario == "success" {
				if a.Stale || len(a.Entries) != 0 || a.Writing {
					t.Fatal(a.Message, a.Entries)
				}
				if after != string(testgit.Run(t, dir, "rev-parse", "HEAD")) {
					t.Fatal("remote did not receive commit")
				}
			}
		})
	}
}
