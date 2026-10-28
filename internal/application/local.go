package application

import (
	"context"
	"errors"
	"path"
	"strings"

	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/git"
	"gitviewer/internal/infrastructure/ssh"
	"gitviewer/internal/infrastructure/sshfs"
	"gitviewer/internal/infrastructure/storage"
	"gitviewer/internal/infrastructure/workspace"
)

type LocalService struct {
	Reader git.LocalReader
	Recent storage.RecentRepositories
	Remote ssh.Executor
}

type RemoteOpenResult struct {
	Root       domain.EditorRoot
	Repository *domain.RepositoryContext
	Files      *sshfs.Client
}

type RemoteWorkspaceOpenResult struct {
	Document     workspace.Document
	View         domain.WorkspaceView
	Repositories []domain.RepositoryContext
	Roots        []domain.EditorRoot
	Files        map[string]*sshfs.Client
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

// OpenRemote opens a remote folder as an editor root and, when possible, its
// Git repository. A non-Git folder remains usable; transport and host-key
// errors are returned instead of being hidden.
func (s LocalService) OpenRemote(ctx context.Context, host, dir string) (RemoteOpenResult, error) {
	if !ssh.ValidHost(host) || !path.IsAbs(dir) || strings.ContainsRune(dir, 0) {
		return RemoteOpenResult{}, &domain.GitError{Code: domain.InvalidRepository}
	}
	files, err := sshfs.Open(ctx, sshfs.Config{
		SSHPath: s.Remote.SSHPath, ConfigPath: s.Remote.ConfigPath, KnownHostsFile: s.Remote.KnownHostsFile,
	}, host, dir)
	if err != nil {
		return RemoteOpenResult{}, err
	}
	canonical, err := files.Canonical(ctx, files.Root())
	if err != nil {
		_ = files.Close()
		return RemoteOpenResult{}, err
	}
	root := domain.EditorRoot{Name: path.Base(canonical), Location: domain.LocationSSH, SSHHost: host, Path: canonical}
	result := RemoteOpenResult{Root: root, Files: files}
	repo, repoErr := s.Remote.Open(ctx, host, canonical)
	if repoErr == nil {
		root.RepositoryID = repo.Repository.RepositoryID
		result.Root = root
		result.Repository = &repo
		return result, nil
	}
	var ge *domain.GitError
	if errors.As(repoErr, &ge) && (ge.Code == domain.InvalidRepository || ge.Code == domain.GitUnavailable || ge.Code == domain.UnsupportedGitVersion) {
		return result, nil
	}
	_ = files.Close()
	return RemoteOpenResult{}, repoErr
}

func (s LocalService) RemoteInfo(ctx context.Context, host, file string) (domain.RemoteFileInfo, error) {
	if !ssh.ValidHost(host) || !path.IsAbs(file) || strings.ContainsRune(file, 0) {
		return domain.RemoteFileInfo{}, &domain.GitError{Code: domain.InvalidRepository}
	}
	client, err := sshfs.Open(ctx, sshfs.Config{
		SSHPath: s.Remote.SSHPath, ConfigPath: s.Remote.ConfigPath, KnownHostsFile: s.Remote.KnownHostsFile,
	}, host, file)
	if err != nil {
		return domain.RemoteFileInfo{}, err
	}
	defer client.Close()
	return client.Stat(ctx, file)
}

func (s LocalService) CanonicalRemotePath(ctx context.Context, host, file string) (string, error) {
	if !ssh.ValidHost(host) || !path.IsAbs(file) || strings.ContainsRune(file, 0) {
		return "", &domain.GitError{Code: domain.InvalidRepository}
	}
	client, err := sshfs.Open(ctx, sshfs.Config{
		SSHPath: s.Remote.SSHPath, ConfigPath: s.Remote.ConfigPath, KnownHostsFile: s.Remote.KnownHostsFile,
	}, host, file)
	if err != nil {
		return "", err
	}
	defer client.Close()
	return client.Canonical(ctx, file)
}

func (s LocalService) remoteWorkspaceClient(ctx context.Context, host, file string) (*sshfs.Client, error) {
	if !ssh.ValidHost(host) || !path.IsAbs(file) || strings.ContainsRune(file, 0) {
		return nil, &domain.GitError{Code: domain.InvalidRepository}
	}
	return sshfs.Open(ctx, sshfs.Config{
		SSHPath: s.Remote.SSHPath, ConfigPath: s.Remote.ConfigPath, KnownHostsFile: s.Remote.KnownHostsFile,
	}, host, path.Dir(file))
}

func (s LocalService) remoteWorkspaceOpen(ctx context.Context, host, file string, store workspace.Store, files map[string]*sshfs.Client) (RemoteWorkspaceOpenResult, error) {
	opened, err := store.Open(ctx, file, func(ctx context.Context, root string) (*domain.RepositoryContext, error) {
		result, openErr := s.OpenRemote(ctx, host, root)
		if openErr != nil {
			var ge *domain.GitError
			if errors.As(openErr, &ge) && ge.Code == domain.InvalidRepository {
				return nil, openErr
			}
			return nil, openErr
		}
		files[result.Root.Path] = result.Files
		return result.Repository, nil
	})
	if err != nil {
		for _, client := range files {
			_ = client.Close()
		}
		return RemoteWorkspaceOpenResult{}, err
	}
	return RemoteWorkspaceOpenResult{
		Document: opened.Document, View: opened.View, Repositories: opened.Repositories, Roots: opened.Roots, Files: files,
	}, nil
}

func (s LocalService) OpenRemoteWorkspace(ctx context.Context, host, file string) (RemoteWorkspaceOpenResult, error) {
	client, err := s.remoteWorkspaceClient(ctx, host, file)
	if err != nil {
		return RemoteWorkspaceOpenResult{}, err
	}
	defer client.Close()
	store := workspace.RemoteStore{Host: host, Files: client}
	return s.remoteWorkspaceOpen(ctx, host, file, store, map[string]*sshfs.Client{})
}

func (s LocalService) SaveRemoteWorkspace(ctx context.Context, host, file, revision string, folders []domain.WorkspaceFolder, create bool) (RemoteWorkspaceOpenResult, error) {
	client, err := s.remoteWorkspaceClient(ctx, host, file)
	if err != nil {
		return RemoteWorkspaceOpenResult{}, err
	}
	defer client.Close()
	store := workspace.RemoteStore{Host: host, Files: client}
	if create {
		_, err = store.Create(ctx, file, folders)
	} else {
		_, err = store.Save(ctx, file, revision, folders)
	}
	if err != nil {
		return RemoteWorkspaceOpenResult{}, err
	}
	return s.remoteWorkspaceOpen(ctx, host, file, store, map[string]*sshfs.Client{})
}
