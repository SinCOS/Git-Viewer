package workspace_test

import (
	"context"
	"errors"
	"os"
	"path"
	"strings"
	"sync"
	"testing"
	"time"

	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/workspace"
)

type memoryFS struct {
	mu    sync.Mutex
	files map[string][]byte
	modes map[string]os.FileMode
}

func (m *memoryFS) Canonical(_ context.Context, p string) (string, error) { return path.Clean(p), nil }
func (m *memoryFS) Stat(_ context.Context, p string) (domain.RemoteFileInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.files[p]
	if !ok {
		return domain.RemoteFileInfo{}, &domain.GitError{Code: domain.InvalidRepository}
	}
	return domain.RemoteFileInfo{Path: p, Name: path.Base(p), Size: int64(len(data)), Mode: m.modes[p], ModTime: time.Unix(1, 0)}, nil
}
func (m *memoryFS) ReadDir(context.Context, string) ([]domain.RemoteFileInfo, error) {
	return nil, nil
}
func (m *memoryFS) ReadFile(_ context.Context, p string, limit int64) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.files[p]
	if !ok {
		return nil, &domain.GitError{Code: domain.InvalidRepository}
	}
	if int64(len(data)) > limit {
		return nil, &domain.GitError{Code: domain.OutputLimitExceeded}
	}
	return append([]byte(nil), data...), nil
}
func (m *memoryFS) WriteFileAtomic(_ context.Context, p string, data []byte, mode os.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[p] = append([]byte(nil), data...)
	m.modes[p] = mode
	return nil
}
func (m *memoryFS) Walk(context.Context, string, map[string]bool) ([]string, error) {
	return nil, nil
}
func (m *memoryFS) Close() error { return nil }

func TestRemoteStorePreservesJSONCAndSameHostRoots(t *testing.T) {
	file := "/remote/team.code-workspace"
	text := []byte("{\n // keep\n \"folders\": [\n  {\"path\": \"a\"},\n  {\"path\": \"/remote/b\"},\n  {\"path\": \"a\"},\n ],\n \"unknown\": {\"x\": true},\n}\n")
	fs := &memoryFS{
		files: map[string][]byte{file: text},
		modes: map[string]os.FileMode{file: 0o644},
	}
	store := workspace.RemoteStore{Host: "dev", Files: fs}
	result, err := store.Open(context.Background(), file, func(_ context.Context, root string) (*domain.RepositoryContext, error) {
		if root == "/remote/a" {
			return &domain.RepositoryContext{Repository: domain.Repository{RepositoryID: "repo-a", Location: "ssh", Path: root}, SSHHost: "dev"}, nil
		}
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.View.Location != "ssh" || result.View.SSHHost != "dev" || len(result.Roots) != 2 || len(result.Repositories) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if result.Roots[0].Path != "/remote/a" || result.Roots[1].Path != "/remote/b" {
		t.Fatalf("roots = %+v", result.Roots)
	}
	saved, err := store.Save(context.Background(), file, result.Document.Revision, []domain.WorkspaceFolder{{Path: "/remote/c"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved.Text), "// keep") || !strings.Contains(string(saved.Text), "\"unknown\"") {
		t.Fatalf("JSONC fields lost: %s", saved.Text)
	}
	if _, err := store.Save(context.Background(), file, result.Document.Revision, nil); !errors.Is(err, workspace.ErrConflict) {
		t.Fatalf("stale revision accepted: %v", err)
	}
	if _, err := store.Create(context.Background(), file, nil); err == nil {
		t.Fatal("create overwrote existing workspace")
	}
}
