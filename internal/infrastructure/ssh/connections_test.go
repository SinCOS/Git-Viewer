package ssh

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gitviewer/internal/domain"
	"gitviewer/internal/testgit"
)

func TestConnectionsLifecycle(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ssh")
	starts := filepath.Join(dir, "starts")
	// The fixture models control operations, a foreground master, and clients
	// separately. No user SSH configuration or credentials are involved.
	testgit.Write(t, script, `#!/bin/sh
while [ "$#" -gt 0 ]; do
 case "$1" in
  -o|-F) shift 2 ;;
  -S) socket=$2; shift 2 ;;
  -O) check=1; shift 2 ;;
  -N) master=1; shift ;;
  -T) shift ;;
  *) break ;;
 esac
done
if [ "$check" = 1 ]; then test -e "$socket"; exit $?; fi
if [ "$master" = 1 ]; then
 printf 'start\n' >> '`+starts+`'
 if [ -f '`+dir+`/reject' ]; then printf 'Permission denied' >&2; exit 255; fi
 touch "$socket"
 exec sleep 30
fi
shift
exec sh -c "$*"
`)
	if err := os.Chmod(script, 0700); err != nil {
		t.Fatal(err)
	}
	pool := &Connections{}
	t.Cleanup(pool.Close)
	e := Executor{SSHPath: script, Connections: pool}

	// Auth failure cannot leave a ready connection behind.
	testgit.Write(t, filepath.Join(dir, "reject"), "")
	_, err := e.ReadConnectionArgs(context.Background(), "fixture")
	var ge *domain.GitError
	if !errors.As(err, &ge) || ge.Code != domain.SshAuthFailed {
		t.Fatal(err)
	}
	pool.mu.Lock()
	var failed *connection
	for _, c := range pool.entries {
		failed = c
	}
	pool.mu.Unlock()
	<-failed.done
	if err := os.Remove(filepath.Join(dir, "reject")); err != nil {
		t.Fatal(err)
	}

	// Concurrent first callers all share exactly one authenticated master.
	const count = 12
	paths := make(chan string, count)
	var wg sync.WaitGroup
	for range count {
		wg.Go(func() {
			args, err := e.ReadConnectionArgs(context.Background(), "fixture")
			if err != nil {
				t.Error(err)
				return
			}
			paths <- args[len(args)-1]
		})
	}
	wg.Wait()
	close(paths)
	var socket string
	for p := range paths {
		if socket != "" && socket != p {
			t.Fatal("concurrent callers created different masters")
		}
		socket = p
	}
	data, err := os.ReadFile(starts)
	if err != nil || strings.Count(string(data), "start\n") != 2 {
		t.Fatalf("master starts=%q err=%v", data, err)
	}
	terminalArgs, err := e.SessionArgs(context.Background(), "fixture", TerminalSession)
	if err != nil || terminalArgs[0] != "-tt" || terminalArgs[len(terminalArgs)-1] != socket {
		t.Fatalf("terminal did not share authentication: %v %v", terminalArgs, err)
	}
	sftpCommand, err := e.Command(context.Background(), "fixture", SharedSession, nil, "sftp")
	if err != nil || !strings.Contains(strings.Join(sftpCommand.Args, " "), socket) {
		t.Fatalf("SFTP did not share authentication: %v %v", sftpCommand, err)
	}
	info, err := os.Stat(filepath.Dir(socket))
	if err != nil || info.Mode().Perm() != 0700 || len(socket) >= 100 {
		t.Fatalf("unsafe socket directory: %v %v", info, err)
	}

	// Cancelling a query must leave the master available to the next query.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = e.runReadRemote(ctx, "fixture", time.Second, 1024, []string{"sleep", "30"})
	if !errors.As(err, &ge) || ge.Code != domain.OperationTimedOut {
		t.Fatal(err)
	}
	result, err := e.runReadRemote(context.Background(), "fixture", time.Second, 1024, []string{"printf", "ok"})
	if err != nil || !bytes.Equal(result.Stdout, []byte("ok")) {
		t.Fatal(result, err)
	}

	c, err := pool.get(context.Background(), e, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	c.cancel()
	<-c.done
	args, err := e.ReadConnectionArgs(context.Background(), "fixture")
	if err != nil || args[len(args)-1] == socket {
		t.Fatalf("dead master was reused: %v %v", args, err)
	}
	newSocket := args[len(args)-1]
	pool.Close()
	for _, p := range []string{socket, newSocket} {
		if _, err := os.Stat(filepath.Dir(p)); !os.IsNotExist(err) {
			t.Fatalf("socket directory was not removed: %s %v", p, err)
		}
	}
	if _, err := e.ReadConnectionArgs(context.Background(), "fixture"); err == nil {
		t.Fatal("closed pool accepted new requests")
	}
}

func TestReadBatchPreservesBinaryAndLimits(t *testing.T) {
	// The real shell fixture used by the reader test supports the same master
	// lifecycle; use real Git output here including NUL and empty results.
	repo := testgit.Repo(t)
	testgit.Write(t, filepath.Join(repo, "line\nbreak"), "x")
	testgit.Run(t, repo, "add", ".")
	testgit.Run(t, repo, "commit", "-m", "batch reads")
	dir := t.TempDir()
	script := filepath.Join(dir, "ssh")
	testgit.Write(t, script, `#!/bin/sh
while [ "$#" -gt 0 ]; do
 case "$1" in
  -o|-F) shift 2 ;;
  -S) socket=$2; shift 2 ;;
  -O) check=1; shift 2 ;;
  -N) master=1; shift ;;
  -T) shift ;;
  *) break ;;
 esac
done
if [ "$check" = 1 ]; then test -e "$socket"; exit $?; fi
if [ "$master" = 1 ]; then touch "$socket"; exec sleep 30; fi
shift
exec sh -c "$*"
`)
	if err := os.Chmod(script, 0700); err != nil {
		t.Fatal(err)
	}
	pool := &Connections{}
	t.Cleanup(pool.Close)
	e := Executor{SSHPath: script, Connections: pool}
	commands := [][]string{{"ls-files", "-z"}, {"status", "--porcelain=v1", "-z"}}
	results, err := e.runReadBatch(context.Background(), "fixture", repo, 1024, commands)
	if err != nil || len(results) != 2 || !bytes.Equal(results[0].Stdout, []byte("line\nbreak\x00")) || len(results[1].Stdout) != 0 {
		t.Fatalf("results=%+v err=%v", results, err)
	}
	_, err = e.runReadBatch(context.Background(), "fixture", repo, 2, commands)
	var ge *domain.GitError
	if !errors.As(err, &ge) || ge.Code != domain.OutputLimitExceeded {
		t.Fatal(err)
	}
	_, err = e.runReadBatch(context.Background(), "fixture", repo, 1024, [][]string{{"rev-parse", "--verify", "refs/heads/missing"}})
	if !errors.As(err, &ge) || ge.Code != domain.CommandFailed {
		t.Fatal(err)
	}
	_, err = e.runReadBatch(context.Background(), "fixture", repo, 1024, [][]string{{"stash", "push"}})
	if !errors.As(err, &ge) || ge.Code != domain.UnsafeOperation {
		t.Fatal(err)
	}
}
