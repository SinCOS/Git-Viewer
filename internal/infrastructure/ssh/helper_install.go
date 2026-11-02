package ssh

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"gitviewer/internal/domain"
)

const (
	helperRemoteVersion = "v1"
	helperBinaryName    = "gitviewer-helper"
	helperDirMode       = os.FileMode(0700)
	helperBinaryMode    = os.FileMode(0700)
	helperSidecarMode   = os.FileMode(0600)
	helperOldBuildAge   = 7 * 24 * time.Hour
	helperTempAge       = time.Hour
)

// ErrHelperPermanent marks install failures that cannot succeed without a
// configuration change (unsupported platform, read-only home, no SFTP,
// noexec). The pool stops retrying for the lifetime of the application.
var ErrHelperPermanent = errors.New("remote helper is permanently unavailable")

// HelperResourcesDir resolves mygo.PathResources. It is installed by the
// application entry point so this package does not depend on the UI toolkit.
var HelperResourcesDir func() (string, error)

// InstalledHelper identifies the uploaded binary used by one host entry.
type InstalledHelper struct {
	Path     string
	Platform string
	BuildID  string
}

// HelperInstaller uploads and verifies one helper binary. Ops, LocalBinary and
// Now are injectable for tests.
type HelperInstaller struct {
	Ops         HelperRemoteOps
	LocalBinary func(goos, goarch string) (string, error)
	RemoteBase  string
	Now         func() time.Time
}

// Ensure probes the platform, resolves the matching local binary and installs
// it at <base>/<buildID>/<name>. Re-running with the same binary is a no-op.
func (i *HelperInstaller) Ensure(ctx context.Context) (InstalledHelper, error) {
	goos, goarch, err := probeHelperPlatform(ctx, i.Ops)
	if err != nil {
		return InstalledHelper{}, installError(err)
	}
	platform := goos + "/" + goarch
	resolve := i.LocalBinary
	if resolve == nil {
		resolve = HelperLocalPath
	}
	localPath, err := resolve(goos, goarch)
	if err != nil {
		return InstalledHelper{}, installError(err)
	}
	data, err := os.ReadFile(localPath)
	if err != nil {
		return InstalledHelper{}, installError(err)
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	buildID := hash[:12]
	base, err := i.base(ctx)
	if err != nil {
		return InstalledHelper{}, installError(err)
	}
	if err := ensureRemoteDir(ctx, i.Ops, base); err != nil {
		return InstalledHelper{}, installError(err)
	}
	if err := i.Ops.Chmod(ctx, base, helperDirMode); err != nil {
		return InstalledHelper{}, installError(err)
	}
	dir := path.Join(base, buildID)
	if err := ensureRemoteDir(ctx, i.Ops, dir); err != nil {
		return InstalledHelper{}, installError(err)
	}
	if err := i.Ops.Chmod(ctx, dir, helperDirMode); err != nil {
		return InstalledHelper{}, installError(err)
	}
	name := helperBinaryName + "-" + goos + "-" + goarch
	remotePath, err := ensureRemoteFile(ctx, i.Ops, dir, name, data, hash)
	if err != nil {
		return InstalledHelper{}, installError(err)
	}
	// Cleanup is scheduled separately by the pool after a successful handshake;
	// directory enumeration and removal must not delay readiness.
	return InstalledHelper{Path: remotePath, Platform: platform, BuildID: buildID}, nil
}

func (i *HelperInstaller) base(ctx context.Context) (string, error) {
	if i.RemoteBase != "" {
		base := path.Clean(i.RemoteBase)
		if !path.IsAbs(base) || strings.ContainsRune(base, 0) {
			return "", fmt.Errorf("invalid helper base directory")
		}
		return base, nil
	}
	home, err := i.Ops.Home(ctx)
	if err != nil {
		return "", err
	}
	home = path.Clean(home)
	if !path.IsAbs(home) {
		return "", fmt.Errorf("remote home directory is not absolute")
	}
	return path.Join(home, ".local", "share", "gitviewer", "helper", helperRemoteVersion), nil
}

func (i *HelperInstaller) cleanup(ctx context.Context, base, current string) {
	now := time.Now()
	if i.Now != nil {
		now = i.Now()
	}
	entries, err := i.Ops.ReadDir(ctx, base)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if ctx.Err() != nil {
			return
		}
		if name == current || !helperHexName(name, 12) || !entry.IsDir() || entry.Mode()&os.ModeSymlink != 0 {
			continue
		}
		full := path.Join(base, name)
		info, err := i.Ops.Lstat(ctx, full)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if now.Sub(info.ModTime()) > helperOldBuildAge {
			_ = i.Ops.RemoveAll(ctx, full)
			continue
		}
		// Upload temporaries live inside build directories, not at base.
		files, err := i.Ops.ReadDir(ctx, full)
		if err != nil {
			continue
		}
		for _, file := range files {
			if file.Mode().IsRegular() && helperTempName(file.Name()) && now.Sub(file.ModTime()) > helperTempAge {
				_ = i.Ops.Remove(ctx, path.Join(full, file.Name()))
			}
		}
	}
}

