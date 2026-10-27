package local

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	"gitviewer/internal/domain"
)

const externalEditorFileLimit = 8 << 20

// OpenInEditor is called on the UI thread with a repository-relative path.
// Validation and launching run in a.start's worker; the result is applied on
// the UI thread through Dispatch. The file is opened from the current worktree.
func (a *App) OpenInEditor(path string) {
	repo := a.Repository
	a.start(func(ctx context.Context) (func(), error) {
		result := openLocalEditor(ctx, repo, path)
		return func() { a.Message = result }, nil
	})
}

// OpenInTerminal asks macOS to open the repository directory with Terminal.
// The system application controls the resulting shell and its working directory.
func (a *App) OpenInTerminal() {
	repo := a.Repository
	a.start(func(ctx context.Context) (func(), error) {
		root, result := externalRepositoryRoot(repo)
		if result == "" {
			switch {
			case runtime.GOOS != "darwin":
				result = "Open in Terminal is available only on macOS"
			case ctx.Err() != nil:
				result = "Open in Terminal cancelled"
			default:
				err := exec.CommandContext(ctx, "/usr/bin/open", "-b", "com.apple.Terminal", root).Run()
				switch {
				case ctx.Err() != nil:
					result = "Open in Terminal cancelled"
				case err != nil:
					result = "Could not open the repository folder in Terminal"
				default:
					result = "Requested Terminal for the repository folder"
				}
			}
		}
		return func() { a.Message = result }, nil
	})
}

func externalRepositoryRoot(repo domain.RepositoryContext) (string, string) {
	path := repo.Repository.Path
	if repo.Repository.Location != "local" || repo.Repository.RepositoryID == "" || repo.SSHHost != "" || !filepath.IsAbs(path) || strings.ContainsRune(path, 0) || hasParentComponent(path) {
		return "", "A valid local repository is required"
	}
	root, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", "Repository directory is unavailable"
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "", "Repository directory is unavailable"
	}
	return root, ""
}

func hasParentComponent(path string) bool {
	for _, part := range strings.Split(path, string(filepath.Separator)) {
		if part == ".." {
			return true
		}
	}
	return false
}

func openLocalEditor(ctx context.Context, repo domain.RepositoryContext, path string) string {
	root, problem := externalRepositoryRoot(repo)
	if problem != "" {
		return problem
	}
	if runtime.GOOS != "darwin" {
		return "Open in Editor is available only on macOS"
	}
	if path == "" || strings.ContainsRune(path, 0) || filepath.IsAbs(path) || hasParentComponent(path) {
		return "A valid repository-relative file path is required"
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(root, path))
	if err != nil {
		return "File is unavailable"
	}
	rel, err := filepath.Rel(root, resolved)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "File must be inside the repository"
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "File is unavailable"
	}
	if !info.Mode().IsRegular() {
		return "Only regular files can be opened in the editor"
	}
	// Root confines the read even if a symlink changes during validation.
	dir, err := os.OpenRoot(root)
	if err != nil {
		return "Repository directory is unavailable"
	}
	defer dir.Close()
	file, err := dir.Open(rel)
	if err != nil {
		return "File is unavailable"
	}
	defer file.Close()
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return "Only regular files can be opened in the editor"
	}
	if info.Size() > externalEditorFileLimit {
		return "File is too large to check before opening"
	}
	data, err := io.ReadAll(io.LimitReader(file, externalEditorFileLimit+1))
	if err != nil {
		return "File is unavailable"
	}
	if len(data) > externalEditorFileLimit {
		return "File is too large to check before opening"
	}
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		return "Binary files cannot be opened in the editor"
	}
	if ctx.Err() != nil {
		return "Open in Editor cancelled"
	}
	// -t selects the default text editor. No shell or stderr is exposed.
	if err := exec.CommandContext(ctx, "/usr/bin/open", "-t", resolved).Run(); err != nil {
		if ctx.Err() != nil {
			return "Open in Editor cancelled"
		}
		return "Could not open the file in the default text editor"
	}
	return "Opened in the default text editor"
}
