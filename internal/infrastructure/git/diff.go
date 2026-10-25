package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"gitviewer/internal/domain"
)

const PreviewLimit = 512 * 1024
const PreviewLineLimit = 10000

func validPath(path string) bool {
	return path != "" && filepath.IsLocal(path) && !strings.ContainsRune(path, 0) && filepath.ToSlash(filepath.Clean(path)) == path && path != "."
}

func (r LocalReader) Diff(ctx context.Context, repo domain.RepositoryContext, request domain.DiffRequest) (domain.DiffPreview, error) {
	preview := domain.DiffPreview{RepositoryID: repo.Repository.RepositoryID, Scope: request.Scope, Path: request.Path}
	reject := func() (domain.DiffPreview, error) { return preview, &domain.GitError{Code: domain.UnsafeOperation} }
	if (repo.Repository.Location != "local" && r.Transport == nil) || repo.Repository.RepositoryID == "" {
		return preview, &domain.GitError{Code: domain.InvalidRepository}
	}
	if request.Path != "" && !validPath(request.Path) {
		return reject()
	}
	if request.Scope != domain.CommitDiff && request.Commit != "" {
		return reject()
	}
	var paths []string
	if request.Path != "" {
		paths = append(paths, request.Path)
	}
	switch request.Scope {
	case domain.WorktreeDiff, domain.StagedDiff:
		if request.Path == "" {
			return reject()
		}
		status, err := r.Status(ctx, repo)
		if err != nil {
			return preview, err
		}
		var found *domain.Change
		for _, entry := range domain.StatusEntries(status) {
			if entry.Change.Path == request.Path && entry.Scope == request.Scope {
				c := entry.Change
				found = &c
				break
			}
		}
		if found == nil {
			return preview, nil
		} // file was cleaned since status was loaded
		if found.Untracked {
			if r.Transport != nil {
				return r.remoteUntracked(ctx, repo, preview)
			}
			return r.untracked(ctx, repo, preview)
		}
		if found.OldPath != nil {
			if !validPath(*found.OldPath) {
				return reject()
			}
			paths = append(paths, *found.OldPath)
		}
	case domain.CommitDiff:
		if !oidPattern.MatchString(request.Commit) {
			return reject()
		}
	default:
		return reject()
	}
	args := []string{"diff"}
	if request.Scope == domain.StagedDiff {
		args = append(args, "--cached")
	}
	if request.Scope == domain.CommitDiff {
		args = []string{"show", "--format=", "--root", "--first-parent", request.Commit + "^{commit}"}
	}
	args = append(args, "--no-ext-diff", "--no-textconv", "--no-color", "--no-relative", "--find-renames", "--submodule=short", "--patch", "--")
	args = append(args, paths...)
	bounded := r
	if bounded.OutputLimit <= 0 || bounded.OutputLimit > PreviewLimit {
		bounded.OutputLimit = PreviewLimit
	}
	result, err := bounded.run(ctx, repo.Repository.Path, args...)
	if err != nil {
		var ge *domain.GitError
		if errors.As(err, &ge) && ge.Code == domain.OutputLimitExceeded {
			preview.Limited = true
			return preview, nil
		}
		return preview, err
	}
	preview.Binary = !utf8.Valid(result.Stdout) || bytes.Contains(result.Stdout, []byte("\nBinary files "))
	if !utf8.Valid(result.Stdout) {
		return preview, nil
	}
	preview.Text = displayText(result.Stdout, &preview.Limited)
	return preview, nil
}

func (r LocalReader) untracked(ctx context.Context, repo domain.RepositoryContext, preview domain.DiffPreview) (domain.DiffPreview, error) {
	if ctx.Err() != nil {
		return preview, &domain.GitError{Code: domain.OperationCancelled}
	}
	root, err := os.OpenRoot(repo.Repository.Path)
	if err != nil {
		return preview, &domain.GitError{Code: domain.InvalidRepository}
	}
	defer root.Close()
	info, err := root.Lstat(preview.Path)
	if err != nil {
		return preview, &domain.GitError{Code: domain.CommandFailed}
	}
	// A symlink's diff is its link text; never follow it into another directory.
	var data []byte
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := root.Readlink(preview.Path)
		if err != nil {
			return preview, &domain.GitError{Code: domain.CommandFailed}
		}
		data = []byte(target)
	} else {
		if !info.Mode().IsRegular() {
			return preview, &domain.GitError{Code: domain.UnsafeOperation}
		}
		if info.Size() > PreviewLimit {
			preview.Limited = true
			return preview, nil
		}
		file, err := root.Open(preview.Path)
		if err != nil {
			return preview, &domain.GitError{Code: domain.CommandFailed}
		}
		defer file.Close()
		data, err = io.ReadAll(io.LimitReader(file, PreviewLimit+1))
		if err != nil {
			return preview, &domain.GitError{Code: domain.CommandFailed}
		}
	}
	if ctx.Err() != nil {
		return preview, &domain.GitError{Code: domain.OperationCancelled}
	}
	if len(data) > PreviewLimit {
		preview.Limited = true
		return preview, nil
	}
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		preview.Binary = true
		return preview, nil
	}
	var text strings.Builder
	fmt.Fprintf(&text, "Untracked file %q\n", preview.Path)
	if len(data) != 0 {
		lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
		if len(lines) > PreviewLineLimit-3 {
			preview.Limited = true
			return preview, nil
		}
		fmt.Fprintf(&text, "@@ -0,0 +1,%d @@\n", len(lines))
		for _, line := range lines {
			text.WriteString("+")
			text.WriteString(line)
			text.WriteByte('\n')
		}
		if data[len(data)-1] != '\n' {
			text.WriteString("\\ No newline at end of file\n")
		}
	}
	preview.Text = displayText([]byte(text.String()), &preview.Limited)
	return preview, nil
}

// Escape control characters to prevent invisible filename/content spoofing.
// Tabs and line breaks stay readable; non UTF-8 content uses a binary fallback.
func displayText(data []byte, limited *bool) string {
	if bytes.Count(data, []byte{'\n'}) > PreviewLineLimit || len(data) > PreviewLimit {
		*limited = true
		return ""
	}
	return strings.Map(func(c rune) rune {
		if c < 32 && c != '\n' && c != '\t' || c == 127 {
			return '�'
		}
		return c
	}, string(data))
}