func helperHexName(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func helperTempName(name string) bool {
	prefix, suffix, ok := strings.Cut(name, ".tmp.")
	if !ok || !helperHexName(suffix, 16) {
		return false
	}
	prefix = strings.TrimSuffix(prefix, ".sha256")
	for _, goos := range []string{"linux", "darwin"} {
		for _, goarch := range []string{"amd64", "arm64"} {
			if prefix == helperBinaryName+"-"+goos+"-"+goarch {
				return true
			}
		}
	}
	return false
}

// probeHelperPlatform maps uname to the helper's build targets.
func probeHelperPlatform(ctx context.Context, ops HelperRemoteOps) (string, string, error) {
	system, err := ops.Run(ctx, []string{"uname", "-s"}, 5*time.Second)
	if err != nil {
		return "", "", err
	}
	machine, err := ops.Run(ctx, []string{"uname", "-m"}, 5*time.Second)
	if err != nil {
		return "", "", err
	}
	var goos string
	switch strings.ToLower(strings.TrimSpace(string(system.Stdout))) {
	case "linux":
		goos = "linux"
	case "darwin":
		goos = "darwin"
	default:
		return "", "", fmt.Errorf("unsupported remote system")
	}
	var goarch string
	switch strings.ToLower(strings.TrimSpace(string(machine.Stdout))) {
	case "x86_64", "amd64":
		goarch = "amd64"
	case "aarch64", "arm64":
		goarch = "arm64"
	default:
		return "", "", fmt.Errorf("unsupported remote architecture")
	}
	return goos, goarch, nil
}

// ensureRemoteDir creates every missing component with 0700 and refuses to
// traverse an existing symbolic link. This keeps an attacker-controlled path
// component from redirecting the upload.
func ensureRemoteDir(ctx context.Context, ops HelperRemoteOps, dir string) error {
	if !path.IsAbs(dir) || path.Clean(dir) != dir || strings.ContainsRune(dir, 0) {
		return fmt.Errorf("invalid remote directory")
	}
	parts := strings.Split(strings.TrimPrefix(dir, "/"), "/")
	prefix := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		prefix += "/" + part
		info, err := ops.Lstat(ctx, prefix)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("remote helper path contains a symbolic link")
			}
			if !info.IsDir() {
				return fmt.Errorf("remote helper path component is not a directory")
			}
			continue
		}
		if !isNotExist(err) {
			return err
		}
		if err := ops.Mkdir(ctx, prefix, helperDirMode); err != nil {
			// Another helper may have created it between Lstat and Mkdir.
			if info, statErr := ops.Lstat(ctx, prefix); statErr == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
				continue
			}
			return err
		}
	}
	return nil
}

func ensureRemoteFile(ctx context.Context, ops HelperRemoteOps, dir, name string, data []byte, hash string) (string, error) {
	remotePath := path.Join(dir, name)
	sidecar := remotePath + ".sha256"
	// Sidecars are untrusted metadata. Refuse links even when the binary is
	// missing, and hash the actual binary before accepting an existing upload.
	if info, err := ops.Lstat(ctx, sidecar); err == nil {
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("remote helper sidecar is not a regular file")
		}
	} else if !isNotExist(err) {
		return "", err
	}
	if info, err := ops.Lstat(ctx, remotePath); err == nil {
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("remote helper path is not a regular file")
		}
		if info.Size() == int64(len(data)) {
			valid, err := verifyRemoteHash(ctx, ops, remotePath, int64(len(data)), hash)
			if err != nil {
				return "", err
			}
			sidecarData, sideErr := ops.ReadFile(ctx, sidecar, 128)
			if sideErr != nil && !isNotExist(sideErr) {
				return "", sideErr
			}
			if valid && sideErr == nil && strings.TrimSpace(string(sidecarData)) == hash {
				if err := ops.Chmod(ctx, remotePath, helperBinaryMode); err != nil {
					return "", err
				}
				return remotePath, nil
			}
		}
	} else if !isNotExist(err) {
		return "", err
	}
	tmp, err := tempName(remotePath)
	if err != nil {
		return "", err
	}
	if err := writeRemoteFile(ctx, ops, tmp, data, helperBinaryMode); err != nil {
		return "", err
	}
	valid, err := verifyRemoteHash(ctx, ops, tmp, int64(len(data)), hash)
	if err != nil || !valid {
		_ = ops.Remove(ctx, tmp)
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("remote helper upload hash verification failed")
	}
	if err := ops.Rename(ctx, tmp, remotePath); err != nil {
		_ = ops.Remove(ctx, tmp)
		return "", err
	}
	sidecarTmp, err := tempName(sidecar)
	if err != nil {
		return "", err
	}
	if err := writeRemoteFile(ctx, ops, sidecarTmp, []byte(hash+"\n"), helperSidecarMode); err != nil {
		return "", err
	}
	if err := ops.Rename(ctx, sidecarTmp, sidecar); err != nil {
		_ = ops.Remove(ctx, sidecarTmp)
		return "", err
	}
	info, err := ops.Lstat(ctx, remotePath)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() != int64(len(data)) {
		return "", fmt.Errorf("remote helper upload verification failed")
	}
	valid, err = verifyRemoteHash(ctx, ops, remotePath, int64(len(data)), hash)
	if err != nil {
		return "", err
	}
	if !valid {
		return "", fmt.Errorf("remote helper upload hash verification failed")
	}
	return remotePath, nil
}

