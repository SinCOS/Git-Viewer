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
	if request.OldPath != "" && !validPath(request.OldPath) {
		return reject()
	}
	if request.NewPath != "" && !validPath(request.NewPath) {
		return reject()
	}
	if request.Scope != domain.CommitDiff && request.Commit != "" {
		return reject()
	}
	if request.Scope != domain.CommitDiff && (request.OldPath != "" || request.NewPath != "") {
		return reject()
	}
	var paths []string
	var change *domain.Change
	if request.Path != "" {
		paths = append(paths, request.Path)
	}
	if request.OldPath != "" && request.OldPath != request.Path {
		paths = append(paths, request.OldPath)
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
				change = found
				break
			}
		}
		if found == nil {
			return preview, nil
		} // file was cleaned since status was loaded
		if found.Untracked {
			var err error
			if r.Transport != nil {
				preview, err = r.remoteUntracked(ctx, repo, preview)
			} else {
				preview, err = r.untracked(ctx, repo, preview)
			}
			if err != nil {
				return preview, err
			}
			if preview.Binary || preview.Limited || r.Transport != nil {
				return preview, nil
			}
			return r.loadFullWorkingText(ctx, repo, request.Scope, *change, preview)
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
	// Git 2.25 does not understand --no-relative. Running from the worktree
	// root already keeps patch paths relative to that root.
	args = append(args, "--no-ext-diff", "--no-textconv", "--no-color", "--find-renames", "--submodule=short", "--patch", "--")
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
	if request.Path != "" && !preview.Binary && !preview.Limited {
		switch request.Scope {
		case domain.CommitDiff:
			preview, err = r.loadFullCommitText(ctx, repo, request, preview)
		case domain.WorktreeDiff, domain.StagedDiff:
			if change != nil {
				preview, err = r.loadFullWorkingText(ctx, repo, request.Scope, *change, preview)
			}
		}
		if err != nil {
			return preview, err
		}
	}
	return preview, nil
}

// loadFullCommitText loads both sides of a selected commit file. The patch
// remains in Text for copy/stats; FullText tells the editor to compare the
// complete blobs instead of only the hunks in the patch.
func (r LocalReader) loadFullCommitText(ctx context.Context, repo domain.RepositoryContext, request domain.DiffRequest, preview domain.DiffPreview) (domain.DiffPreview, error) {
	parent, hasParent, err := r.firstParent(ctx, repo, request.Commit)
	if err != nil {
		return preview, err
	}
	oldText, oldOK := "", true
	if hasParent && request.OldPath != "" {
		oldText, oldOK, err = r.readCommitFile(ctx, repo, parent, request.OldPath)
		if err != nil {
			return preview, err
		}
	}
	newText, newOK, err := r.readCommitFile(ctx, repo, request.Commit, request.NewPath)
	if err != nil {
		return preview, err
	}
	if oldOK && newOK {
		preview.OldText, preview.NewText, preview.FullText = oldText, newText, true
	}
	return preview, nil
}

func (r LocalReader) firstParent(ctx context.Context, repo domain.RepositoryContext, commit string) (string, bool, error) {
	result, err := r.run(ctx, repo.Repository.Path, "rev-list", "--parents", "-n", "1", commit+"^{commit}", "--")
	if err != nil {
		return "", false, err
	}
	fields := strings.Fields(string(result.Stdout))
	if len(fields) == 0 || fields[0] != commit {
		return "", false, parseError()
	}
	if len(fields) == 1 {
		return "", false, nil
	}
	if !oidPattern.MatchString(fields[1]) {
		return "", false, parseError()
	}
	return fields[1], true, nil
}

func (r LocalReader) readCommitFile(ctx context.Context, repo domain.RepositoryContext, revision, path string) (string, bool, error) {
	if path == "" {
		return "", true, nil
	}
	return r.readBlob(ctx, repo, revision+":"+path, false)
}

func (r LocalReader) readBlob(ctx context.Context, repo domain.RepositoryContext, spec string, missingEmpty bool) (string, bool, error) {
	bounded := r
	if bounded.OutputLimit <= 0 || bounded.OutputLimit > PreviewLimit {
		bounded.OutputLimit = PreviewLimit
	}
	result, err := bounded.run(ctx, repo.Repository.Path, "cat-file", "blob", spec)
	if err != nil {
		var ge *domain.GitError
		if errors.As(err, &ge) {
			switch ge.Code {
			case domain.OutputLimitExceeded:
				return "", false, nil
			case domain.CommandFailed:
				if missingEmpty {
					return "", true, nil
				}
				return "", false, nil
			}
		}
		return "", false, err
	}
	if !utf8.Valid(result.Stdout) || bytes.IndexByte(result.Stdout, 0) >= 0 {
		return "", false, nil
	}
	limited := false
	text := displayText(result.Stdout, &limited)
	if limited {
		return "", false, nil
	}
	return text, true, nil
}

