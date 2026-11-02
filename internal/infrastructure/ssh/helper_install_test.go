package ssh

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pkg/sftp"

	"gitviewer/internal/domain"
)

type fakeInfo struct {
	name    string
	mode    os.FileMode
	size    int64
	modTime time.Time
}

func (i fakeInfo) Name() string       { return i.name }
func (i fakeInfo) Size() int64        { return i.size }
func (i fakeInfo) Mode() os.FileMode  { return i.mode }
func (i fakeInfo) ModTime() time.Time { return i.modTime }
func (i fakeInfo) IsDir() bool        { return i.mode.IsDir() }
func (i fakeInfo) Sys() any           { return nil }

type fakeOps struct {
	mu        sync.Mutex
	home      string
	files     map[string][]byte
	modes     map[string]os.FileMode
	mods      map[string]time.Time
	symlinks  map[string]bool
	unameS    string
	unameM    string
	runErr    error
	mkdirErr  error
	writeErr  error
	renameErr error
	writes    int
	closed    bool
}

func newFakeOps() *fakeOps {
	return &fakeOps{
		home:     "/home/user",
		files:    map[string][]byte{},
		modes:    map[string]os.FileMode{},
		mods:     map[string]time.Time{},
		symlinks: map[string]bool{},
		unameS:   "Linux\n",
		unameM:   "x86_64\n",
	}
}

func (f *fakeOps) Home(context.Context) (string, error) { return f.home, nil }

func (f *fakeOps) Lstat(_ context.Context, p string) (os.FileInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.symlinks[p] {
		return fakeInfo{name: path.Base(p), mode: os.ModeSymlink | 0777, modTime: f.mods[p]}, nil
	}
	if data, ok := f.files[p]; ok {
		return fakeInfo{name: path.Base(p), mode: f.modes[p], size: int64(len(data)), modTime: f.mods[p]}, nil
	}
	if f.isDir(p) {
		return fakeInfo{name: path.Base(p), mode: os.ModeDir | 0700, modTime: f.mods[p]}, nil
	}
	return nil, os.ErrNotExist
}

func (f *fakeOps) isDir(p string) bool {
	if p == "/" {
		return true
	}
	for file := range f.files {
		if strings.HasPrefix(file, p+"/") {
			return true
		}
	}
	for link := range f.symlinks {
		if strings.HasPrefix(link, p+"/") {
			return true
		}
	}
	for dir := range f.mods {
		if dir == p || strings.HasPrefix(dir, p+"/") && f.modes[dir].IsDir() {
			return true
		}
	}
	return false
}

func (f *fakeOps) ReadDir(_ context.Context, p string) ([]os.FileInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	seen := map[string]os.FileInfo{}
	for file, data := range f.files {
		if path.Dir(file) == p {
			seen[path.Base(file)] = fakeInfo{name: path.Base(file), mode: f.modes[file], size: int64(len(data)), modTime: f.mods[file]}
		}
	}
	for link := range f.symlinks {
		if path.Dir(link) == p {
			seen[path.Base(link)] = fakeInfo{name: path.Base(link), mode: os.ModeSymlink, modTime: f.mods[link]}
		}
	}
	for dir, mode := range f.modes {
		if path.Dir(dir) == p && mode.IsDir() {
			seen[path.Base(dir)] = fakeInfo{name: path.Base(dir), mode: os.ModeDir | 0700, modTime: f.mods[dir]}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]os.FileInfo, 0, len(names))
	for _, name := range names {
		result = append(result, seen[name])
	}
	return result, nil
}

func (f *fakeOps) Mkdir(_ context.Context, p string, mode os.FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.mkdirErr != nil {
		return f.mkdirErr
	}
	if f.isDir(p) {
		return fmt.Errorf("exists")
	}
	f.modes[p] = os.ModeDir | mode
	return nil
}

func (f *fakeOps) OpenRead(_ context.Context, p string) (io.ReadCloser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.files[p]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

type fakeWriter struct {
	ops  *fakeOps
	path string
	buf  bytes.Buffer
}

func (w *fakeWriter) Write(p []byte) (int, error) { return w.buf.Write(p) }
func (w *fakeWriter) Close() error {
	w.ops.mu.Lock()
	defer w.ops.mu.Unlock()
	if w.ops.writeErr != nil {
		return w.ops.writeErr
	}
	w.ops.files[w.path] = append([]byte(nil), w.buf.Bytes()...)
	w.ops.modes[w.path] = 0600
	w.ops.writes++
	return nil
}

func (f *fakeOps) OpenWrite(_ context.Context, p string) (io.WriteCloser, error) {
	if _, err := f.Lstat(context.Background(), p); err == nil {
		return nil, os.ErrExist
	}
	return &fakeWriter{ops: f, path: p}, nil
}

func (f *fakeOps) ReadFile(_ context.Context, p string, limit int64) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.files[p]
	if !ok {
		return nil, os.ErrNotExist
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("too large")
	}
	return append([]byte(nil), data...), nil
}