func verifyRemoteHash(ctx context.Context, ops HelperRemoteOps, name string, size int64, expected string) (bool, error) {
	reader, err := ops.OpenRead(ctx, name)
	if err != nil {
		return false, err
	}
	hash := sha256.New()
	n, readErr := io.Copy(hash, io.LimitReader(reader, size+1))
	closeErr := reader.Close()
	if readErr != nil {
		return false, readErr
	}
	if closeErr != nil {
		return false, closeErr
	}
	return n == size && hex.EncodeToString(hash.Sum(nil)) == expected, nil
}

func writeRemoteFile(ctx context.Context, ops HelperRemoteOps, remotePath string, data []byte, mode os.FileMode) (err error) {
	// OpenWrite must use O_EXCL. A collision must never truncate or remove
	// someone else's file, including a pre-existing symbolic link.
	writer, err := ops.OpenWrite(ctx, remotePath)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = ops.Remove(ctx, remotePath)
		}
	}()
	n, writeErr := writer.Write(data)
	closeErr := writer.Close()
	if writeErr != nil {
		return writeErr
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	if closeErr != nil {
		return closeErr
	}
	return ops.Chmod(ctx, remotePath, mode)
}

func tempName(base string) (string, error) {
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", err
	}
	return base + ".tmp." + hex.EncodeToString(suffix[:]), nil
}

// installError separates transient connection failures (retry with backoff)
// from permanent deployment failures.
func installError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrHelperPermanent) {
		return err
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var networkError net.Error
	if errors.As(err, &networkError) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return &domain.GitError{Code: domain.RemoteUnavailable, Cause: err}
	}
	var ge *domain.GitError
	if errors.As(err, &ge) {
		switch ge.Code {
		case domain.RemoteUnavailable, domain.OperationTimedOut, domain.OperationCancelled,
			domain.SshAuthFailed, domain.HostKeyRejected:
			return err
		}
	}
	return fmt.Errorf("%w: %v", ErrHelperPermanent, err)
}

// HelperLocalPath finds the shipped helper for one target. Development builds
// use resources/helper/<goos>-<goarch>; a packaged app uses Contents/Resources.
func HelperLocalPath(goos, goarch string) (string, error) {
	if !supportedHelperTarget(goos, goarch) {
		return "", fmt.Errorf("unsupported helper target %s/%s", goos, goarch)
	}
	if value := os.Getenv("GITVIEWER_HELPER_PATH"); value != "" {
		return regularFile(value)
	}
	rel := filepath.Join("helper", goos+"-"+goarch, helperBinaryName)
	var candidates []string
	if HelperResourcesDir != nil {
		if dir, err := HelperResourcesDir(); err == nil && dir != "" {
			candidates = append(candidates, filepath.Join(dir, rel))
		}
	}
	if executable, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(executable)
		candidates = append(candidates,
			filepath.Join(exeDir, rel),
			filepath.Join(exeDir, "resources", rel),
			filepath.Join(exeDir, "..", "Resources", rel),
			filepath.Join(exeDir, "..", "Resources", "helper", goos+"-"+goarch, helperBinaryName),
		)
	}
	if workingDir, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(workingDir, "resources", rel), filepath.Join(workingDir, rel))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("helper binary for %s/%s was not found", goos, goarch)
}

func regularFile(value string) (string, error) {
	info, err := os.Stat(value)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("helper binary is missing")
	}
	return value, nil
}

func supportedHelperTarget(goos, goarch string) bool {
	switch goos {
	case "linux", "darwin":
		return goarch == "amd64" || goarch == "arm64"
	}
	return false
}