func (r LocalReader) loadFullWorkingText(ctx context.Context, repo domain.RepositoryContext, scope domain.DiffScope, change domain.Change, preview domain.DiffPreview) (domain.DiffPreview, error) {
	if change.Conflicted {
		return preview, nil
	}
	var oldText, newText string
	var oldOK, newOK bool
	var err error
	switch scope {
	case domain.WorktreeDiff:
		if change.Untracked {
			oldText, oldOK = "", true
		} else {
			oldText, oldOK, err = r.readBlob(ctx, repo, ":"+change.Path, true)
			if err != nil {
				return preview, err
			}
		}
		newText, newOK, err = r.readWorktreeText(ctx, repo, change.Path)
	case domain.StagedDiff:
		oldPath := change.Path
		if change.OldPath != nil {
			oldPath = *change.OldPath
		}
		oldText, oldOK, err = r.readBlob(ctx, repo, "HEAD:"+oldPath, true)
		if err != nil {
			return preview, err
		}
		newText, newOK, err = r.readBlob(ctx, repo, ":"+change.Path, true)
	default:
		return preview, nil
	}
	if err != nil {
		return preview, err
	}
	if oldOK && newOK {
		preview.OldText, preview.NewText, preview.FullText = oldText, newText, true
	}
	return preview, nil
}

func (r LocalReader) readWorktreeText(ctx context.Context, repo domain.RepositoryContext, path string) (string, bool, error) {
	if r.Transport != nil {
		return r.remoteWorktreeText(ctx, repo, path)
	}
	return r.localWorktreeText(ctx, repo, path)
}

func (r LocalReader) localWorktreeText(ctx context.Context, repo domain.RepositoryContext, path string) (string, bool, error) {
	if ctx.Err() != nil {
		return "", false, &domain.GitError{Code: domain.OperationCancelled}
	}
	root, err := os.OpenRoot(repo.Repository.Path)
	if err != nil {
		return "", false, &domain.GitError{Code: domain.InvalidRepository}
	}
	defer root.Close()
	info, err := root.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", true, nil
		}
		return "", false, &domain.GitError{Code: domain.CommandFailed}
	}
	var data []byte
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := root.Readlink(path)
		if err != nil {
			return "", false, &domain.GitError{Code: domain.CommandFailed}
		}
		data = []byte(target)
	} else {
		if !info.Mode().IsRegular() {
			return "", false, nil
		}
		if info.Size() > PreviewLimit {
			return "", false, nil
		}
		file, err := root.Open(path)
		if err != nil {
			return "", false, &domain.GitError{Code: domain.CommandFailed}
		}
		defer file.Close()
		data, err = io.ReadAll(io.LimitReader(file, PreviewLimit+1))
		if err != nil {
			return "", false, &domain.GitError{Code: domain.CommandFailed}
		}
	}
	if ctx.Err() != nil {
		return "", false, &domain.GitError{Code: domain.OperationCancelled}
	}
	if len(data) > PreviewLimit || bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		return "", false, nil
	}
	limited := false
	text := displayText(data, &limited)
	if limited {
		return "", false, nil
	}
	return text, true, nil
}

func (r LocalReader) remoteWorktreeText(ctx context.Context, repo domain.RepositoryContext, path string) (string, bool, error) {
	r.OutputLimit = PreviewLimit
	result, err := r.run(ctx, repo.Repository.Path, "diff", "--no-index", "--no-ext-diff", "--no-textconv", "--no-color", "--", "/dev/null", path)
	if err != nil {
		var ge *domain.GitError
		if errors.As(err, &ge) {
			switch ge.Code {
			case domain.OutputLimitExceeded:
				return "", false, nil
			case domain.CommandFailed:
				if result.ExitCode == 1 {
					break
				}
				return "", false, err
			default:
				return "", false, err
			}
		} else {
			return "", false, err
		}
	}
	if !utf8.Valid(result.Stdout) ||
		bytes.Contains(result.Stdout, []byte("\nBinary files ")) ||
		bytes.HasPrefix(result.Stdout, []byte("Binary files ")) {
		return "", false, nil
	}
	text, ok := addedTextFromPatch(result.Stdout)
	return text, ok, nil
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
