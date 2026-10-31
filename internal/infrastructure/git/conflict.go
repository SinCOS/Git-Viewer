package git

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"gitviewer/internal/domain"
)

// ConflictFile reads the base, ours and theirs stages without touching the
// index or worktree. The application layer adds the current worktree result.
func (r LocalReader) ConflictFile(ctx context.Context, repo domain.RepositoryContext, path string) (domain.ConflictFile, error) {
	conflict := domain.ConflictFile{RepositoryID: repo.Repository.RepositoryID, Path: path}
	if repo.Repository.RepositoryID == "" || !validPath(path) {
		return conflict, &domain.GitError{Code: domain.UnsafeOperation}
	}
	result, err := r.run(ctx, repo.Repository.Path, "ls-files", "--unmerged", "-z", "--", path)
	if err != nil {
		return conflict, err
	}
	stages := map[string]string{}
	for _, token := range strings.Split(string(result.Stdout), "\x00") {
		if token == "" {
			continue
		}
		meta, name, ok := strings.Cut(token, "\t")
		if !ok || name != path {
			return conflict, parseError()
		}
		fields := strings.Fields(meta)
		if len(fields) != 3 || !oidPattern.MatchString(fields[1]) || fields[2] < "1" || fields[2] > "3" {
			return conflict, parseError()
		}
		stages[fields[2]] = fields[1]
	}
	if len(stages) == 0 {
		return conflict, &domain.GitError{Code: domain.UnsafeOperation, Reason: "The selected path has no unresolved merge stages"}
	}
	read := func(stage string) (string, bool, error) {
		oid, ok := stages[stage]
		if !ok {
			return "", false, nil
		}
		text, available, err := r.readConflictBlob(ctx, repo, oid)
		if err != nil {
			return "", false, err
		}
		if !available {
			conflict.Binary = true
		}
		return text, available, nil
	}
	conflict.Base, conflict.BaseAvailable, err = read("1")
	if err != nil {
		return conflict, err
	}
	conflict.Ours, conflict.OursAvailable, err = read("2")
	if err != nil {
		return conflict, err
	}
	conflict.Theirs, conflict.TheirsAvailable, err = read("3")
	if err != nil {
		return conflict, err
	}
	return conflict, nil
}

func (r LocalReader) readConflictBlob(ctx context.Context, repo domain.RepositoryContext, oid string) (string, bool, error) {
	bounded := r
	if bounded.OutputLimit <= 0 || bounded.OutputLimit > PreviewLimit {
		bounded.OutputLimit = PreviewLimit
	}
	result, err := bounded.run(ctx, repo.Repository.Path, "cat-file", "blob", oid)
	if err != nil {
		var ge *domain.GitError
		if errors.As(err, &ge) && ge.Code == domain.OutputLimitExceeded {
			return "", false, nil
		}
		return "", false, err
	}
	if !utf8.Valid(result.Stdout) || bytes.IndexByte(result.Stdout, 0) >= 0 {
		return "", false, nil
	}
	return string(result.Stdout), true, nil
}
