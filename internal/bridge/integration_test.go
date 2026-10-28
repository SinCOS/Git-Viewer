package bridge

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func bridgeKeygen(t *testing.T, path string) {
	t.Helper()
	out, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", path).CombinedOutput()
	if err != nil {
		t.Fatalf("keygen: %v %s", err, out)
	}
}

func bridgeWrite(t *testing.T, path, value string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestReverseUnixBridgeTokenAndEndpoint(t *testing.T) {
	if _, err := os.Stat("/usr/sbin/sshd"); err != nil {
		t.Skip("sshd unavailable")
	}
	dir := t.TempDir()
	bridgeKeygen(t, filepath.Join(dir, "host"))
	bridgeKeygen(t, filepath.Join(dir, "identity"))
	public, err := os.ReadFile(filepath.Join(dir, "identity.pub"))
	if err != nil {
		t.Fatal(err)
	}
	bridgeWrite(t, filepath.Join(dir, "authorized_keys"), string(public))
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
	serverConfig := fmt.Sprintf(
		"Port %d\nListenAddress 127.0.0.1\nHostKey %s\nPidFile %s\nAuthorizedKeysFile %s\n"+
			"StrictModes no\nPasswordAuthentication no\nKbdInteractiveAuthentication no\nUsePAM no\n"+
			"AllowStreamLocalForwarding yes\nLogLevel ERROR\n",
		port, filepath.Join(dir, "host"), filepath.Join(dir, "pid"), filepath.Join(dir, "authorized_keys"),
	)
	bridgeWrite(t, filepath.Join(dir, "sshd_config"), serverConfig)
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
	bridgeWrite(t, known, fmt.Sprintf("[127.0.0.1]:%d %s", port, hostKey))
	configPath := filepath.Join(dir, "ssh_config")
	bridgeWrite(t, configPath, fmt.Sprintf(
		"Host fixture\n HostName 127.0.0.1\n Port %d\n User %s\n IdentityFile %s\n IdentitiesOnly yes\n GlobalKnownHostsFile /dev/null\n",
		port, current.Username, filepath.Join(dir, "identity"),
	))
	remoteDir := filepath.Join(dir, "remote-config")
	received := make(chan OpenRequest, 1)
	session, err := Start(context.Background(), Config{
		ConfigPath: configPath, KnownHostsFile: known, RemoteConfigDir: remoteDir,
	}, "fixture", func(_ context.Context, request OpenRequest) (<-chan struct{}, error) {
		received <- request
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	endpoint, err := os.ReadFile(filepath.Join(remoteDir, "endpoint"))
	if err != nil {
		t.Fatal(err)
	}
	var socket, token, host string
	for _, line := range strings.Split(string(endpoint), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "socket":
			socket = value
		case "token":
			token = value
		case "host":
			host = value
		}
	}
	if socket == "" || token == "" || host != "fixture" {
		t.Fatalf("endpoint = %q", endpoint)
	}
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		},
	}}
	send := func(body string) int {
		request, err := http.NewRequest(http.MethodPost, "http://localhost/v1/open", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/x-gitviewer-argv")
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		_, _ = io.Copy(io.Discard, response.Body)
		return response.StatusCode
	}
	if code := send(strings.Join([]string{"wrong", "fixture", "/tmp", "."}, "\x00") + "\x00"); code != http.StatusForbidden {
		t.Fatalf("wrong token status = %d", code)
	}
	if code := send(strings.Join([]string{token, "fixture", "/tmp", ".", "--new-window"}, "\x00") + "\x00"); code != http.StatusOK {
		t.Fatalf("valid token status = %d", code)
	}
	select {
	case request := <-received:
		if request.Host != "fixture" || len(request.Argv) != 2 || request.Argv[1] != "--new-window" {
			t.Fatalf("request = %+v", request)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not receive request")
	}
}
