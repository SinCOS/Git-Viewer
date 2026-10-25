package storage

import (
	"encoding/json"
	"errors"
	"gitviewer/internal/domain"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var alias = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

type Locations struct{ Path string }

func validLocation(l domain.SavedLocation) bool {
	return !strings.ContainsRune(l.Path, 0) && (l.Kind == "ssh" && alias.MatchString(l.Host) && path.IsAbs(l.Path) || l.Kind == "workspace" && l.Host == "" && filepath.IsAbs(l.Path))
}
func (s Locations) Load() ([]domain.SavedLocation, error) {
	f, err := os.Open(s.Path)
	if os.IsNotExist(err) {
		return []domain.SavedLocation{}, nil
	}
	if err != nil {
		return nil, errors.New("cannot read saved locations")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 128*1024 {
		return nil, errors.New("invalid saved locations")
	}
	data, err := io.ReadAll(io.LimitReader(f, 128*1024+1))
	if err != nil || len(data) > 128*1024 {
		return nil, errors.New("invalid saved locations")
	}
	var values []domain.SavedLocation
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if dec.Decode(&values) != nil || values == nil || len(values) > RecentLimit {
		return nil, errors.New("invalid saved locations")
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		return nil, errors.New("invalid saved locations")
	}
	seen := map[domain.SavedLocation]bool{}
	for _, v := range values {
		if !validLocation(v) || seen[v] {
			return nil, errors.New("invalid saved locations")
		}
		seen[v] = true
	}
	return values, nil
}
func (s Locations) Remember(value domain.SavedLocation) ([]domain.SavedLocation, error) {
	recentWrites.Lock()
	defer recentWrites.Unlock()
	if !validLocation(value) {
		return nil, errors.New("invalid saved location")
	}
	old, err := s.Load()
	if err != nil {
		return nil, err
	}
	next := []domain.SavedLocation{value}
	for _, v := range old {
		if v != value {
			next = append(next, v)
		}
	}
	if len(next) > RecentLimit {
		next = next[:RecentLimit]
	}
	if err = os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		return nil, err
	}
	if err = SaveJSON(s.Path, next); err != nil {
		return nil, err
	}
	return next, nil
}
