package git

import (
	"bytes"
	"context"
	"errors"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"gitviewer/internal/domain"
)

// FileHistoryResult is a bounded snapshot of commits affecting a literal path at HEAD.
type FileHistoryResult struct {
	Commits []domain.CommitRecord
	More    bool
}

func fileReadError(reason string) error {
	return &domain.GitError{Code: domain.CommandFailed, Reason: reason}
}

func (r LocalReader) fileHead(ctx context.Context, repo domain.RepositoryContext, file string) (string, error) {
	if repo.Repository.RepositoryID == "" || strings.ContainsRune(repo.Repository.Path, 0) ||
		(repo.Repository.Location == "local" && !filepath.IsAbs(repo.Repository.Path)) ||
		(repo.Repository.Location == "ssh" && (r.Transport == nil || !path.IsAbs(repo.Repository.Path))) ||
		(repo.Repository.Location != "local" && repo.Repository.Location != "ssh") {
		return "", &domain.GitError{Code: domain.InvalidRepository}
	}
	if !validPath(file) {
		return "", &domain.GitError{Code: domain.UnsafeOperation}
	}
	out, err := r.run(ctx, repo.Repository.Path, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		var ge *domain.GitError
		if errors.As(err, &ge) && ge.Code == domain.CommandFailed {
			return "", fileReadError("No HEAD commit is available")
		}
		return "", err
	}
	head := strings.TrimSuffix(string(out.Stdout), "\n")
	if !oidPattern.MatchString(head) {
		return "", parseError()
	}
	return head, nil
}

// FileHistory returns at most 100 commits reachable from HEAD, without following renames.
func (r LocalReader) FileHistory(ctx context.Context, repo domain.RepositoryContext, file string) (FileHistoryResult, error) {
	var result FileHistoryResult
	r = r.fileReadBounded()
	head, err := r.fileHead(ctx, repo, file)
	if err != nil {
		return result, err
	}
	entry, err := r.run(ctx, repo.Repository.Path, "ls-tree", "-z", "--full-tree", head, "--", file)
	if err != nil {
		return result, err
	}
	fields, err := tokens(entry.Stdout)
	if err != nil {
		return result, err
	}
	if len(fields) != 1 {
		return result, fileReadError("File is not tracked at HEAD")
	}
	meta, name, found := strings.Cut(string(fields[0]), "\t")
	parts := strings.Fields(meta)
	if !found || name != file || len(parts) != 3 || parts[1] != "blob" || (parts[0] != "100644" && parts[0] != "100755") {
		return result, fileReadError("Path is not a regular file at HEAD")
	}
	out, err := r.run(ctx, repo.Repository.Path, "-c", "log.showSignature=false", "log", "--topo-order", "-z", "--format="+HistoryFormat, "--max-count=101", head, "--", file)
	if err != nil {
		return result, err
	}
	result.Commits, err = ParseHistory(out.Stdout)
	if err != nil {
		return FileHistoryResult{}, err
	}
	if len(result.Commits) == 0 {
		return result, fileReadError("No history for this path at HEAD; it may be untracked")
	}
	if len(result.Commits) > 100 {
		result.More = true
		result.Commits = result.Commits[:100]
	}
	return result, nil
}

func (r LocalReader) fileReadBounded() LocalReader {
	if r.OutputLimit <= 0 || r.OutputLimit > PreviewLimit {
		r.OutputLimit = PreviewLimit
	}
	r.Input = nil
	r.WriteMode = false
	return r
}

// FileBlame returns Git's textual blame for the exact file in the HEAD commit.
func (r LocalReader) FileBlame(ctx context.Context, repo domain.RepositoryContext, file string) (string, error) {
	r = r.fileReadBounded()
	head, err := r.fileHead(ctx, repo, file)
	if err != nil {
		return "", err
	}
	entry, err := r.run(ctx, repo.Repository.Path, "ls-tree", "-z", "--full-tree", head, "--", file)
	if err != nil {
		return "", err
	}
	fields, err := tokens(entry.Stdout)
	if err != nil {
		return "", err
	}
	if len(fields) != 1 {
		return "", fileReadError("File is not tracked at HEAD")
	}
	meta, name, found := strings.Cut(string(fields[0]), "\t")
	parts := strings.Fields(meta)
	if !found || name != file || len(parts) != 3 || parts[1] != "blob" || !oidPattern.MatchString(parts[2]) || (parts[0] != "100644" && parts[0] != "100755") {
		return "", fileReadError("File is not a regular tracked file at HEAD")
	}
	blob, err := r.run(ctx, repo.Repository.Path, "cat-file", "blob", parts[2])
	if err != nil {
		return "", err
	}
	if bytes.IndexByte(blob.Stdout, 0) >= 0 || !utf8.Valid(blob.Stdout) {
		return "", fileReadError("Binary file; blame is unavailable")
	}
	out, err := r.run(ctx, repo.Repository.Path, "blame", "--no-textconv", "--encoding=UTF-8", head, "--", file)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(out.Stdout) || bytes.IndexByte(out.Stdout, 0) >= 0 {
		return "", fileReadError("Blame output is not text")
	}
	limited := false
	text := displayText(out.Stdout, &limited)
	if limited {
		return "", &domain.GitError{Code: domain.OutputLimitExceeded}
	}
	return text, nil
}
