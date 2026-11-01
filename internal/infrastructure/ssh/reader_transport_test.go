package ssh

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitviewer/internal/domain"
	"gitviewer/internal/testgit"
)

func TestRemoteReaderReadsHistoryAndCommitDiff(t *testing.T) {
	repo := testgit.Repo(t)
	subject := "remote transport reads history"
	testgit.Write(t, filepath.Join(repo, "feature.txt"), "first\nsecond\n")
	testgit.Run(t, repo, "add", "feature.txt")
	testgit.Run(t, repo, "commit", "-m", subject)
	oid := strings.TrimSpace(string(testgit.Run(t, repo, "rev-parse", "HEAD")))
	// Leave a worktree change: the commit diff must not depend on its status.
	testgit.Write(t, filepath.Join(repo, "feature.txt"), "first\nsecond\nthird\n")

	// A real remote shell crosses a quote boundary: model both the OpenSSH
	// words and the POSIX shell command received by the remote host. OpenSSH
	// joins its command words with spaces; parsing each word as an argv token
	// here would falsely accept the old invalid "--no-pager after -C" order.
	dir := t.TempDir()
	script := filepath.Join(dir, "ssh-git")
	logPath := filepath.Join(t.TempDir(), "remote-git-log")
	controlLog := filepath.Join(t.TempDir(), "control-log")
	testgit.Write(t, script, `#!/bin/sh
while [ "$#" -gt 0 ]; do
  case "$1" in
    -o) shift 2 ;;
    -F) shift 2 ;;
    -S) socket=$2; shift 2 ;;
    -O) check=1; shift 2 ;;
    -N) master=1; shift ;;
    -T|-s) shift ;;
    -M) shift ;;
    *) break ;;
  esac
done
if [ "$check" = 1 ]; then test -e "$socket"; exit $?; fi
if [ "$master" = 1 ]; then touch "$socket"; exec sleep 30; fi
target=$1
shift
if [ "$target" != "fixture" ]; then
  exec /usr/bin/ssh "$target" "$@"
fi
remote=""
for word in "$@"; do
  if [ -n "$remote" ]; then remote="$remote "; fi
  remote="$remote$word"
done
printf '%s\n' "$remote" >> "`+logPath+`"
printf '%s\n' "$socket" >> "`+controlLog+`"
exec sh -c "$remote"
`)
	if err := os.Chmod(script, 0700); err != nil {
		t.Fatal(err)
	}
	pool := &Connections{}
	t.Cleanup(pool.Close)
	executor := Executor{SSHPath: script, Connections: pool}
	opened, err := executor.Open(context.Background(), "fixture", repo)
	if err != nil {
		t.Fatal(err)
	}
	reader := executor.Reader("fixture")
	page, err := reader.History(context.Background(), opened, "", 0, 100)
	if err != nil || len(page.Commits) != 1 || page.Commits[0].OID != oid || page.Commits[0].Subject != subject {
		t.Fatalf("history = %+v, err = %v", page, err)
	}
	details, err := reader.CommitDetails(context.Background(), opened, oid)
	if err != nil || details.Commit.OID != oid || len(details.Files) != 1 || details.Files[0].Path != "feature.txt" {
		t.Fatalf("details = %+v, err = %v", details, err)
	}
	preview, err := reader.Diff(context.Background(), opened, domain.DiffRequest{Scope: domain.CommitDiff, Commit: oid, Path: "feature.txt", OldPath: "feature.txt", NewPath: "feature.txt"})
	if err != nil || !strings.Contains(preview.Text, "+second") {
		t.Fatalf("diff = %q, err = %v", preview.Text, err)
	}
	written, err := reader.Write(context.Background(), opened, domain.GitOperation{RepositoryID: opened.Repository.RepositoryID, Operation: domain.Stage, Paths: []string{"feature.txt"}})
	if err != nil || !written.Verified {
		t.Fatal(written, err)
	}
	controls, err := os.ReadFile(controlLog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(controls), "none\n") || !strings.Contains(string(controls), "/gv-ssh-") {
		t.Fatalf("reads and writes did not use separate transports: %q", controls)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(logged)), "\n") {
		noPager := strings.Index(line, "'--no-pager'")
		changeDirectory := strings.Index(line, "'-C'")
		if noPager < 0 || changeDirectory < 0 || noPager > changeDirectory {
			t.Fatalf("remote Git options were rejected by the old-Git parser: %q", line)
		}
	}
}
