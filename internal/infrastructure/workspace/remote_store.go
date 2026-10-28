package workspace

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"gitviewer/internal/domain"
)

// RemoteStore persists a .code-workspace file through a remote filesystem
// rooted at the workspace file's directory.
type RemoteStore struct {
	Host  string
	Files domain.RemoteFileSystem
}

func (s RemoteStore) Read(ctx context.Context, file string) (Document, error) {
	if err := ctx.Err(); err != nil {
		return Document{}, &domain.GitError{Code: domain.OperationCancelled}
	}
	if !path.IsAbs(file) || strings.ContainsRune(file, 0) {
		return Document{}, ErrInvalid
	}
	info, err := s.Files.Stat(ctx, file)
	if err != nil {
		return Document{}, err
	}
	if info.Dir || info.Symlink || info.Size > MaxDocumentSize {
		return Document{}, ErrInvalid
	}
	data, err := s.Files.ReadFile(ctx, file, MaxDocumentSize)
	if err != nil {
		return Document{}, err
	}
	d, err := Parse(data)
	d.Revision = fmt.Sprintf("%x", sha256.Sum256(data))
	return d, err
}

func (s RemoteStore) Save(ctx context.Context, file, revision string, folders []domain.WorkspaceFolder) (Document, error) {
	current, err := s.Read(ctx, file)
	if err != nil {
		return Document{}, err
	}
	if revision == "" || revision != current.Revision {
		return Document{}, ErrConflict
	}
	info, err := s.Files.Stat(ctx, file)
	if err != nil {
		return Document{}, err
	}
	if info.Mode.Perm()&0o222 == 0 {
		return Document{}, os.ErrPermission
	}
	data, err := current.Edit(folders)
	if err != nil {
		return Document{}, err
	}
	if err := s.Files.WriteFileAtomic(ctx, file, data, info.Mode.Perm()); err != nil {
		return Document{}, err
	}
	return s.Read(ctx, file)
}

func (s RemoteStore) Create(ctx context.Context, file string, folders []domain.WorkspaceFolder) (Document, error) {
	if !path.IsAbs(file) || strings.ContainsRune(file, 0) {
		return Document{}, ErrInvalid
	}
	if _, err := s.Files.Stat(ctx, file); err == nil {
		return Document{}, os.ErrExist
	}
	base, err := Parse([]byte("{\n  \"folders\": []\n}\n"))
	if err != nil {
		return Document{}, err
	}
	data, err := base.Edit(folders)
	if err != nil {
		return Document{}, err
	}
	if err := s.Files.WriteFileAtomic(ctx, file, data, 0o644); err != nil {
		return Document{}, err
	}
	return s.Read(ctx, file)
}

func (s RemoteStore) Open(ctx context.Context, file string, open FolderOpener) (OpenResult, error) {
	d, err := s.Read(ctx, file)
	if err != nil {
		return OpenResult{}, err
	}
	view := domain.WorkspaceView{
		ID:       fmt.Sprintf("workspace-%x", sha256.Sum256([]byte(s.Host+"\x00"+file))),
		FilePath: file,
		Location: string(domain.LocationSSH),
		SSHHost:  s.Host,
		Revision: d.Revision,
		Folders:  []domain.WorkspaceFolderView{},
	}
	var repos []domain.RepositoryContext
	var roots []domain.EditorRoot
	seenRepos := map[string]bool{}
	seenRoots := map[string]bool{}
	for _, folder := range d.Folders {
		resolved := folder.Path
		if !path.IsAbs(resolved) {
			resolved = path.Join(path.Dir(file), resolved)
		}
		resolved = path.Clean(resolved)
		if seenRoots[resolved] {
			continue
		}
		seenRoots[resolved] = true
		row := domain.WorkspaceFolderView{
			Path: folder.Path, Name: folder.Name, ResolvedPath: resolved,
			Location: string(domain.LocationSSH), SSHHost: s.Host, Diagnostics: []string{},
		}
		repo, openErr := open(ctx, resolved)
		if openErr != nil {
			var ge *domain.GitError
			if !errors.As(openErr, &ge) || ge.Code != domain.InvalidRepository {
				return OpenResult{}, openErr
			}
			row.Diagnostics = append(row.Diagnostics, "Remote directory unavailable")
		} else {
			name := folder.Name
			if name == "" {
				name = path.Base(resolved)
			}
			root := domain.EditorRoot{Name: name, Location: domain.LocationSSH, SSHHost: s.Host, Path: resolved}
			if repo != nil {
				root.RepositoryID = repo.Repository.RepositoryID
				row.RepositoryID = repo.Repository.RepositoryID
				if !seenRepos[root.RepositoryID] {
					seenRepos[root.RepositoryID] = true
					repos = append(repos, *repo)
				}
			}
			roots = append(roots, root)
		}
		view.Folders = append(view.Folders, row)
	}
	return OpenResult{Document: d, View: view, Repositories: repos, Roots: roots}, nil
}
