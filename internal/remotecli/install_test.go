package remotecli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gitviewer/internal/deeplink"
	"gitviewer/internal/infrastructure/ssh"
)

func TestWrapperDeepLinkFallback(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "gitviewer")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "install"), []byte("host=dev-box\noriginal=\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "code")
	if err := os.WriteFile(path, []byte(wrapper), 0o700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("/bin/sh", path, "--new-window", "--goto", "/tmp/中文 file.go:12:4")
	command.Env = append(os.Environ(), "HOME="+home)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatal(err, string(output))
	}
	line := strings.TrimSpace(string(output))
	if i := strings.LastIndex(line, "gitviewer://"); i >= 0 {
		line = line[i:]
	}
	if i := strings.IndexByte(line, '\x1b'); i >= 0 {
		line = line[:i]
	}
	request, err := deeplink.Parse(line)
	if err != nil {
		t.Fatalf("deep link %q: %v", line, err)
	}
	if request.Host != "dev-box" || request.Path != "/tmp/中文 file.go" || !request.NewWindow || request.Line != 12 || request.Column != 4 {
		t.Fatalf("request = %+v", request)
	}
}

func TestWrapperDelegatesOnlyThroughBackup(t *testing.T) {
	home := t.TempDir()
	configDir := filepath.Join(home, ".config", "gitviewer")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "code-backup")
	if err := os.WriteFile(backup, []byte("#!/bin/sh\necho delegated \"$@\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	state := "host=dev\noriginal=/usr/bin/code\ndelegate=" + backup + "\n"
	if err := os.WriteFile(filepath.Join(configDir, "install"), []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "code")
	if err := os.WriteFile(path, []byte(wrapper), 0o700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("/bin/sh", path, "--install-extension", "publisher.extension")
	command.Env = append(os.Environ(), "HOME="+home)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatal(err, string(output))
	}
	if strings.TrimSpace(string(output)) != "delegated --install-extension publisher.extension" {
		t.Fatalf("output = %q", output)
	}
}

func TestInstallScriptRecordsHostAndBackup(t *testing.T) {
	value := script("dev-box")
	for _, part := range []string{"GITVIEWER_REMOTE_CLI_VERSION", "code-backups", "host=dev-box", "delegate=$backup", "delegate=$found", "ln -sf"} {
		if !strings.Contains(value, part) {
			t.Fatalf("install script misses %q", part)
		}
	}
}

func TestWrapperFallbackMakesRelativeTargetsAbsolute(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	configDir := filepath.Join(home, ".config", "gitviewer")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "install"), []byte("host=dev-box\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "code")
	if err := os.WriteFile(path, []byte(wrapper), 0o700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("/bin/sh", path, "src/main.go")
	command.Dir = work
	command.Env = append(os.Environ(), "HOME="+home)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatal(err, string(output))
	}
	line := strings.TrimSpace(string(output))
	if i := strings.LastIndex(line, "gitviewer://"); i >= 0 {
		line = line[i:]
	}
	if i := strings.IndexByte(line, '\x1b'); i >= 0 {
		line = line[:i]
	}
	request, err := deeplink.Parse(line)
	if err != nil {
		t.Fatalf("deep link %q: %v", line, err)
	}
	want, err := filepath.EvalSymlinks(work)
	if err != nil {
		t.Fatal(err)
	}
	if request.Path != filepath.Join(want, "src/main.go") {
		t.Fatalf("path = %q", request.Path)
	}
}

func TestInspectReadsRemoteInstallRecord(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	configDir := filepath.Join(home, ".config", "gitviewer")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	state := "original=/usr/bin/code\nbackup=/home/dev/.local/share/code\n delegate=bad\ntarget=/home/dev/.local/bin/code\nhost=dev-box\nversion=1\n"
	if err := os.WriteFile(filepath.Join(configDir, "install"), []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}
	fakeSSH := filepath.Join(t.TempDir(), "ssh")
	if err := os.WriteFile(fakeSSH, []byte("#!/bin/sh\nfor last do :; done\nexec /bin/sh -c \"$last\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	status, err := Inspect(context.Background(), ssh.Executor{SSHPath: fakeSSH}, "dev-box")
	if err != nil {
		t.Fatal(err)
	}
	if !status.Installed || status.Host != "dev-box" || status.Version != 1 || status.Delegate != "" || status.Target != "/home/dev/.local/bin/code" {
		t.Fatalf("status = %+v", status)
	}
	if err := os.Remove(filepath.Join(configDir, "install")); err != nil {
		t.Fatal(err)
	}
	status, err = Inspect(context.Background(), ssh.Executor{SSHPath: fakeSSH}, "dev-box")
	if err != nil || status.Installed {
		t.Fatalf("missing status = %+v, %v", status, err)
	}
}

func TestInstallScriptBacksUpWritableCodeAndDelegates(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	codePath := filepath.Join(bin, "code")
	if err := os.WriteFile(codePath, []byte("#!/bin/sh\necho original \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	runInstallScript(t, home, bin)
	data, err := os.ReadFile(codePath)
	if err != nil || !strings.Contains(string(data), "GITVIEWER_REMOTE_CLI_VERSION") {
		t.Fatalf("code was not replaced: %v %s", err, data)
	}
	state := readState(t, home)
	if state.Original != codePath || state.Target != codePath || state.Backup == "" || state.Delegate != state.Backup {
		t.Fatalf("state = %+v", state)
	}
	if _, err := os.Stat(state.Backup); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(codePath, "--install-extension", "publisher.extension")
	command.Env = append(os.Environ(), "HOME="+home, "PATH="+bin+":/usr/bin:/bin")
	output, err := command.CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != "original --install-extension publisher.extension" {
		t.Fatalf("delegation = %q, %v", output, err)
	}
}

func TestInstallScriptFallsBackForReadOnlyCode(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	codePath := filepath.Join(bin, "code")
	if err := os.WriteFile(codePath, []byte("#!/bin/sh\necho original \"$@\"\n"), 0o555); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(bin, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(bin, 0o755) })
	runInstallScript(t, home, bin)
	target := filepath.Join(home, ".local", "bin", "code")
	if _, err := os.Stat(target); err != nil {
		t.Fatal(err)
	}
	state := readState(t, home)
	if state.Original != codePath || state.Delegate != codePath || state.Target != target {
		t.Fatalf("state = %+v", state)
	}
}

func runInstallScript(t *testing.T, home, bin string) {
	t.Helper()
	command := exec.Command("/bin/sh", "-c", script("dev-box"))
	command.Env = append(os.Environ(), "HOME="+home, "PATH="+bin+":/usr/bin:/bin")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("install: %v %s", err, output)
	}
}

func readState(t *testing.T, home string) InstallStatus {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".config", "gitviewer", "install"))
	if err != nil {
		t.Fatal(err)
	}
	status := InstallStatus{Installed: true}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "original":
			status.Original = value
		case "backup":
			status.Backup = value
		case "delegate":
			status.Delegate = value
		case "target":
			status.Target = value
		case "host":
			status.Host = value
		}
	}
	return status
}
