package sshfs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/ssh"
)

func sftpKeygen(t *testing.T, path string) {
	t.Helper()
	out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", path).CombinedOutput()
	if err != nil {
		t.Fatalf("keygen: %v %s", err, out)
	}
}

func sftpWrite(t *testing.T, path, value string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}

func newSFTPFixture(t *testing.T) Config {
	t.Helper()
	if _, err := os.Stat("/usr/sbin/sshd"); err != nil {
		t.Skip("sshd unavailable")
	}
	dir := t.TempDir()
	sftpKeygen(t, filepath.Join(dir, "host"))
	sftpKeygen(t, filepath.Join(dir, "identity"))
	public, err := os.ReadFile(filepath.Join(dir, "identity.pub"))
	if err != nil {
		t.Fatal(err)
	}
	sftpWrite(t, filepath.Join(dir, "authorized_keys"), string(public))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	current, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf(
		"Port %d\nListenAddress 127.0.0.1\nHostKey %s\nPidFile %s\nAuthorizedKeysFile %s\n"+
			"StrictModes no\nPasswordAuthentication no\nKbdInteractiveAuthentication no\nUsePAM no\n"+
			"AllowStreamLocalForwarding yes\nSubsystem sftp internal-sftp\nLogLevel ERROR\n",
		port, filepath.Join(dir, "host"), filepath.Join(dir, "pid"), filepath.Join(dir, "authorized_keys"),
	)
	sftpWrite(t, filepath.Join(dir, "sshd_config"), config)
	var log bytes.Buffer
	server := exec.Command("/usr/sbin/sshd", "-D", "-e", "-f", filepath.Join(dir, "sshd_config"))
	server.Stdout = &log
	server.Stderr = &log
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Wait() }()
	t.Cleanup(func() {
		_ = server.Process.Kill()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("sshd shutdown timed out")
		}
	})
	ready := false
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		select {
		case err := <-done:
			done <- err
			t.Skipf("isolated sshd unavailable: %v: %s", err, log.String())
		default:
		}
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 50*time.Millisecond)
		if err == nil {
			conn.Close()
			ready = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ready {
		t.Fatal("sshd did not listen")
	}
	hostKey, err := os.ReadFile(filepath.Join(dir, "host.pub"))
	if err != nil {
		t.Fatal(err)
	}
	known := filepath.Join(dir, "known_hosts")
	sftpWrite(t, known, fmt.Sprintf("[127.0.0.1]:%d %s", port, hostKey))
	clientConfig := filepath.Join(dir, "ssh_config")
	sftpWrite(t, clientConfig, fmt.Sprintf(
		"Host fixture\n HostName 127.0.0.1\n Port %d\n User %s\n IdentityFile %s\n IdentitiesOnly yes\n GlobalKnownHostsFile /dev/null\n",
		port, current.Username, filepath.Join(dir, "identity"),
	))
	pool := &ssh.Connections{}
	t.Cleanup(pool.Close)
	return Config{ConfigPath: clientConfig, KnownHostsFile: known, Connections: pool}
}

func TestSFTPReadWriteAtomicAndConfinement(t *testing.T) {
	cfg := newSFTPFixture(t)
	root := filepath.Join(t.TempDir(), "中文 root\n'")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "src 中文\nfile.go")
	if err := os.WriteFile(file, []byte("package old\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "skip"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skip", "ignored"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	client, err := Open(context.Background(), cfg, "fixture", root)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	executor := ssh.Executor{ConfigPath: cfg.ConfigPath, KnownHostsFile: cfg.KnownHostsFile, Connections: cfg.Connections}
	if _, err := executor.ReadConnectionArgs(context.Background(), "fixture"); err != nil {
		t.Fatal(err)
	}
	if _, err := executor.Open(context.Background(), "fixture", root); err == nil {
		t.Fatal("non-Git directory accepted")
	}
	// Prove a new editor/list subsystem reuses the existing authentication:
	// fresh SSH authentication is intentionally made impossible.
	configData, err := os.ReadFile(cfg.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	badConfig := strings.ReplaceAll(string(configData), "identity", "missing-identity")
	sftpWrite(t, cfg.ConfigPath, badConfig)
	defer sftpWrite(t, cfg.ConfigPath, string(configData))
	shared, err := Open(context.Background(), cfg, "fixture", root)
	if err != nil {
		t.Fatal("file list/editor opened a fresh connection:", err)
	}
	if _, err := shared.ReadDir(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if _, err := shared.ReadFile(context.Background(), file, 1<<20); err != nil {
		t.Fatal(err)
	}
	if err := shared.Close(); err != nil {
		t.Fatal(err)
	}
	// Git probes also run while the shared SFTP subsystem remains usable.
	canonicalRoot, _ := filepath.EvalSymlinks(root)
	if client.Root() != canonicalRoot {
		t.Fatalf("root = %q, want %q", client.Root(), canonicalRoot)
	}
	entries, err := client.ReadDir(context.Background(), root)
	if err != nil || len(entries) < 3 {
		t.Fatal(entries, err)
	}
	data, err := client.ReadFile(context.Background(), file, 1<<20)
	if err != nil || string(data) != "package old\n" {
		t.Fatal(string(data), err)
	}
	if _, err := client.ReadFile(context.Background(), filepath.Join(root, "escape"), 1<<20); err == nil {
		t.Fatal("symlink escape was readable")
	}
	if _, err := client.ReadFile(context.Background(), outside, 1<<20); err == nil {
		t.Fatal("outside root was readable")
	}
	if err := client.WriteFileAtomic(context.Background(), file, []byte("package new\n"), 0o640); err != nil {
		t.Fatalf("%v cause=%v", err, errors.Unwrap(err))
	}
	saved, err := os.ReadFile(file)
	if err != nil || string(saved) != "package new\n" {
		t.Fatal(string(saved), err)
	}
	info, err := os.Stat(file)
	if err != nil || info.Mode().Perm() != 0o640 {
		t.Fatal(info, err)
	}
	files, err := client.Walk(context.Background(), root, map[string]bool{"skip": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || !strings.Contains(files[0], "src") {
		t.Fatal(files)
	}
	if _, err := client.ReadFile(context.Background(), file, 4); err == nil {
		t.Fatal("size limit was not enforced")
	}
	var ge *domain.GitError
	if _, err := client.ReadFile(context.Background(), filepath.Join(root, "escape"), 1<<20); !errors.As(err, &ge) || ge.Code != domain.UnsafeOperation {
		t.Fatalf("escape error = %v", err)
	}
}

func TestSFTPBrowserStartsInRemoteHome(t *testing.T) {
	cfg := newSFTPFixture(t)
	client, err := Open(context.Background(), cfg, "fixture", "/")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	home, err := client.WorkingDirectory(context.Background())
	if err != nil || !filepath.IsAbs(home) {
		t.Fatalf("home=%q err=%v", home, err)
	}
	if _, err := client.ReadDir(context.Background(), home); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.WorkingDirectory(ctx); err == nil {
		t.Fatal("cancelled home lookup succeeded")
	}
}
