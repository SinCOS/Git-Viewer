package workspace

import (
	"context"
	"crypto/sha256"
	"fmt"
	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
	"io"
	"os"
	"path/filepath"
	"sync"
)

const MaxDocumentSize = 4 * 1024 * 1024

var saveMu sync.Mutex

func Read(path string) (Document, error) {
	if !filepath.IsAbs(path) {
		return Document{}, ErrInvalid
	}
	info, err := os.Lstat(path)
	if err != nil {
		return Document{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxDocumentSize {
		return Document{}, ErrInvalid
	}
	f, err := os.Open(path)
	if err != nil {
		return Document{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, MaxDocumentSize+1))
	if err != nil {
		return Document{}, err
	}
	if len(data) > MaxDocumentSize {
		return Document{}, ErrInvalid
	}
	d, err := Parse(data)
	d.Revision = fmt.Sprintf("%x", sha256.Sum256(data))
	return d, err
}
func Save(path, revision string, folders []domain.WorkspaceFolder) (Document, error) {
	saveMu.Lock()
	defer saveMu.Unlock()
	d, err := Read(path)
	if err != nil {
		return Document{}, err
	}
	if revision == "" || revision != d.Revision {
		return Document{}, ErrConflict
	}
	data, err := d.Edit(folders)
	if info, e := os.Stat(path); e != nil {
		return Document{}, e
	} else if info.Mode().Perm()&0222 == 0 {
		return Document{}, os.ErrPermission
	}
	if err != nil {
		return Document{}, err
	}
	return replace(path, data, revision, false)
}
func Create(path string, folders []domain.WorkspaceFolder) (Document, error) {
	if !filepath.IsAbs(path) {
		return Document{}, ErrInvalid
	}
	base, err := Parse([]byte("{\n  \"folders\": []\n}\n"))
	if err != nil {
		return Document{}, err
	}
	data, err := base.Edit(folders)
	if err != nil {
		return Document{}, err
	}
	saveMu.Lock()
	defer saveMu.Unlock()
	return replace(path, data, "", true)
}
func replace(path string, data []byte, revision string, create bool) (Document, error) {
	temp, err := os.CreateTemp(filepath.Dir(path), ".gitviewer-workspace-*")
	if err != nil {
		return Document{}, err
	}
	name := temp.Name()
	defer os.Remove(name)
	if !create {
		info, err := os.Lstat(path)
		if err != nil {
			temp.Close()
			return Document{}, err
		}
		if err = temp.Chmod(info.Mode().Perm()); err != nil {
			temp.Close()
			return Document{}, err
		}
	}
	if _, err = temp.Write(data); err != nil {
		temp.Close()
		return Document{}, err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return Document{}, err
	}
	if err = temp.Close(); err != nil {
		return Document{}, err
	}
	if create {
		err = os.Link(name, path)
	} else {
		latest, e := Read(path)
		if e != nil {
			return Document{}, e
		}
		if latest.Revision != revision {
			return Document{}, ErrConflict
		}
		err = os.Rename(name, path)
	}
	if err != nil {
		return Document{}, err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return Document{}, err
	}
	err = dir.Sync()
	dir.Close()
	if err != nil {
		return Document{}, err
	}
	return Read(path)
}
func View(ctx context.Context, path string, d Document, reader gitexec.LocalReader) (domain.WorkspaceView, []domain.RepositoryContext, error) {
	view := domain.WorkspaceView{ID: fmt.Sprintf("workspace-%x", sha256.Sum256([]byte(path))), FilePath: path, Revision: d.Revision, Folders: []domain.WorkspaceFolderView{}}
	repos := []domain.RepositoryContext{}
	seen := map[string]bool{}
	for _, f := range d.Folders {
		if err := ctx.Err(); err != nil {
			return view, nil, err
		}
		resolved := f.Path
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(filepath.Dir(path), resolved)
		}
		row := domain.WorkspaceFolderView{Path: f.Path, Name: f.Name, ResolvedPath: resolved, Diagnostics: []string{}}
		canonical, err := filepath.EvalSymlinks(resolved)
		if err != nil {
			row.Diagnostics = append(row.Diagnostics, "Directory unavailable")
		} else {
			row.ResolvedPath = canonical
			repo, err := reader.Open(ctx, canonical)
			if err != nil {
				row.Diagnostics = append(row.Diagnostics, "Not an accessible Git working tree")
			} else {
				row.RepositoryID = repo.Repository.RepositoryID
				if !seen[row.RepositoryID] {
					seen[row.RepositoryID] = true
					repos = append(repos, repo)
				}
			}
		}
		view.Folders = append(view.Folders, row)
	}
	return view, repos, nil
}
