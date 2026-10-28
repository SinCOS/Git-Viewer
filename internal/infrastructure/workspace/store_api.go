package workspace

import (
	"context"

	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
)

// FolderOpener resolves one workspace folder and reports whether it is a Git
// working tree. A nil repository means the folder is still editable.
type FolderOpener func(context.Context, string) (*domain.RepositoryContext, error)

// OpenResult is the complete view state produced by a workspace store.
type OpenResult struct {
	Document     Document
	View         domain.WorkspaceView
	Repositories []domain.RepositoryContext
	Roots        []domain.EditorRoot
}

// Store keeps workspace parsing and persistence replaceable for local and
// remote locations.
type Store interface {
	Read(context.Context, string) (Document, error)
	Save(context.Context, string, string, []domain.WorkspaceFolder) (Document, error)
	Create(context.Context, string, []domain.WorkspaceFolder) (Document, error)
	Open(context.Context, string, FolderOpener) (OpenResult, error)
}

type LocalStore struct {
	Reader gitexec.LocalReader
}

func (s LocalStore) Read(_ context.Context, path string) (Document, error) {
	return Read(path)
}

func (s LocalStore) Save(_ context.Context, path, revision string, folders []domain.WorkspaceFolder) (Document, error) {
	return Save(path, revision, folders)
}

func (s LocalStore) Create(_ context.Context, path string, folders []domain.WorkspaceFolder) (Document, error) {
	return Create(path, folders)
}

func (s LocalStore) Open(ctx context.Context, path string, open FolderOpener) (OpenResult, error) {
	d, err := Read(path)
	if err != nil {
		return OpenResult{}, err
	}
	view, repos, err := View(ctx, path, d, s.Reader)
	if err != nil {
		return OpenResult{}, err
	}
	roots := make([]domain.EditorRoot, 0, len(view.Folders))
	for _, folder := range view.Folders {
		roots = append(roots, domain.EditorRoot{
			Name: folder.Name, Location: domain.LocationLocal, Path: folder.ResolvedPath, RepositoryID: folder.RepositoryID,
		})
	}
	return OpenResult{Document: d, View: view, Repositories: repos, Roots: roots}, nil
}
