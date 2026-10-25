package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gitviewer/internal/domain"
)

func TestRecentSchemaAndBound(t *testing.T) {
	compiler := jsonschema.NewCompiler()
	for _, name := range []string{"repository", "recent-repositories"} {
		data, err := os.ReadFile(filepath.Join("../../../docs/features/git-viewer/schemas", name+".schema.json"))
		if err != nil {
			t.Fatal(err)
		}
		var spec any
		if err := json.Unmarshal(data, &spec); err != nil {
			t.Fatal(err)
		}
		if err := compiler.AddResource(name+".schema.json", spec); err != nil {
			t.Fatal(err)
		}
	}
	schema, err := compiler.Compile("recent-repositories.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	store := RecentRepositories{Path: filepath.Join(t.TempDir(), "recent.json")}
	for i := 0; i < RecentLimit+2; i++ {
		id := string(rune('a' + i))
		if _, err := store.Remember(domain.Repository{RepositoryID: id, Location: "local", Path: "/tmp/" + id, DisplayName: id}); err != nil {
			t.Fatal(err)
		}
	}
	repos, err := store.Load()
	if err != nil || len(repos) != RecentLimit || repos[0].RepositoryID != "v" {
		t.Fatalf("%+v %v", repos, err)
	}
	raw, err := os.ReadFile(store.Path)
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(value); err != nil {
		t.Fatal(err)
	}
	value.([]any)[0].(map[string]any)["location"] = "ssh"
	if schema.Validate(value) == nil {
		t.Fatal("SSH accepted in local recents")
	}
	if err := os.WriteFile(store.Path, []byte("null"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil {
		t.Fatal("null accepted as recent array")
	}
}
