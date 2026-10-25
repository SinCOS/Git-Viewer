package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// SaveJSON is a Phase 0 persistence POC for app-owned configuration in a
// trusted directory. Workspace revision checks and JSONC editing belong to W.
func SaveJSON(path string, value any) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("configuration path must be absolute")
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("configuration target must be a regular file")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".gitviewer-*.tmp")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if _, err = temp.Write(append(data, '\n')); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