func (f *fakeOps) Chmod(_ context.Context, p string, mode os.FileMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.modes[p] = f.modes[p].Type() | mode
	return nil
}

func (f *fakeOps) Rename(_ context.Context, oldPath, newPath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.renameErr != nil {
		return f.renameErr
	}
	data, ok := f.files[oldPath]
	if !ok {
		return os.ErrNotExist
	}
	delete(f.files, oldPath)
	f.files[newPath] = data
	f.modes[newPath] = f.modes[oldPath]
	return nil
}

func (f *fakeOps) Remove(_ context.Context, p string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.files, p)
	delete(f.modes, p)
	return nil
}

func (f *fakeOps) RemoveAll(_ context.Context, p string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for file := range f.files {
		if strings.HasPrefix(file, p+"/") {
			delete(f.files, file)
			delete(f.modes, file)
		}
	}
	delete(f.modes, p)
	return nil
}

func (f *fakeOps) Run(_ context.Context, argv []string, _ time.Duration) (domain.GitResult, error) {
	if f.runErr != nil {
		return domain.GitResult{}, f.runErr
	}
	switch argv[0] {
	case "uname":
		if len(argv) > 1 && argv[1] == "-m" {
			return domain.GitResult{Stdout: []byte(f.unameM)}, nil
		}
		return domain.GitResult{Stdout: []byte(f.unameS)}, nil
	}
	return domain.GitResult{}, fmt.Errorf("unexpected command")
}

func (f *fakeOps) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func writeLocalHelper(t *testing.T, data string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "gitviewer-helper")
	if err := os.WriteFile(p, []byte(data), 0700); err != nil {
		t.Fatal(err)
	}
	return p
}

func installerWith(t *testing.T, ops *fakeOps, local string) *HelperInstaller {
	t.Helper()
	return &HelperInstaller{
		Ops: ops,
		LocalBinary: func(goos, goarch string) (string, error) {
			if goos != "linux" || goarch != "amd64" {
				t.Fatalf("unexpected target %s/%s", goos, goarch)
			}
			return local, nil
		},
	}
}

func TestHelperInstallUploadsOnce(t *testing.T) {
	ops := newFakeOps()
	local := writeLocalHelper(t, "binary")
	installer := installerWith(t, ops, local)
	first, err := installer.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.Platform != "linux/amd64" || len(first.BuildID) != 12 || !strings.HasPrefix(first.Path, "/home/user/.local/share/gitviewer/helper/v1/") {
		t.Fatalf("unexpected install: %+v", first)
	}
	if string(ops.files[first.Path]) != "binary" || ops.modes[first.Path] != 0700 {
		t.Fatalf("binary not installed correctly: %q %o", ops.files[first.Path], ops.modes[first.Path])
	}
	sum := sha256.Sum256([]byte("binary"))
	if strings.TrimSpace(string(ops.files[first.Path+".sha256"])) != hex.EncodeToString(sum[:]) {
		t.Fatal("sidecar hash mismatch")
	}
	writes := ops.writes
	second, err := installer.Ensure(context.Background())
	if err != nil || second.Path != first.Path {
		t.Fatalf("second ensure: %+v %v", second, err)
	}
	if ops.writes != writes {
		t.Fatalf("re-uploaded an intact helper: %d -> %d", writes, ops.writes)
	}
}

