package local_test

import (
	"gitviewer/internal/testgit"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestOperationsLayoutLabels(t *testing.T) {
	a, _ := setup(t)
	a.ConnectionMenuOpen = true
	tt := ui.NewTester(a.View, 1600, 1000)
	tt.Frame()
	for _, label := range []string{"Commit and Push ▾", "Local Changes", "Connect SSH", "Fetch", "Push", "Repository:  ▾"} {
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
	// "Connect SSH" is Disabled(a.Loading) and OpenSSH has no Loading guard of
	// its own, so a click that fired would start a worker and enqueue a dispatch.
	if err := tt.Click("Connect SSH"); err != nil {
		t.Fatal(err)
	}
	a.Wait()
	if len(queue) != 0 {
		t.Fatal("button was not disabled while Loading")
	}
}

func TestCommitAndPushRequiresSuccessfulCommitAndSecondConfirmation(t *testing.T) {
	for _, scenario := range []string{"success", "cancel-commit", "cancel-push", "failed-commit"} {
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
			a.Confirm = func(_, detail string) bool {
				confirmations++
				if confirmations == 2 && !strings.Contains(detail, "Push") {
					return false
				}
				return scenario != "cancel-commit" && !(scenario == "cancel-push" && confirmations == 2)
			}
			a.CommitAndPush()
			settle(t, a, q)
			wantConfirmations := 2
			if scenario == "cancel-commit" || scenario == "failed-commit" {
				wantConfirmations = 1
			}
			if confirmations != wantConfirmations {
				t.Fatalf("confirmations=%d, want %d", confirmations, wantConfirmations)
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
