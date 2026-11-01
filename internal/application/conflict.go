package application

import (
	"bytes"
	"context"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/git"
	"gitviewer/internal/infrastructure/sshfs"
)

const maxConflictFileSize = 8 << 20

// ConflictFile returns Git's three stages plus the current worktree text when
// it is a bounded, editable UTF-8 file.
func (s LocalService) ConflictFile(ctx context.Context, repo domain.RepositoryContext, file string) (domain.ConflictFile, error) {
	conflict, err := s.ReaderFor(repo).ConflictFile(ctx, repo, file)
	if err != nil {
		return conflict, err
	}
	result, available, err := s.readConflictResult(ctx, repo, file)
	if err != nil {
		return conflict, err
	}
	conflict.Result, conflict.ResultAvailable = result, available
	return conflict, nil
}

func (s LocalService) readConflictResult(ctx context.Context, repo domain.RepositoryContext, file string) (string, bool, error) {
	if repo.Repository.Location == "ssh" {
		client, err := sshfs.Open(ctx, s.Remote, repo.SSHHost, repo.Repository.Path)
		if err != nil {
			return "", false, err
		}
		defer client.Close()
		info, err := client.Stat(ctx, path.Join(repo.Repository.Path, file))
		if err != nil || info.Dir || info.Symlink || info.Size > maxConflictFileSize {
			return "", false, nil
		}
		data, err := client.ReadFile(ctx, info.Path, maxConflictFileSize)
		if err != nil {
			return "", false, err
		}
		return conflictText(data)
	}
	if repo.Repository.Location != "local" {
		return "", false, &domain.GitError{Code: domain.InvalidRepository}
	}
	root, err := os.OpenRoot(repo.Repository.Path)
	if err != nil {
		return "", false, &domain.GitError{Code: domain.InvalidRepository}
	}
	defer root.Close()
	fileHandle, err := root.Open(file)
	if err != nil {
		return "", false, nil
	}
	defer fileHandle.Close()
	info, err := fileHandle.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxConflictFileSize {
		return "", false, nil
	}
	data, err := io.ReadAll(io.LimitReader(fileHandle, maxConflictFileSize+1))
	if err != nil {
		return "", false, err
	}
	if len(data) > maxConflictFileSize {
		return "", false, nil
	}
	return conflictText(data)
}

func conflictText(data []byte) (string, bool, error) {
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		return "", false, nil
	}
	return string(data), true, nil
}

// ResolveConflict atomically replaces the worktree result and stages exactly
// that path. The path must still be unresolved when this method starts.
func (s LocalService) ResolveConflict(ctx context.Context, repo domain.RepositoryContext, file, content string) (git.WriteResult, error) {
	var result git.WriteResult
	if len(content) > maxConflictFileSize || strings.ContainsRune(content, 0) || !validConflictPath(file) {
		return result, &domain.GitError{Code: domain.UnsafeOperation}
	}
	if _, err := s.ReaderFor(repo).ConflictFile(ctx, repo, file); err != nil {
		return result, err
	}
	if err := s.writeConflictResult(ctx, repo, file, []byte(content)); err != nil {
		return result, err
	}
	return s.Write(ctx, repo, domain.GitOperation{
		RepositoryID: repo.Repository.RepositoryID,
		Operation:    domain.Stage,
		Paths:        []string{file},
	})
}

func validConflictPath(file string) bool {
	return file != "" && !strings.ContainsRune(file, 0) &&
		filepath.IsLocal(file) && filepath.ToSlash(filepath.Clean(file)) == file && file != "."
}

func (s LocalService) writeConflictResult(ctx context.Context, repo domain.RepositoryContext, file string, data []byte) error {
	if repo.Repository.Location == "ssh" {
		client, err := sshfs.Open(ctx, s.Remote, repo.SSHHost, repo.Repository.Path)
		if err != nil {
			return err
		}
		defer client.Close()
		target := path.Join(repo.Repository.Path, file)
		info, err := client.Stat(ctx, target)
		if err != nil || info.Dir || info.Symlink {
			return &domain.GitError{Code: domain.UnsafeOperation, Reason: "The conflicted file is not a regular file"}
		}
		return client.WriteFileAtomic(ctx, target, data, info.Mode.Perm())
	}
	if repo.Repository.Location != "local" {
		return &domain.GitError{Code: domain.InvalidRepository}
	}
	root, err := os.OpenRoot(repo.Repository.Path)
	if err != nil {
		return &domain.GitError{Code: domain.InvalidRepository}
	}
	defer root.Close()
	info, err := root.Lstat(file)
	if err != nil || !info.Mode().IsRegular() {
		return &domain.GitError{Code: domain.UnsafeOperation, Reason: "The conflicted file is not a regular file"}
	}
	return writeConflictAtomic(repo.Repository.Path, file, data, info.Mode().Perm())
}

func writeConflictAtomic(rootPath, file string, data []byte, mode os.FileMode) error {
	target := filepath.Join(rootPath, filepath.FromSlash(file))
	if !strings.HasPrefix(target, rootPath+string(filepath.Separator)) {
		return &domain.GitError{Code: domain.UnsafeOperation}
	}
	temp, err := os.CreateTemp(filepath.Dir(target), ".gitviewer-conflict-*.tmp")
	if err != nil {
		return err
	}
	name := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(name)
	}
	if _, err := temp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := temp.Chmod(mode.Perm()); err != nil {
		cleanup()
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, target); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}
