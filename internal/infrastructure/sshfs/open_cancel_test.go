package sshfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitviewer/internal/infrastructure/ssh"
)

func TestOpenCancellationStopsSubsystemHandshake(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "ssh")
	marker := filepath.Join(dir, "subsystem-started")
	script := `#!/bin/sh
while [ "$#" -gt 0 ]; do
 case "$1" in
 -o|-F) shift 2 ;;
 -S) socket=$2; shift 2 ;;
 -O) check=1; shift 2 ;;
 -N) master=1; shift ;;
 -T|-s) shift ;;
 *) break ;;
 esac
done
if [ "$check" = 1 ]; then test -e "$socket"; exit $?; fi
if [ "$master" = 1 ]; then touch "$socket"; exec sleep 30; fi
touch '` + marker + `'
exec sleep 30
`
	if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	pool := &ssh.Connections{}
	t.Cleanup(pool.Close)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		client, err := Open(ctx, Config{SSHPath: executable, Connections: pool}, "fixture", "/repo")
		if client != nil {
			client.Close()
		}
		done <- err
	}()
	for deadline := time.Now().Add(10 * time.Second); ; {
		select {
		case err := <-done:
			t.Fatalf("SFTP exited before subsystem handshake: %v", err)
		default:
		}
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("subsystem never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled open succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled SFTP initialization did not return")
	}
	// Cancelling this subsystem must not own the shared master.
	if _, err := (ssh.Executor{SSHPath: executable, Connections: pool}).ReadConnectionArgs(context.Background(), "fixture"); err != nil {
		t.Fatal(err)
	}
}
