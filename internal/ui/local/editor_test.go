package local_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestEditorTabsOpenSaveClose(t *testing.T) {
	a, _ := setup(t)
	path := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.OpenEditorFile(path); err != nil {
		t.Fatal(err)
	}
	if len(a.EditorTabs) != 1 || a.EditorActive != 0 || a.Mode != "editor" {
		t.Fatal(len(a.EditorTabs), a.EditorActive, a.Mode)
	}
	text := "package main\n\nfunc main() {}\n"
	buffer := a.ActiveEditorBuffer()
	buffer.Set(text)
	if !buffer.Modified() {
		t.Fatal("edited buffer should be dirty")
	}
	if err := a.SaveActiveEditorFile(); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil || string(saved) != text {
		t.Fatalf("save mismatch: %q %v", saved, err)
	}
	if buffer.Modified() {
		t.Fatal("saved buffer should be clean")
	}
	if err := a.OpenEditorFile(path); err != nil || len(a.EditorTabs) != 1 {
		t.Fatal("reopen should reuse the tab", err, len(a.EditorTabs))
	}
	a.CloseEditorTab(0)
	if len(a.EditorTabs) != 0 {
		t.Fatal(len(a.EditorTabs))
	}
}

func TestOpenEditorFileDialogReturnsBeforeBlocking(t *testing.T) {
	a, queue := setup(t)
	path := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.ChooseFile = func() (string, error) { return path, nil }
	tt := ui.NewTester(a.View, 1200, 800)
	a.Pane = "Editor"
	tt.Frame()
	done := make(chan struct{})
	go func() {
		_ = tt.Click("Open file…")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("open-file click blocked the UI thread")
	}
	select {
	case apply := <-queue:
		apply()
	case <-time.After(5 * time.Second):
		t.Fatal("dialog result was not dispatched")
	}
	if len(a.EditorTabs) != 1 {
		t.Fatal("file was not opened", len(a.EditorTabs))
	}
}

func TestEditorViewMounts(t *testing.T) {
	a, _ := setup(t)
	path := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.OpenEditorFile(path); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(a.View, 1200, 800)
	for _, label := range []string{"Editor workspace", "Editor tabs", "OPEN EDITORS", "Save"} {
		if _, ok := tt.Find(label); !ok {
			t.Fatal("missing", label)
		}
	}
}
