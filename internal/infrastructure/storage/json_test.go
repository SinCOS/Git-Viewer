package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
)

func TestAtomicJSONAndNativeWatcher(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := SaveJSON(path, map[string]int{"version": 1}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	watcher, err := mygo.WatchFiles(ctx, path, mygo.FileWatchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer watcher.Close()
	if err := SaveJSON(path, map[string]int{"version": 2}); err != nil {
		t.Fatal(err)
	}
	events, err := watcher.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events {
		if event.Path == "." && event.Op == mygo.FileWrite {
			found = true
		}
	}
	if !found {
		t.Fatalf("atomic replacement not observed: %+v", events)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]int
	if err := json.Unmarshal(data, &value); err != nil || value["version"] != 2 {
		t.Fatal(string(data), err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal(info.Mode())
	}
	if err := SaveJSON(path, make(chan int)); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(data) {
		t.Fatal("failed save changed existing config", err)
	}
	temps, err := filepath.Glob(filepath.Join(dir, ".gitviewer-*.tmp"))
	if err != nil || len(temps) > 0 {
		t.Fatal("temporary files leaked", temps, err)
	}
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if err := SaveJSON(link, map[string]int{"version": 3}); err == nil {
		t.Fatal("symlink accepted")
	}
	if err := SaveJSON(dir, map[string]int{}); err == nil {
		t.Fatal("directory accepted")
	}
	if err := SaveJSON("relative.json", map[string]int{}); err == nil {
		t.Fatal("relative path accepted")
	}
	if err := watcher.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-watcher.Done():
	case <-ctx.Done():
		t.Fatal("watcher cleanup did not complete")
	}
	t.Log("mygo.WatchFiles observed atomic JSON replacement and released resources")
}
