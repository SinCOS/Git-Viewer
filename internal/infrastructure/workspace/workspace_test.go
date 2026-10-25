package workspace_test

import (
	"bytes"
	"context"
	"errors"
	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
	"gitviewer/internal/infrastructure/workspace"
	"gitviewer/internal/testgit"
	"os"
	"path/filepath"
	"testing"
)

func TestJSONCPreservesAndRejectsConflicts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "中文 workspace.code-workspace")
	original := []byte("{\n // outside\n \"folders\": [ // folder comment\n {\"path\": \"old\",},],\n \"settings\": {\"url\": \"https://example.invalid/*x*/\",},\n \"unknown\": [1,2,], // tail\n}\n")
	testgit.Write(t, path, string(original))
	d, err := workspace.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	unchanged, err := d.Edit(d.Folders)
	if err != nil || !bytes.Equal(unchanged, original) {
		t.Fatal("round trip", err)
	}
	saved, err := workspace.Save(path, d.Revision, []domain.WorkspaceFolder{{Path: "new", Name: "中文"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"// outside", "// folder comment", "\"settings\": {\"url\": \"https://example.invalid/*x*/\",}", "\"unknown\": [1,2,]", "// tail"} {
		if !bytes.Contains(saved.Text, []byte(part)) {
			t.Fatalf("lost %s: %s", part, saved.Text)
		}
	}
	testgit.Write(t, path, "{\"folders\":[],\"external\":true}")
	_, err = workspace.Save(path, saved.Revision, nil)
	if !errors.Is(err, workspace.ErrConflict) {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Contains(after, []byte("external")) {
		t.Fatal("external update lost")
	}
	if _, err = workspace.Create(path, nil); err == nil {
		t.Fatal("create overwrote existing file")
	}
	if _, err = workspace.Parse([]byte(`{"folders":[],"folders":[]}`)); err == nil {
		t.Fatal("duplicate key accepted")
	}
	if _, err = workspace.Parse([]byte(`{"folders":[] /*`)); err == nil {
		t.Fatal("unterminated comment accepted")
	}
	unknown, _ := workspace.Parse([]byte(`{"folders":[{"path":"old","uri":"keep"}]}`))
	if _, err = unknown.Edit([]domain.WorkspaceFolder{{Path: "new"}}); err == nil {
		t.Fatal("unknown folder property lost")
	}
}
func TestSaveFailuresPreserveOriginalAndCleanTemps(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "protected.code-workspace")
	d, err := workspace.Create(path, []domain.WorkspaceFolder{{Path: "old"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(path, 0400); err != nil {
		t.Fatal(err)
	}
	_, err = workspace.Save(path, d.Revision, []domain.WorkspaceFolder{{Path: "new"}})
	if !errors.Is(err, os.ErrPermission) {
		t.Fatal("read-only document replaced", err)
	}
	os.Chmod(path, 0600)
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, d.Text) {
		t.Fatal("failed save changed document")
	}
	if os.Geteuid() != 0 {
		if err = os.Chmod(dir, 0500); err != nil {
			t.Fatal(err)
		}
		_, err = workspace.Save(path, d.Revision, []domain.WorkspaceFolder{{Path: "new"}})
		os.Chmod(dir, 0700)
		if err == nil {
			t.Fatal("unwritable directory accepted")
		}
		after, _ = os.ReadFile(path)
		if !bytes.Equal(after, d.Text) {
			t.Fatal("atomic failure changed document")
		}
	}
	temps, _ := filepath.Glob(filepath.Join(dir, ".gitviewer-workspace-*"))
	if len(temps) > 0 {
		t.Fatal("temporary files leaked", temps)
	}
}
func TestDiscoveryCreateAndSymlinks(t *testing.T) {
	dir := t.TempDir()
	repo := testgit.Repo(t)
	link := filepath.Join(dir, "link")
	if err := os.Symlink(repo, link); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "new.code-workspace")
	d, err := workspace.Create(path, []domain.WorkspaceFolder{{Path: repo}, {Path: "link"}, {Path: "missing"}, {Path: dir}})
	if err != nil {
		t.Fatal(err)
	}
	view, repos, err := workspace.View(context.Background(), path, d, gitexec.LocalReader{})
	if err != nil || len(repos) != 1 || len(view.Folders) != 4 || len(view.Folders[2].Diagnostics) == 0 || len(view.Folders[3].Diagnostics) == 0 {
		t.Fatal(view, repos, err)
	}
	symlink := filepath.Join(dir, "alias.code-workspace")
	if err = os.Symlink(path, symlink); err != nil {
		t.Fatal(err)
	}
	if _, err = workspace.Read(symlink); err == nil {
		t.Fatal("symlink document accepted")
	}
	files, _ := filepath.Glob(filepath.Join(dir, ".gitviewer-workspace-*"))
	if len(files) != 0 {
		t.Fatal("temporary files leaked", files)
	}
}
