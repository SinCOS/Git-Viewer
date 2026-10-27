package application

import (
	"context"
	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/git"
)

func (s LocalService) WorktreeStatus(ctx context.Context, repo domain.RepositoryContext, path string) (domain.GitStatus, error) {
	return s.ReaderFor(repo).WorktreeStatus(ctx, repo, path)
}
func (s LocalService) CreateWorktree(ctx context.Context, repo domain.RepositoryContext, path, branch string) (git.WorktreeWriteResult, error) {
	return s.ReaderFor(repo).CreateWorktree(ctx, repo, path, branch)
}
func (s LocalService) RemoveWorktree(ctx context.Context, repo domain.RepositoryContext, path string) (git.WorktreeWriteResult, error) {
	return s.ReaderFor(repo).RemoveWorktree(ctx, repo, path)
}