func TestHelperInstallRepairsBadHashAndCleansOldBuilds(t *testing.T) {
	ops := newFakeOps()
	local := writeLocalHelper(t, "binary")
	installer := installerWith(t, ops, local)
	first, err := installer.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ops.files[first.Path+".sha256"] = []byte("wrong\n")
	ops.writes = 0
	if _, err := installer.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ops.writes == 0 {
		t.Fatal("bad sidecar did not trigger a re-upload")
	}
	base := path.Dir(path.Dir(first.Path))
	oldDir := path.Join(base, "000000000000")
	ops.modes[oldDir] = os.ModeDir | 0700
	ops.mods[oldDir] = time.Now().Add(-8 * 24 * time.Hour)
	ops.files[path.Join(oldDir, "gitviewer-helper-linux-amd64")] = []byte("old")
	recentDir := path.Join(base, "abcdef012345")
	ops.modes[recentDir] = os.ModeDir | 0700
	ops.mods[recentDir] = time.Now()
	tempFile := path.Join(recentDir, "gitviewer-helper-linux-amd64.tmp.0123456789abcdef")
	ops.files[tempFile] = []byte("junk")
	ops.mods[tempFile] = time.Now().Add(-2 * time.Hour)
	installer.Now = time.Now
	if _, err := installer.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := ops.files[path.Join(oldDir, "gitviewer-helper-linux-amd64")]; !ok {
		t.Fatal("Ensure performed cleanup on the install path")
	}
	installer.cleanup(context.Background(), base, first.BuildID)
	if _, ok := ops.files[path.Join(oldDir, "gitviewer-helper-linux-amd64")]; ok {
		t.Fatal("old build directory was not removed")
	}
	if _, ok := ops.files[tempFile]; ok {
		t.Fatal("stale temp file was not removed")
	}
	if _, ok := ops.files[first.Path]; !ok {
		t.Fatal("current build was removed")
	}
}

func TestHelperInstallRejectsSymlinkAndUnsupported(t *testing.T) {
	ops := newFakeOps()
	local := writeLocalHelper(t, "binary")
	installer := installerWith(t, ops, local)
	ops.symlinks["/home/user/.local"] = true
	if _, err := installer.Ensure(context.Background()); !errors.Is(err, ErrHelperPermanent) {
		t.Fatalf("symlink path was not permanent: %v", err)
	}
	ops = newFakeOps()
	ops.unameS = "FreeBSD\n"
	installer = installerWith(t, ops, local)
	if _, err := installer.Ensure(context.Background()); !errors.Is(err, ErrHelperPermanent) {
		t.Fatalf("unsupported platform was not permanent: %v", err)
	}
	ops = newFakeOps()
	ops.unameM = "riscv64\n"
	installer = installerWith(t, ops, local)
	if _, err := installer.Ensure(context.Background()); !errors.Is(err, ErrHelperPermanent) {
		t.Fatalf("unsupported architecture was not permanent: %v", err)
	}
}

func TestHelperInstallClassifiesFailures(t *testing.T) {
	local := writeLocalHelper(t, "binary")
	ops := newFakeOps()
	ops.mkdirErr = &domain.GitError{Code: domain.PermissionDenied}
	if _, err := installerWith(t, ops, local).Ensure(context.Background()); !errors.Is(err, ErrHelperPermanent) {
		t.Fatalf("permission failure was not permanent: %v", err)
	}
	ops = newFakeOps()
	ops.runErr = &domain.GitError{Code: domain.RemoteUnavailable}
	if _, err := installerWith(t, ops, local).Ensure(context.Background()); err == nil || errors.Is(err, ErrHelperPermanent) {
		t.Fatalf("network failure should be temporary: %v", err)
	}
	ops = newFakeOps()
	ops.renameErr = fmt.Errorf("rename failed")
	if _, err := installerWith(t, ops, local).Ensure(context.Background()); !errors.Is(err, ErrHelperPermanent) {
		t.Fatalf("rename failure was not permanent: %v", err)
	}
}

func TestHelperInstallUsesExplicitBaseWithoutHome(t *testing.T) {
	ops := newFakeOps()
	ops.home = ""
	local := writeLocalHelper(t, "binary")
	installer := installerWith(t, ops, local)
	installer.RemoteBase = "/srv/helpers"
	installed, err := installer.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(installed.Path, "/srv/helpers/") {
		t.Fatalf("unexpected path: %s", installed.Path)
	}
}

