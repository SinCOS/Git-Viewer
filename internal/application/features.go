package application

import (
	"context"
	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/git"
	"gitviewer/internal/infrastructure/storage"
)

func (s LocalService) Locations() storage.Locations {
	return storage.Locations{Path: s.Recent.Path + ".locations.json"}
}
func (s LocalService) ReaderFor(repo domain.RepositoryContext) git.LocalReader {
	if repo.Repository.Location == "ssh" {
		return s.Remote.Reader(repo.SSHHost)
	}
	return s.Reader
}
func (s LocalService) OpenSSH(ctx context.Context, host, path string) (domain.RepositoryContext, error) {
	return s.Remote.Open(ctx, host, path)
}
func (s LocalService) CommitDetails(ctx context.Context, repo domain.RepositoryContext, oid string) (git.CommitDetails, error) {
	return s.ReaderFor(repo).CommitDetails(ctx, repo, oid)
}
func (s LocalService) History(ctx context.Context, repo domain.RepositoryContext, anchor string, offset int) (git.HistoryPage, error) {
	return s.ReaderFor(repo).History(ctx, repo, anchor, offset, 100)
}
func (s LocalService) Branches(ctx context.Context, repo domain.RepositoryContext) ([]git.BranchRecord, error) {
	return s.ReaderFor(repo).BranchList(ctx, repo)
}
func (s LocalService) Worktrees(ctx context.Context, repo domain.RepositoryContext) ([]domain.WorktreeRecord, error) {
	return s.ReaderFor(repo).WorktreeList(ctx, repo)
}
func (s LocalService) Write(ctx context.Context, repo domain.RepositoryContext, op domain.GitOperation) (git.WriteResult, error) {
	return s.ReaderFor(repo).Write(ctx, repo, op)
}
