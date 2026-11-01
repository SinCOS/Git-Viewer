package domain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestJSONSchemaContracts(t *testing.T) {
	old := "before\n中文"
	cases := []struct {
		name  string
		value any
		typ   reflect.Type
	}{
		{"workspace", WorkspaceDocument{Folders: []WorkspaceFolder{{Path: "../中文", Name: "Example"}}}, reflect.TypeOf(WorkspaceDocument{})},
		{"workspace-view", WorkspaceView{ID: "workspace-id", FilePath: "/tmp/example.code-workspace", Revision: "hash", Folders: []WorkspaceFolderView{{Path: ".", ResolvedPath: "/tmp", Diagnostics: []string{}}}}, reflect.TypeOf(WorkspaceView{})},
		{"saved-location", SavedLocation{Kind: "ssh", Host: "fixture", Path: "/tmp/repo"}, reflect.TypeOf(SavedLocation{})},
		{"diff-preview", DiffPreview{RepositoryID: "repo", Scope: WorktreeDiff, Path: "中文 space\nfile", Text: "+new\n"}, reflect.TypeOf(DiffPreview{})},
		{"repository", Repository{"repo", "ssh", "/tmp/中文 path", "Sample"}, reflect.TypeOf(Repository{})},
		{"git-operation", GitOperation{"repo", History, []string{"-file\n"}, "HEAD", "", "message", 100, ""}, reflect.TypeOf(GitOperation{})},
		{"git-status", GitStatus{"repo", "2026-10-25T00:00:00Z", false, []Change{{"after", "R", "M", false, false, &old}}}, reflect.TypeOf(GitStatus{})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join("../../docs/features/git-viewer/schemas", tc.name+".schema.json")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var doc any
			err = json.Unmarshal(raw, &doc)
			if err != nil {
				t.Fatal(err)
			}
			compiler := jsonschema.NewCompiler()
			if err := compiler.AddResource("contract.json", doc); err != nil {
				t.Fatal(err)
			}
			schema, err := compiler.Compile("contract.json")
			if err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatal(err)
			}
			var value map[string]any
			if err := json.Unmarshal(data, &value); err != nil {
				t.Fatal(err)
			}
			if err := schema.Validate(value); err != nil {
				t.Fatal(err)
			}
			var spec struct {
				Properties map[string]json.RawMessage
				Required   []string
			}
			if err := json.Unmarshal(raw, &spec); err != nil {
				t.Fatal(err)
			}
			fields := map[string]bool{}
			for i := 0; i < tc.typ.NumField(); i++ {
				tag := tc.typ.Field(i).Tag.Get("json")
				for j, ch := range tag {
					if ch == ',' {
						tag = tag[:j]
						break
					}
				}
				fields[tag] = true
			}
			if len(fields) != len(spec.Properties) {
				t.Fatalf("Go/schema field count differs: %v / %v", fields, spec.Properties)
			}
			for key := range spec.Properties {
				if !fields[key] {
					t.Errorf("missing Go field %s", key)
				}
			}
			for _, required := range spec.Required {
				saved := value[required]
				delete(value, required)
				if schema.Validate(value) == nil {
					t.Errorf("missing %s accepted", required)
				}
				value[required] = saved
			}
			if tc.name == "git-operation" {
				value["operation"] = "shell"
				if schema.Validate(value) == nil {
					t.Error("unknown operation accepted")
				}
				value["operation"] = "history"
				value["limit"] = 1001
				if schema.Validate(value) == nil {
					t.Error("invalid limit accepted")
				}
				for _, op := range []Operation{Status, Diff, History, Branches, Worktrees, Graph, Stage, Unstage, StageHunk, UnstageHunk, DiscardHunk, Discard, Commit, UndoCommit, StashPush, StashUntracked, StashPop, Amend, SwitchBranch, CreateBranch, CherryPick, Revert, Fetch, FetchPrune, FetchAll, PullFFOnly, PullRebase, PullFrom, Push, PushTo, Sync} {
					value["operation"] = string(op)
					value["limit"] = 1
					if err := schema.Validate(value); err != nil {
						t.Errorf("%s: %v", op, err)
					}
				}
			}
			if tc.name == "git-status" {
				row := value["changes"].([]any)[0].(map[string]any)
				for _, key := range []string{"path", "indexStatus", "worktreeStatus", "untracked", "conflicted"} {
					v := row[key]
					delete(row, key)
					if schema.Validate(value) == nil {
						t.Errorf("missing change.%s accepted", key)
					}
					row[key] = v
				}
			}
			if tc.name == "saved-location" {
				for _, saved := range []SavedLocation{
					{Kind: "ssh-folder", Host: "fixture", Path: "/tmp/folder"},
					{Kind: "ssh-workspace", Host: "fixture", Path: "/tmp/example.code-workspace"},
				} {
					data, err := json.Marshal(saved)
					if err != nil {
						t.Fatal(err)
					}
					var candidate map[string]any
					if err := json.Unmarshal(data, &candidate); err != nil {
						t.Fatal(err)
					}
					if err := schema.Validate(candidate); err != nil {
						t.Fatalf("%s: %v", saved.Kind, err)
					}
				}
			}
		})
	}
}
