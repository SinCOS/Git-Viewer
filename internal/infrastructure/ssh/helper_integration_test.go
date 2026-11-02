//go:build unix

package ssh

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"gitviewer/internal/domain"
	"gitviewer/internal/testgit"
)

func integrationHelperBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "gitviewer-helper")
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	goPath := filepath.Join(root, ".tools/go/bin/go")
	if _, err := os.Stat(goPath); err != nil {
		goPath = "go"
	}
	cmd := exec.Command(goPath, "build", "-o", binary, "./cmd/gitviewer-helper")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("helper build: %v: %s", err, output)
	}
	return binary
}

type helperSSHFixture struct {
	executor    Executor
	dir, binary string
	helpers     *HelperPool
}

func newHelperSSHFixture(t *testing.T, sftpEnabled bool) *helperSSHFixture {
	t.Helper()
	if _, err := os.Stat("/usr/sbin/sshd"); err != nil {
		t.Skip("sshd unavailable")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	keygen(t, filepath.Join(dir, "host"))
	keygen(t, filepath.Join(dir, "identity"))
	pub, err := os.ReadFile(filepath.Join(dir, "identity.pub"))
	if err != nil {
		t.Fatal(err)
	}
	testgit.Write(t, filepath.Join(dir, "authorized_keys"), string(pub))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	config := fmt.Sprintf("Port %d\nListenAddress 127.0.0.1\nHostKey %s\nPidFile %s\nAuthorizedKeysFile %s\nStrictModes no\nPasswordAuthentication no\nKbdInteractiveAuthentication no\nUsePAM no\nLogLevel ERROR\n", port, filepath.Join(dir, "host"), filepath.Join(dir, "pid"), filepath.Join(dir, "authorized_keys"))
	if sftpEnabled {
		config += "Subsystem sftp internal-sftp\n"
	}
	testgit.Write(t, filepath.Join(dir, "sshd_config"), config)
	serverLog := &connectionOutput{}
	server := exec.Command("/usr/sbin/sshd", "-D", "-e", "-f", filepath.Join(dir, "sshd_config"))
	server.Stdout = serverLog
	server.Stderr = serverLog
	server.WaitDelay = 250 * time.Millisecond
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	stopped := make(chan error, 1)
	go func() { stopped <- server.Wait() }()
	t.Cleanup(func() {
		server.Process.Kill()
		select {
		case <-stopped:
		case <-time.After(3 * time.Second):
			t.Error("fixture sshd failed to stop")
		}
	})
	for deadline := time.Now().Add(3 * time.Second); ; {
		select {
		case err := <-stopped:
			stopped <- err
			t.Skipf("isolated sshd unavailable: %v %s", err, serverLog.bytes())
		default:
		}
		conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 50*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("sshd not ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	current, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	clientConfig := filepath.Join(dir, "ssh_config")
	testgit.Write(t, clientConfig, fmt.Sprintf("Host fixture\n HostName 127.0.0.1\n Port %d\n User %s\n IdentityFile %s\n IdentitiesOnly yes\n GlobalKnownHostsFile /dev/null\n", port, current.Username, filepath.Join(dir, "identity")))
	hostKey, err := os.ReadFile(filepath.Join(dir, "host.pub"))
	if err != nil {
		t.Fatal(err)
	}
	known := filepath.Join(dir, "known_hosts")
	testgit.Write(t, known, fmt.Sprintf("[127.0.0.1]:%d %s", port, hostKey))
	connections := &Connections{}
	t.Cleanup(connections.Close)
	executor := Executor{ConfigPath: clientConfig, KnownHostsFile: known, Connections: connections}
	helpers := NewHelperPool(executor)
	binary := integrationHelperBinary(t)
	helpers.LocalBinary = func(goos, goarch string) (string, error) {
		if goos != runtime.GOOS || goarch != runtime.GOARCH {
			return "", fmt.Errorf("unexpected loopback platform")
		}
		return binary, nil
	}
	helpers.RemoteBase = filepath.Join(dir, "helper-install", "v1")
	helpers.StartWait = 10 * time.Second
	t.Cleanup(helpers.Close)
	executor.Helpers = helpers
	return &helperSSHFixture{executor: executor, dir: dir, binary: binary, helpers: helpers}
}
func helperErrorCode(err error) domain.ErrorCode {
	var ge *domain.GitError
	if errors.As(err, &ge) {
		return ge.Code
	}
	return ""
}
func waitHelperMarker(t *testing.T, file string) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if _, err := os.Stat(file); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("fixture operation did not start")
}

func TestHelperOpenSSHIntegration(t *testing.T) {
	f := newHelperSSHFixture(t, true)
	ctx := context.Background()
	dir := filepath.Join(f.dir, "repo 中文 ' $()\n-end")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	testgit.Run(t, dir, "init", "-b", "main")
	testgit.Run(t, dir, "config", "user.name", "Test")
	testgit.Run(t, dir, "config", "user.email", "test@example.invalid")
	name := "nonutf8-" + string([]byte{255})
	if runtime.GOOS == "darwin" {
		name = "中文\nfile"
	} // APFS rejects invalid UTF-8 filenames; wire round-trips cover those bytes.
	testgit.Write(t, filepath.Join(dir, name), "one\n")
	repo, err := f.executor.Open(ctx, "fixture", dir)
	if err != nil {
		t.Fatal(err)
	}
	client, err := f.helpers.Acquire(ctx, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	pid := client.remotePID
	reader := f.executor.Reader("fixture")
	status, err := reader.Status(ctx, repo)
	if err != nil || len(status.Changes) != 1 || status.Changes[0].Path != name {
		t.Fatalf("byte path status: %+v %v", status, err)
	}
	for _, op := range []domain.GitOperation{{Operation: domain.Stage, Paths: []string{name}}, {Operation: domain.Commit, Message: "first helper commit"}} {
		op.RepositoryID = repo.Repository.RepositoryID
		result, err := reader.Write(ctx, repo, op)
		if err != nil || !result.Verified || result.Unknown {
			t.Fatalf("%s: %+v %v", op.Operation, result, err)
		}
	}
	history, err := reader.History(ctx, repo, "", 0, 10)
	if err != nil || len(history.Commits) != 1 {
		t.Fatalf("history %+v %v", history, err)
	}
	details, err := reader.CommitDetails(ctx, repo, history.Commits[0].OID)
	if err != nil || len(details.Files) != 1 || details.Files[0].Path != name {
		t.Fatalf("batch details %+v %v", details, err)
	}
	testgit.Write(t, filepath.Join(dir, name), "two\n")
	preview, err := reader.Diff(ctx, repo, domain.DiffRequest{Scope: domain.WorktreeDiff, Path: name})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Binary && !strings.Contains(preview.Text, "+two") {
		t.Fatalf("diff %q", preview.Text)
	}
	t.Run("cancel-hook-commit", func(t *testing.T) {
		hook := filepath.Join(dir, ".git/hooks/pre-commit")
		marker := filepath.Join(f.dir, "hook-started")
		testgit.Write(t, hook, "#!/bin/sh\nprintf started > '"+marker+"'\nsleep 30\n")
		os.Chmod(hook, 0700)
		testgit.Run(t, dir, "add", "--", name)
		operationCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		done := make(chan error, 1)
		go func() {
			_, err := reader.Write(operationCtx, repo, domain.GitOperation{RepositoryID: repo.Repository.RepositoryID, Operation: domain.Commit, Message: "cancelled"})
			done <- err
		}()
		waitHelperMarker(t, marker)
		cancel()
		select {
		case err := <-done:
			if helperErrorCode(err) != domain.OperationStateUnknown {
				t.Fatalf("expected unknown: %v", err)
			}
		case <-time.After(8 * time.Second):
			t.Fatal("canceled write stuck")
		}
		os.Remove(hook)
		if _, err := reader.Status(ctx, repo); err != nil {
			t.Fatalf("helper unusable after cancel: %v", err)
		}
		if !client.Alive() {
			t.Fatal("helper died after cancel")
		}
	})
	t.Run("read-death-falls-back", func(t *testing.T) {
		// Test a real in-flight framed read using a scoped fake Git executable;
		// CLI fallback still executes the ordinary system Git over OpenSSH.
		fake := filepath.Join(f.dir, "slow-git")
		marker := filepath.Join(f.dir, "read-started")
		testgit.Write(t, fake, "#!/bin/sh\nprintf started > '"+marker+"'\nsleep 30\n")
		os.Chmod(fake, 0700)
		child := exec.Command(os.Args[0], "-test.run=^TestHelperClientServer$")
		child.Env = append(os.Environ(), "GITVIEWER_HELPER_SERVER=1", "GITVIEWER_HELPER_GIT="+fake)
		slow, err := startHelperClient(ctx, child)
		if err != nil {
			t.Fatal(err)
		}
		pool := NewHelperPool(f.executor)
		defer pool.Close()
		pool.entries["fixture"] = &helperEntry{host: "fixture", client: slow}
		executor := f.executor
		executor.Helpers = pool
		done := make(chan error, 1)
		go func() { _, err := executor.Reader("fixture").Status(ctx, repo); done <- err }()
		waitHelperMarker(t, marker)
		child.Process.Signal(syscall.SIGTERM)
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("CLI fallback failed %v", err)
			}
		case <-time.After(8 * time.Second):
			t.Fatal("read fallback stuck")
		}
	})
	t.Run("write-death-never-retries", func(t *testing.T) {
		marker := filepath.Join(f.dir, "write-started")
		hook := filepath.Join(dir, ".git/hooks/pre-commit")
		testgit.Write(t, hook, "#!/bin/sh\nprintf 'attempt\\n' >> '"+marker+"'\nsleep 30\n")
		os.Chmod(hook, 0700)
		done := make(chan error, 1)
		go func() {
			_, err := reader.Write(ctx, repo, domain.GitOperation{RepositoryID: repo.Repository.RepositoryID, Operation: domain.Commit, Message: "killed"})
			done <- err
		}()
		waitHelperMarker(t, marker)
		// SIGTERM exercises helper signal handling: it cancels Git's entire group.
		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			if helperErrorCode(err) != domain.OperationStateUnknown {
				t.Fatalf("expected unknown: %v", err)
			}
		case <-time.After(12 * time.Second):
			t.Fatal("write death stuck")
		}
		data, err := os.ReadFile(marker)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Count(data, []byte("attempt\n")) != 1 {
			t.Fatalf("write retried: %q", data)
		}
		os.Remove(hook)
	})
	f.helpers.Close()
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		if syscall.Kill(pid, 0) == syscall.ESRCH {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("remote helper %d survived pool close", pid)
}
func TestHelperOpenSSHIntegrationRepairsCorruptBinary(t *testing.T) {
	f := newHelperSSHFixture(t, true)
	ctx := context.Background()
	ops, err := openSFTPHelperOps(ctx, f.executor, "fixture")
	if err != nil {
		t.Fatal(err)
	}
	defer ops.Close()
	installer := HelperInstaller{Ops: ops, LocalBinary: f.helpers.LocalBinary, RemoteBase: f.helpers.RemoteBase}
	installed, err := installer.Ensure(ctx)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(f.binary)
	if err != nil {
		t.Fatal(err)
	}
	corrupt := append([]byte(nil), original...)
	corrupt[len(corrupt)/2] ^= 0xff
	if err := os.WriteFile(installed.Path, corrupt, 0700); err != nil {
		t.Fatal(err)
	}
	// The original sidecar and file size still match; only hashing actual bytes
	// can detect this corruption across the real SFTP transport.
	repaired, err := installer.Ensure(ctx)
	if err != nil || repaired.Path != installed.Path {
		t.Fatalf("repair: %+v %v", repaired, err)
	}
	data, err := os.ReadFile(repaired.Path)
	if err != nil || sha256.Sum256(data) != sha256.Sum256(original) {
		t.Fatalf("uploaded bytes were not repaired: %v", err)
	}
	if _, err := f.helpers.Acquire(ctx, "fixture"); err != nil {
		t.Fatalf("repaired helper did not start: %v", err)
	}
}

func TestHelperOpenSSHIntegrationUnsupportedPlatform(t *testing.T) {
	f := newHelperSSHFixture(t, true)
	f.helpers.Ops = func(ctx context.Context, executor Executor, host string) (HelperRemoteOps, error) {
		ops, err := openSFTPHelperOps(ctx, executor, host)
		if err != nil {
			return nil, err
		}
		return unsupportedHelperOps{HelperRemoteOps: ops}, nil
	}
	repo, err := f.executor.Open(context.Background(), "fixture", testgit.Repo(t))
	if err != nil || repo.Repository.RepositoryID == "" {
		t.Fatalf("unsupported platform CLI fallback: %+v %v", repo, err)
	}
}

type unsupportedHelperOps struct{ HelperRemoteOps }

func (ops unsupportedHelperOps) Run(ctx context.Context, args []string, timeout time.Duration) (domain.GitResult, error) {
	if len(args) == 2 && args[0] == "uname" {
		return domain.GitResult{Stdout: []byte("unsupported\n")}, nil
	}
	return ops.HelperRemoteOps.Run(ctx, args, timeout)
}

func TestHelperOpenSSHIntegrationSFTPDisabled(t *testing.T) {
	f := newHelperSSHFixture(t, false)
	dir := testgit.Repo(t)
	repo, err := f.executor.Open(context.Background(), "fixture", dir)
	if err != nil {
		t.Fatal(err)
	}
	if repo.Repository.RepositoryID == "" {
		t.Fatal("CLI fallback did not open repo")
	}
}
