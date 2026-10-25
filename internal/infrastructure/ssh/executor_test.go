package ssh

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
	"gitviewer/internal/testgit"
)

func TestEncodeShellRoundTrip(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "INJECTED")
	values := []string{"", "space 中文", "line\nbreak", "single'quote", "-leading", "$(touch " + marker + ")", "`touch " + marker + "`", "; touch " + marker, "\\\"$HOME"}
	encoded, err := Encode(append([]string{"printf", "%s\\0"}, values...))
	if err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("/bin/sh", "-c", encoded).Output()
	if err != nil {
		t.Fatal(err)
	}
	expected := []byte(strings.Join(values, "\x00") + "\x00")
	if !bytes.Equal(out, expected) {
		t.Fatalf("%q != %q", out, expected)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("shell injection")
	}
	if _, err := Encode([]string{"bad\x00arg"}); err == nil {
		t.Fatal("NUL accepted")
	}
}
func TestRejectHostInjection(t *testing.T) {
	for _, host := range []string{"-oProxyCommand=touch bad", "host;touch bad", "user@host", "host\nother", ""} {
		c := remoteCommand("/tmp/repo")
		c.Repository.SSHHost = host
		if _, err := (Executor{}).Execute(context.Background(), c); err == nil {
			t.Fatal(host)
		}
	}
}
func remoteCommand(dir string) domain.GitCommand {
	c := testgit.Command(dir, domain.Status)
	c.Repository.Repository.Location = "ssh"
	c.Repository.SSHHost = "fixture"
	c.Timeout = 5 * time.Second
	return c
}
func keygen(t *testing.T, path string) {
	t.Helper()
	out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", path).CombinedOutput()
	if err != nil {
		t.Fatalf("keygen: %v %s", err, out)
	}
}

// The server listens only on loopback, uses fresh keys, and has no relation to
// the user's ~/.ssh configuration or the machine's remote-login service.
func TestOpenSSHIntegration(t *testing.T) {
	if _, err := os.Stat("/usr/sbin/sshd"); err != nil {
		t.Skip("sshd unavailable: ", err)
	}
	dir := t.TempDir()
	keygen(t, filepath.Join(dir, "host"))
	keygen(t, filepath.Join(dir, "identity"))
	keygen(t, filepath.Join(dir, "wrong"))
	identity, err := os.ReadFile(filepath.Join(dir, "identity.pub"))
	if err != nil {
		t.Fatal(err)
	}
	testgit.Write(t, filepath.Join(dir, "authorized_keys"), string(identity))
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
	serverConfig := fmt.Sprintf("Port %d\nListenAddress 127.0.0.1\nHostKey %s\nPidFile %s\nAuthorizedKeysFile %s\nStrictModes no\nPasswordAuthentication no\nKbdInteractiveAuthentication no\nUsePAM no\nLogLevel ERROR\n", port, filepath.Join(dir, "host"), filepath.Join(dir, "pid"), filepath.Join(dir, "authorized_keys"))
	testgit.Write(t, filepath.Join(dir, "sshd_config"), serverConfig)
	var serverLog bytes.Buffer
	server := exec.Command("/usr/sbin/sshd", "-D", "-e", "-f", filepath.Join(dir, "sshd_config"))
	server.Stdout = &serverLog
	server.Stderr = &serverLog
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
	for i := 0; i < 100; i++ {
		select {
		case err := <-done:
			done <- err
			t.Skipf("isolated sshd unavailable: %v: %s", err, serverLog.String())
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
	testgit.Write(t, known, fmt.Sprintf("[127.0.0.1]:%d %s", port, hostKey))
	config := filepath.Join(dir, "ssh_config")
	writeConfig := func(identity string) {
		testgit.Write(t, config, fmt.Sprintf("Host fixture\n HostName 127.0.0.1\n Port %d\n User %s\n IdentityFile %s\n IdentitiesOnly yes\n GlobalKnownHostsFile /dev/null\n", port, current.Username, identity))
	}
	writeConfig(filepath.Join(dir, "identity"))
	executor := Executor{ConfigPath: config, KnownHostsFile: known}
	repo := filepath.Join(dir, "repo 中文 ' $(touch INJECTED)\n-end")
	if err := os.Mkdir(repo, 0700); err != nil {
		t.Fatal(err)
	}
	testgit.Run(t, repo, "init", "-b", "main")
	testgit.Write(t, filepath.Join(repo, "中文\nfile"), "data")
	result, err := executor.Execute(context.Background(), remoteCommand(repo))
	if err != nil {
		t.Fatalf("SSH Config/remote Git: %v stderr=%s", err, result.Stderr)
	}
	if !bytes.Contains(result.Stdout, []byte("中文\nfile\x00")) {
		t.Fatalf("%q", result.Stdout)
	}
	t.Log("real loopback SSH Config, host key verification and remote Git passed")
	for _, tc := range []struct {
		name   string
		change func()
		code   domain.ErrorCode
	}{
		{"unknown-host", func() { testgit.Write(t, known, "") }, domain.HostKeyRejected},
		{"changed-host", func() {
			wrong, _ := os.ReadFile(filepath.Join(dir, "wrong.pub"))
			testgit.Write(t, known, fmt.Sprintf("[127.0.0.1]:%d %s", port, wrong))
		}, domain.HostKeyRejected},
		{"authentication", func() {
			testgit.Write(t, known, fmt.Sprintf("[127.0.0.1]:%d %s", port, hostKey))
			writeConfig(filepath.Join(dir, "wrong"))
		}, domain.SshAuthFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.change()
			result, err := executor.Execute(context.Background(), remoteCommand(repo))
			var ge *domain.GitError
			if !errors.As(err, &ge) || ge.Code != tc.code {
				t.Fatalf("%v %s", err, result.Stderr)
			}
		})
	}
	writeConfig(filepath.Join(dir, "identity"))
	testgit.Write(t, known, fmt.Sprintf("[127.0.0.1]:%d %s", port, hostKey))
	t.Run("cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := executor.Execute(ctx, remoteCommand(repo))
		var ge *domain.GitError
		if !errors.As(err, &ge) || ge.Code != domain.OperationCancelled {
			t.Fatal(err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		c := remoteCommand(repo)
		c.Timeout = time.Nanosecond
		_, err := executor.Execute(context.Background(), c)
		var ge *domain.GitError
		if !errors.As(err, &ge) || ge.Code != domain.OperationTimedOut {
			t.Fatal(err)
		}
	})
	t.Run("disconnect", func(t *testing.T) {
		_ = server.Process.Kill()
		err := <-done
		done <- err
		_, err = executor.Execute(context.Background(), remoteCommand(repo))
		var ge *domain.GitError
		if !errors.As(err, &ge) || ge.Code != domain.RemoteUnavailable {
			t.Fatal(err)
		}
	})
}
