package application

import (
	"context"

	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/git"
)

func (s LocalService) FileHistory(ctx context.Context, repo domain.RepositoryContext, path string) (git.FileHistoryResult, error) {
	return s.ReaderFor(repo).FileHistory(ctx, repo, path)
}

func (s LocalService) FileBlame(ctx context.Context, repo domain.RepositoryContext, path string) (string, error) {
	return s.ReaderFor(repo).FileBlame(ctx, repo, path)
}