func TestHelperInstallHashesBinaryAndRejectsSidecarSymlink(t *testing.T) {
	ops := newFakeOps()
	installer := installerWith(t, ops, writeLocalHelper(t, "binary"))
	installed, err := installer.Ensure(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ops.files[installed.Path] = []byte("tamper") // same size, correct sidecar
	if _, err := installer.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if string(ops.files[installed.Path]) != "binary" {
		t.Fatal("trusted sidecar for a corrupted binary")
	}
	ops.symlinks[installed.Path+".sha256"] = true
	writes := ops.writes
	if _, err := installer.Ensure(context.Background()); !errors.Is(err, ErrHelperPermanent) {
		t.Fatalf("sidecar link accepted: %v", err)
	}
	if ops.writes != writes {
		t.Fatal("wrote through a rejected sidecar")
	}
}

type corruptHelperUploadOps struct{ *fakeOps }

func (o corruptHelperUploadOps) Chmod(ctx context.Context, name string, mode os.FileMode) error {
	if strings.Contains(name, ".tmp.") {
		o.files[name] = bytes.Repeat([]byte{'x'}, len(o.files[name]))
	}
	return o.fakeOps.Chmod(ctx, name, mode)
}

func TestHelperInstallRejectsCorruptedStagedUpload(t *testing.T) {
	ops := corruptHelperUploadOps{newFakeOps()}
	data := []byte("binary")
	sum := sha256.Sum256(data)
	_, err := ensureRemoteFile(context.Background(), ops, "/helpers/0123456789ab", "gitviewer-helper-linux-amd64", data, hex.EncodeToString(sum[:]))
	if err == nil {
		t.Fatal("accepted corrupted upload")
	}
	if len(ops.files) != 0 {
		t.Fatalf("corrupted upload was published or left behind: %v", ops.files)
	}
}

func TestHelperInstallExclusiveWritePreservesCollision(t *testing.T) {
	ops := newFakeOps()
	ops.files["/tmp/collision"] = []byte("existing")
	if err := writeRemoteFile(context.Background(), ops, "/tmp/collision", []byte("new"), 0700); !errors.Is(err, os.ErrExist) {
		t.Fatalf("collision not refused: %v", err)
	}
	if string(ops.files["/tmp/collision"]) != "existing" {
		t.Fatal("collision was truncated or removed")
	}
}

func TestHelperInstallCleanupRestrictedToBuildsAndOldTemps(t *testing.T) {
	ops := newFakeOps()
	now := time.Now()
	base := "/helpers"
	current := "111111111111"
	for _, name := range []string{current, "222222222222", "333333333333", "unrelated", "ABCDEF012345"} {
		dir := path.Join(base, name)
		ops.modes[dir] = os.ModeDir | 0700
		ops.mods[dir] = now.Add(-8 * 24 * time.Hour)
		ops.files[path.Join(dir, "keep")] = []byte("keep")
	}
	ops.mods[path.Join(base, "333333333333")] = now
	ops.symlinks[path.Join(base, "444444444444")] = true
	oldTemp := "gitviewer-helper-linux-arm64.sha256.tmp.0123456789abcdef"
	for _, name := range []string{oldTemp, "gitviewer-helper-linux-arm64.tmp.0123456789abcdef", "junk.tmp.0123456789abcdef", "gitviewer-helper-linux-arm64.tmp.xyz"} {
		file := path.Join(base, "333333333333", name)
		ops.files[file] = []byte("temp")
		ops.mods[file] = now.Add(-2 * time.Hour)
	}
	fresh := path.Join(base, "333333333333", "gitviewer-helper-linux-arm64.tmp.0123456789abcdef")
	ops.mods[fresh] = now.Add(-30 * time.Minute)
	currentTemp := path.Join(base, current, oldTemp)
	ops.files[currentTemp] = []byte("keep")
	ops.mods[currentTemp] = now.Add(-2 * time.Hour)
	installer := &HelperInstaller{Ops: ops, Now: func() time.Time { return now }}
	installer.cleanup(context.Background(), base, current)
	for _, file := range []string{path.Join(base, current, "keep"), currentTemp, path.Join(base, "unrelated", "keep"), path.Join(base, "ABCDEF012345", "keep"), fresh, path.Join(base, "333333333333", "junk.tmp.0123456789abcdef"), path.Join(base, "333333333333", "gitviewer-helper-linux-arm64.tmp.xyz")} {
		if _, ok := ops.files[file]; !ok {
			t.Errorf("removed protected file %q", file)
		}
	}
	for _, file := range []string{path.Join(base, "222222222222", "keep"), path.Join(base, "333333333333", oldTemp)} {
		if _, ok := ops.files[file]; ok {
			t.Errorf("did not remove stale file %q", file)
		}
	}
}

func newInProcessHelperSFTP(t *testing.T) *sftpHelperOps {
	t.Helper()
	local, remote := net.Pipe()
	server, err := sftp.NewServer(remote)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { defer close(done); _ = server.Serve(); _ = server.Close() }()
	client, err := sftp.NewClientPipe(local, local)
	if err != nil {
		t.Fatal(err)
	}
	ops := &sftpHelperOps{client: client, stdin: local, stdout: local}
	t.Cleanup(func() { _ = ops.Close(); <-done })
	return ops
}

func TestHelperInstallSFTPExclusiveUploadAndHash(t *testing.T) {
	ops := newInProcessHelperSFTP(t)
	dir := filepath.ToSlash(t.TempDir())
	data := []byte("fixture binary")
	sum := sha256.Sum256(data)
	name, err := ensureRemoteFile(context.Background(), ops, dir, "gitviewer-helper-linux-amd64", data, hex.EncodeToString(sum[:]))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ops.OpenWrite(context.Background(), name); err == nil {
		t.Fatal("exclusive create overwrote an existing file")
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(name, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ops.OpenWrite(context.Background(), link); err == nil {
		t.Fatal("exclusive create followed a symlink")
	}
	if err := os.WriteFile(name, []byte("broken binary!"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureRemoteFile(context.Background(), ops, dir, "gitviewer-helper-linux-amd64", data, hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(name)
	if err != nil || !bytes.Equal(actual, data) {
		t.Fatalf("upload verification failed: %q %v", actual, err)
	}
}

func TestHelperInstallSFTPCancellationUnblocksRequestAndClose(t *testing.T) {
	local, remote := net.Pipe()
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		defer remote.Close()
		// Read INIT then reply VERSION 3, and stop serving requests.
		init := make([]byte, 9)
		if _, err := io.ReadFull(remote, init); err != nil {
			return
		}
		if _, err := remote.Write([]byte{0, 0, 0, 5, 2, 0, 0, 0, 3}); err != nil {
			return
		}
		_, _ = io.Copy(io.Discard, remote)
	}()
	client, err := sftp.NewClientPipe(local, local)
	if err != nil {
		t.Fatal(err)
	}
	ops := &sftpHelperOps{client: client, stdin: local, stdout: local}
	t.Cleanup(func() { _ = ops.Close(); <-serverDone })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = ops.Lstat(ctx, "/blocked")
	var ge *domain.GitError
	if !errors.As(err, &ge) || ge.Code != domain.OperationTimedOut {
		t.Fatalf("unexpected cancellation: %v", err)
	}
	_ = ops.Close()
	if time.Since(started) > time.Second {
		t.Fatal("request/close did not unblock")
	}
}

func TestHelperInstallSFTPClassification(t *testing.T) {
	for _, test := range []struct {
		name      string
		err       error
		stderr    string
		permanent bool
		code      domain.ErrorCode
	}{
		{name: "unsupported", err: &sftp.StatusError{Code: 8}, permanent: true},
		{name: "readonly", err: &sftp.StatusError{Code: 4}, permanent: true},
		{name: "filesystem permission", err: &sftp.StatusError{Code: 3}, code: domain.PermissionDenied},
		{name: "connection", err: &sftp.StatusError{Code: 7}, code: domain.RemoteUnavailable},
		{name: "subsystem", err: io.EOF, stderr: "subsystem request failed on channel 0", permanent: true},
		{name: "auth", err: io.EOF, stderr: "Permission denied (publickey)", code: domain.SshAuthFailed},
		{name: "host key", err: io.EOF, stderr: "Host key verification failed", code: domain.HostKeyRejected},
	} {
		t.Run(test.name, func(t *testing.T) {
			o := &sftpHelperOps{stderr: &connectionOutput{}}
			_, _ = o.stderr.Write([]byte(test.stderr))
			err := o.wrap(test.err)
			if errors.Is(err, ErrHelperPermanent) != test.permanent {
				t.Fatalf("permanence: %v", err)
			}
			if test.code != "" {
				var ge *domain.GitError
				if !errors.As(err, &ge) || ge.Code != test.code {
					t.Fatalf("classification: %v", err)
				}
			}
		})
	}
	for _, err := range []error{context.Canceled, context.DeadlineExceeded, io.EOF, &net.DNSError{IsTimeout: true}} {
		if errors.Is(installError(err), ErrHelperPermanent) {
			t.Errorf("transient failure marked permanent: %v", err)
		}
	}
}

func TestHelperLocalPath(t *testing.T) {
	resources := t.TempDir()
	target := filepath.Join(resources, "helper", "darwin-arm64", "gitviewer-helper")
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("bin"), 0700); err != nil {
		t.Fatal(err)
	}
	previous := HelperResourcesDir
	HelperResourcesDir = func() (string, error) { return resources, nil }
	t.Cleanup(func() { HelperResourcesDir = previous })
	t.Setenv("GITVIEWER_HELPER_PATH", "")
	got, err := HelperLocalPath("darwin", "arm64")
	if err != nil || got != target {
		t.Fatalf("resolved %q %v", got, err)
	}
	if _, err := HelperLocalPath("linux", "arm64"); err == nil {
		t.Fatal("missing helper was accepted")
	}
	if _, err := HelperLocalPath("windows", "amd64"); err == nil {
		t.Fatal("unsupported target was accepted")
	}
	t.Setenv("GITVIEWER_HELPER_PATH", filepath.Join(resources, "missing"))
	if _, err := HelperLocalPath("darwin", "arm64"); err == nil {
		t.Fatal("explicit missing path was accepted")
	}
}
