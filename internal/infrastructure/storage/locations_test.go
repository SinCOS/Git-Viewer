package storage_test

import (
	"fmt"
	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/storage"
	"os"
	"path/filepath"
	"testing"
)

func TestLocationPersistenceLimitAndCorruption(t *testing.T) {
	store := storage.Locations{Path: filepath.Join(t.TempDir(), "app", "locations.json")}
	for i := 0; i < 25; i++ {
		if _, err := store.Remember(domain.SavedLocation{Kind: "ssh", Host: "host", Path: fmt.Sprintf("/repo/%d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	recent, err := store.Remember(domain.SavedLocation{Kind: "workspace", Path: "/tmp/中文.code-workspace"})
	if err != nil || len(recent) != 20 || recent[0].Kind != "workspace" {
		t.Fatal(recent, err)
	}
	recent, err = store.Remember(recent[0])
	if err != nil || len(recent) != 20 {
		t.Fatal(recent, err)
	}
	for _, value := range []domain.SavedLocation{
		{Kind: "ssh-folder", Host: "host", Path: "/remote/folder"},
		{Kind: "ssh-workspace", Host: "host", Path: "/remote/project.code-workspace"},
	} {
		if _, err := store.Remember(value); err != nil {
			t.Fatal(value.Kind, err)
		}
	}
	for _, bad := range []domain.SavedLocation{{Kind: "ssh", Host: "-oProxyCommand=bad", Path: "/repo"}, {Kind: "workspace", Host: "host", Path: "/repo"}, {Kind: "ssh", Host: "host", Path: "relative"}} {
		if _, err = store.Remember(bad); err == nil {
			t.Fatal("unsafe saved location accepted")
		}
	}
	if err = os.WriteFile(store.Path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Remember(domain.SavedLocation{Kind: "workspace", Path: "/tmp/new"}); err == nil {
		t.Fatal("corrupt data replaced")
	}
	data, _ := os.ReadFile(store.Path)
	if string(data) != "broken" {
		t.Fatal("corruption overwritten")
	}
}
