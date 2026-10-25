package storage

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"gitviewer/internal/domain"
)

const RecentLimit = 20

var recentWrites sync.Mutex

// RecentRepositories stores only the existing Repository DTO, never credentials.
type RecentRepositories struct{ Path string }

func (s RecentRepositories) Load() ([]domain.Repository, error) {
	f, err := os.Open(s.Path)
	if os.IsNotExist(err) {
		return []domain.Repository{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read recent repositories")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 128*1024 {
		return nil, fmt.Errorf("invalid recent repositories")
	}
	var repos []domain.Repository
	dec := json.NewDecoder(io.LimitReader(f, 128*1024))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&repos); err != nil {
		return nil, fmt.Errorf("invalid recent repositories")
	}
	var extra any
	if dec.Decode(&extra) != io.EOF || len(repos) > RecentLimit {
		return nil, fmt.Errorf("invalid recent repositories")
	}
	seen := map[string]bool{}
	for _, repo := range repos {
		if repo.Location != "local" || !filepath.IsAbs(repo.Path) || repo.RepositoryID == "" || repo.DisplayName == "" || seen[repo.RepositoryID] {
			return nil, fmt.Errorf("invalid recent repositories")
		}
		seen[repo.RepositoryID] = true
	}
	if repos == nil {
		return nil, fmt.Errorf("invalid recent repositories")
	}
	return repos, nil
}
func (s RecentRepositories) Remember(repo domain.Repository) ([]domain.Repository, error) {
	recentWrites.Lock()
	defer recentWrites.Unlock()
	if repo.Location != "local" || !filepath.IsAbs(repo.Path) || repo.RepositoryID == "" || repo.DisplayName == "" {
		return nil, fmt.Errorf("invalid recent repository")
	}
	repos, err := s.Load()
	if err != nil {
		return nil, err
	}
	next := []domain.Repository{repo}
	for _, old := range repos {
		if old.RepositoryID != repo.RepositoryID {
			next = append(next, old)
		}
	}
	if len(next) > RecentLimit {
		next = next[:RecentLimit]
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		return nil, fmt.Errorf("cannot create application configuration directory")
	}
	if err := SaveJSON(s.Path, next); err != nil {
		return nil, fmt.Errorf("cannot save recent repositories")
	}
	return next, nil
}
