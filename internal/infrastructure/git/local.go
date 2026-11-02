package git

import (
	"context"
	"path/filepath"
	"strconv"
	"strings"

	"gitviewer/internal/domain"
	"gitviewer/internal/gitargs"
	"gitviewer/internal/infrastructure/process"
)

const HistoryFormat = gitargs.HistoryFormat

// ReadArgs is shared by the local and SSH POCs. No free-form Git arguments.
func ReadArgs(command domain.GitCommand) ([]string, error) {
	r, op := command.Repository.Repository, command.Operation
	reject := func() ([]string, error) { return nil, &domain.GitError{Code: domain.UnsafeOperation} }
	if op.RepositoryID == "" || op.RepositoryID != r.RepositoryID || strings.ContainsRune(r.Path, 0) || r.Path == "" {
		return nil, &domain.GitError{Code: domain.InvalidRepository}
	}
	if len(op.Paths) > 0 || op.Ref != "" || op.Message != "" {
		return reject()
	}
	if op.Limit < 0 || op.Limit > 1000 || (op.Limit != 0 && op.Operation != domain.History) {
		return reject()
	}
	args := []string{"--no-pager", "--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "log.showSignature=false", "-c", "color.ui=false", "-C", r.Path}
	switch op.Operation {
	case domain.Status:
		args = append(args, "status", "--porcelain=v1", "-z", "--untracked-files=all")
	case domain.Diff:
		args = append(args, "diff", "--no-ext-diff", "--no-textconv", "--binary", "--")
	case domain.History:
		limit := op.Limit
		if limit == 0 {
			limit = 100
		}
		args = append(args, "log", "--topo-order", "-z", "--format="+HistoryFormat, "-n", strconv.Itoa(limit), "--")
	case domain.Worktrees:
		args = append(args, "worktree", "list", "--porcelain", "-z")
	case domain.Branches:
		args = append(args, "for-each-ref", "--format=%(refname)%00%(objectname)%00%(upstream)")
	default:
		return reject()
	}
	return args, nil
}

type LocalExecutor struct {
	GitPath     string
	OutputLimit int
}

var _ domain.GitExecutor = LocalExecutor{}

func (e LocalExecutor) Execute(ctx context.Context, command domain.GitCommand) (domain.GitResult, error) {
	args, err := ReadArgs(command)
	if err != nil {
		return domain.GitResult{ExitCode: -1}, err
	}
	r := command.Repository.Repository
	if r.Location != "local" || !filepath.IsAbs(r.Path) {
		return domain.GitResult{ExitCode: -1}, &domain.GitError{Code: domain.InvalidRepository}
	}
	executable := e.GitPath
	if executable == "" {
		executable = "git"
	}
	result, err := process.Run(ctx, command.Timeout, e.OutputLimit, executable, args, CleanEnvironment())
	return result, err
}

// CleanEnvironment removes inherited repository/config overrides; keep
// ordinary PATH, HOME and SSH agent access. Disable terminal prompting and
// pager interaction. Shared with the remote helper.
func CleanEnvironment() []string { return gitargs.CleanEnvironment() }
