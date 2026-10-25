package application

import (
	"context"

	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/git"
	"gitviewer/internal/infrastructure/ssh"
	"gitviewer/internal/infrastructure/storage"
)

type LocalService struct {
	Reader git.LocalReader
	Recent storage.RecentRepositories
	Remote ssh.Executor
}
type OpenResult struct {
	Repository       domain.RepositoryContext
	Recent           []domain.Repository
	PersistenceError bool
}

func (s LocalService) Open(ctx context.Context, path string) (OpenResult, error) {
	repo, err := s.Reader.Open(ctx, path)
	if err != nil {
		return OpenResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return OpenResult{}, &domain.GitError{Code: domain.OperationCancelled}
	}
	recent, err := s.Recent.Remember(repo.Repository)
	return OpenResult{Repository: repo, Recent: recent, PersistenceError: err != nil}, nil
}
func (s LocalService) Status(ctx context.Context, repo domain.RepositoryContext) (domain.GitStatus, error) {
	return s.ReaderFor(repo).Status(ctx, repo)
}
func (s LocalService) Diff(ctx context.Context, repo domain.RepositoryContext, request domain.DiffRequest) (domain.DiffPreview, error) {
	return s.ReaderFor(repo).Diff(ctx, repo, request)
}
