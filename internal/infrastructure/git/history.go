package git

import (
	"context"
	"gitviewer/internal/domain"
	"strconv"
	"strings"
)

type HistoryPage struct {
	Commits []domain.CommitRecord
	Next    string
	Offset  int
	More    bool
}
type BranchRecord struct {
	Name     string
	OID      string
	Upstream string
	Current  bool
}

// History pins every page to the first page's object IDs so new commits do not
// shift subsequent pages. The internal cursor is not part of GitOperation DTO.
func (r LocalReader) History(ctx context.Context, repo domain.RepositoryContext, anchor string, offset, limit int) (HistoryPage, error) {
	p := HistoryPage{Offset: offset}
	if offset < 0 || limit < 1 || limit > 1000 {
		return p, &domain.GitError{Code: domain.UnsafeOperation}
	}
	if anchor == "" {
		refs, err := r.run(ctx, repo.Repository.Path, "for-each-ref", "--format=%(objecttype)%00%(objectname)%00%(*objecttype)%00%(*objectname)", "refs/heads/", "refs/remotes/", "refs/tags/")
		if err != nil {
			return p, err
		}
		ids := []string{}
		for _, line := range strings.Split(strings.TrimSuffix(string(refs.Stdout), "\n"), "\n") {
			if line == "" {
				continue
			}
			f := strings.Split(line, "\x00")
			if len(f) != 4 {
				return p, parseError()
			}
			switch {
			case f[0] == "commit":
				ids = append(ids, f[1])
			case f[0] == "tag" && f[2] == "commit":
				ids = append(ids, f[3])
			case f[0] == "tag" && f[2] == "tag":
				if !oidPattern.MatchString(f[1]) {
					return p, parseError()
				}
				peeled, e := r.run(ctx, repo.Repository.Path, "rev-parse", "--verify", f[1]+"^{commit}")
				if e == nil {
					ids = append(ids, strings.TrimSuffix(string(peeled.Stdout), "\n"))
				}
			}
		}
		head, headErr := r.run(ctx, repo.Repository.Path, "rev-parse", "--verify", "HEAD")
		if headErr == nil {
			ids = append([]string{strings.TrimSuffix(string(head.Stdout), "\n")}, ids...)
		} else if len(ids) == 0 {
			symbolic, e := r.run(ctx, repo.Repository.Path, "symbolic-ref", "-q", "HEAD")
			if e != nil || len(symbolic.Stdout) == 0 {
				return p, headErr
			}
			return p, nil
		}
		seen := map[string]bool{}
		unique := []string{}
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				unique = append(unique, id)
			}
		}
		anchor = strings.Join(unique, ",")
	}
	anchors := strings.Split(anchor, ",")
	if len(anchors) > 4096 {
		return p, &domain.GitError{Code: domain.OutputLimitExceeded}
	}
	for _, id := range anchors {
		if !oidPattern.MatchString(id) {
			return p, &domain.GitError{Code: domain.UnsafeOperation}
		}
	}
	args := []string{"log", "--topo-order", "-z", "--format=" + HistoryFormat, "--max-count=" + strconv.Itoa(limit+1), "--skip=" + strconv.Itoa(offset)}
	args = append(args, anchors...)
	args = append(args, "--")
	out, err := r.run(ctx, repo.Repository.Path, args...)
	if err != nil {
		return p, err
	}
	p.Commits, err = ParseHistory(out.Stdout)
	if err != nil {
		return p, err
	}
	p.More = len(p.Commits) > limit
	if p.More {
		p.Commits = p.Commits[:limit]
	}
	p.Next = anchor
	p.Offset = offset + len(p.Commits)
	return p, nil
}
func (r LocalReader) BranchList(ctx context.Context, repo domain.RepositoryContext) ([]BranchRecord, error) {
	out, err := r.run(ctx, repo.Repository.Path, "for-each-ref", "--format=%(refname)%00%(objectname)%00%(upstream)%00%(HEAD)", "refs/heads/", "refs/remotes/")
	if err != nil {
		return nil, err
	}
	result := []BranchRecord{}
	for _, line := range strings.Split(strings.TrimSuffix(string(out.Stdout), "\n"), "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\x00")
		if len(f) != 4 || !oidPattern.MatchString(f[1]) {
			return nil, parseError()
		}
		result = append(result, BranchRecord{f[0], f[1], f[2], f[3] == "*"})
	}
	return result, nil
}
func (r LocalReader) WorktreeList(ctx context.Context, repo domain.RepositoryContext) ([]domain.WorktreeRecord, error) {
	out, err := r.run(ctx, repo.Repository.Path, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return nil, err
	}
	return ParseWorktrees(out.Stdout)
}
