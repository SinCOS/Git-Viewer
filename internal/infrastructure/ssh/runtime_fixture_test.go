package ssh

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestRuntimeRemoteHelper runs only as the forced command of this fixture's
// authenticated SSH session. No product command or user repository is changed.
func TestRuntimeRemoteHelper(t *testing.T) {
	if os.Getenv("GITVIEWER_RUNTIME_HELPER") != "1" {
		return
	}
	marker := os.Getenv("GITVIEWER_RUNTIME_MARKER")
	if err := os.WriteFile(marker, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		os.Exit(2)
	}
	fmt.Fprint(os.Stdout, "remote-task-started\x00")
	time.Sleep(30 * time.Second)
	os.Exit(0)
}

type runtimeProxy struct {
	listener    net.Listener
	target      string
	mu          sync.Mutex
	connections []net.Conn
	acceptDone  chan struct{}
	workers     sync.WaitGroup
}

func startRuntimeProxy(t *testing.T, target string) *runtimeProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	p := &runtimeProxy{listener: listener, target: target, acceptDone: make(chan struct{})}
	go func() {
		defer close(p.acceptDone)
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			upstream, err := net.DialTimeout("tcp", target, time.Second)
			if err != nil {
				client.Close()
				continue
			}
			p.mu.Lock()
			p.connections = append(p.connections, client, upstream)
			p.mu.Unlock()
			p.workers.Add(2)
			go func() {
				defer p.workers.Done()
				defer client.Close()
				defer upstream.Close()
				_, _ = io.Copy(upstream, client)
			}()
			go func() {
				defer p.workers.Done()
				defer client.Close()
				defer upstream.Close()
				_, _ = io.Copy(client, upstream)
			}()
		}
	}()
	t.Cleanup(func() { listener.Close(); <-p.acceptDone; p.disconnect(); p.workers.Wait() })
	return p
}
func (p *runtimeProxy) disconnect() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, conn := range p.connections {
		conn.Close()
	}
}

type runtimeFixture struct {
	executor  Executor
	marker    string
	proxy     *runtimeProxy
	remotePID int
}

func newRuntimeFixture(t *testing.T) *runtimeFixture {
	t.Helper()
	if _, err := os.Stat("/usr/sbin/sshd"); err != nil {
		t.Skip("sshd unavailable")
	}
	dir := t.TempDir()
	keygen(t, filepath.Join(dir, "host"))
	keygen(t, filepath.Join(dir, "identity"))
	pub, err := os.ReadFile(filepath.Join(dir, "identity.pub"))
	if err != nil {
		t.Fatal(err)
	}
	writeRuntimeFile(t, filepath.Join(dir, "authorized_keys"), string(pub))
	reserve, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serverAddress := reserve.Addr().String()
	port := reserve.Addr().(*net.TCPAddr).Port
	reserve.Close()
	fixture := &runtimeFixture{marker: filepath.Join(dir, "started")}
	helper, err := Encode([]string{"env", "GITVIEWER_RUNTIME_HELPER=1", "GITVIEWER_RUNTIME_MARKER=" + fixture.marker, os.Args[0], "-test.run=^TestRuntimeRemoteHelper$"})
	if err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf("Port %d\nListenAddress 127.0.0.1\nHostKey %s\nPidFile %s\nAuthorizedKeysFile %s\nStrictModes no\nPasswordAuthentication no\nKbdInteractiveAuthentication no\nUsePAM no\nLogLevel ERROR\nForceCommand %s\n", port, filepath.Join(dir, "host"), filepath.Join(dir, "sshd.pid"), filepath.Join(dir, "authorized_keys"), helper)
	serverConfig := filepath.Join(dir, "sshd_config")
	writeRuntimeFile(t, serverConfig, config)
	var log bytes.Buffer
	server := exec.Command("/usr/sbin/sshd", "-D", "-e", "-f", serverConfig)
	server.Stdout = &log
	server.Stderr = &log
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	stopped := make(chan error, 1)
	go func() { stopped <- server.Wait() }()
	t.Cleanup(func() {
		// Cancellation of the local ssh process is not proof of remote termination.
		// Explicitly clean up our known fixture process, independently of assertions.
		if fixture.remotePID != 0 {
			if p, err := os.FindProcess(fixture.remotePID); err == nil {
				_ = p.Kill()
			}
			gone := false
			for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
				if err := syscall.Kill(fixture.remotePID, 0); err == syscall.ESRCH {
					gone = true
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if !gone {
				t.Error("remote fixture helper was not reaped")
			}
		}
		_ = server.Process.Kill()
		select {
		case <-stopped:
		case <-time.After(3 * time.Second):
			t.Error("sshd did not stop")
		}
	})
	ready := false
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		select {
		case err := <-stopped:
			stopped <- err
			t.Fatalf("sshd exited: %v %s", err, log.String())
		default:
		}
		conn, err := net.DialTimeout("tcp", serverAddress, 50*time.Millisecond)
		if err == nil {
			conn.Close()
			ready = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !ready {
		t.Fatal("sshd not ready")
	}
	fixture.proxy = startRuntimeProxy(t, serverAddress)
	proxyPort := fixture.proxy.listener.Addr().(*net.TCPAddr).Port
	current, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	clientConfig := filepath.Join(dir, "ssh_config")
	writeRuntimeFile(t, clientConfig, fmt.Sprintf("Host fixture\n HostName 127.0.0.1\n Port %d\n User %s\n IdentityFile %s\n IdentitiesOnly yes\n GlobalKnownHostsFile /dev/null\n", proxyPort, current.Username, filepath.Join(dir, "identity")))
	hostKey, err := os.ReadFile(filepath.Join(dir, "host.pub"))
	if err != nil {
		t.Fatal(err)
	}
	known := filepath.Join(dir, "known_hosts")
	writeRuntimeFile(t, known, fmt.Sprintf("[127.0.0.1]:%d %s", proxyPort, hostKey))
	fixture.executor = Executor{ConfigPath: clientConfig, KnownHostsFile: known}
	return fixture
}
func writeRuntimeFile(t *testing.T, path, value string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
}
func (f *runtimeFixture) waitStarted(t *testing.T) {
	t.Helper()
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		data, err := os.ReadFile(f.marker)
		if err == nil && len(data) > 0 {
			pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
			if err != nil {
				t.Fatal(err)
			}
			f.remotePID = pid
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("authenticated remote task never reached execution")
}
