package local_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/testgit"
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

func TestSaveEditorRefreshesSourceControl(t *testing.T) {
	dir := testgit.Repo(t)
	path := filepath.Join(dir, "main.go")
	testgit.Write(t, path, "package main\n")
	testgit.Run(t, dir, "add", "main.go")
	testgit.Run(t, dir, "commit", "-m", "base")

	a, queue := setup(t)
	a.Open(dir)
	settle(t, a, queue)
	if len(a.Entries) != 0 {
		t.Fatalf("repository should start clean: %+v", a.Entries)
	}
	if err := a.OpenEditorFile(path); err != nil {
		t.Fatal(err)
	}
	a.ActiveEditorBuffer().Set("package main\n\nfunc main() {}\n")
	if err := a.SaveActiveEditorFile(); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(10 * time.Second)
	for len(a.Entries) == 0 {
		select {
		case apply := <-queue:
			apply()
		case <-deadline:
			t.Fatalf("source control did not refresh after save: %+v", a.Entries)
		}
	}
	if len(a.Entries) != 1 || a.Entries[0].Change.Path != "main.go" {
		t.Fatalf("unexpected source control entries: %+v", a.Entries)
	}
}

func TestOpenFolderDialogOpensRepositoryAutomatically(t *testing.T) {
	dir := testgit.Repo(t)
	a, queue := setup(t)
	a.ChooseFolder = func() (string, error) { return dir, nil }
	a.OpenFolderDialog()
	settle(t, a, queue)

	if a.FolderPath != dir {
		t.Fatalf("explorer folder = %q, want %q", a.FolderPath, dir)
	}
	if a.Repository.Repository.RepositoryID == "" {
		t.Fatalf("folder was not recognized as a repository: %s", a.Message)
	}
	if a.Mode != "editor" {
		t.Fatalf("opening a folder did not switch to Editor: %s", a.Mode)
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

func TestOpenEditorsListHasBoundedHeight(t *testing.T) {
	a, _ := setup(t)
	dir := t.TempDir()
	a.SetFolder(dir)
	for i := 0; i < 12; i++ {
		path := filepath.Join(dir, strings.Repeat("file-", i+1)+".txt")
		if err := os.WriteFile(path, []byte("content\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := a.OpenEditorFile(path); err != nil {
			t.Fatal(err)
		}
	}
	tt := ui.NewTester(a.View, 1200, 800)
	list, ok := tt.Find("Open editors list")
	if !ok {
		t.Fatal("missing open editors scroll area")
	}
	if list.H > 22*9+1 {
		t.Fatalf("open editors grew beyond nine rows: %v", list)
	}
	explorer, ok := tt.Find("EXPLORER")
	if !ok || explorer.Y < list.Y+list.H-1 {
		t.Fatalf("Explorer overlaps the open editors area: list=%v explorer=%v", list, explorer)
	}
}

func TestEditorStatusSharesBottomBarWithRemoteStatus(t *testing.T) {
	a, _ := setup(t)
	path := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.OpenEditorFile(path); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(a.View, 1200, 800)
	remote, ok := tt.Find("Remote connection")
	if !ok {
		t.Fatal("missing remote status")
	}
	save, ok := tt.Find("Save")
	if !ok {
		t.Fatal("missing editor status")
	}
	remoteCenter := remote.Y + remote.H/2
	saveCenter := save.Y + save.H/2
	if remoteCenter-saveCenter > 1 || saveCenter-remoteCenter > 1 {
		t.Fatalf("status bars are not aligned: remote=%v save=%v", remote, save)
	}
}

func TestQuickOpenIncludesCommandsWithoutRepository(t *testing.T) {
	a, _ := setup(t)
	a.OpenQuickOpen()
	tt := ui.NewTester(a.View, 1200, 800)
	for _, label := range []string{"Open Folder…", "Open Remote…", "Switch to Git Log", "Use Claude Theme"} {
		if !tt.HasText(label) {
			t.Fatalf("command palette missing %q: %v", label, tt.Texts())
		}
	}
}

func TestQuickOpenRunsCommandFromKeyboard(t *testing.T) {
	a, _ := setup(t)
	a.OpenQuickOpen()
	tt := ui.NewTester(a.View, 1200, 800)
	if err := tt.Click("Quick open"); err != nil {
		t.Fatal(err)
	}
	tt.Type("Git Log")
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	if a.Mode != "log" || a.QuickOpen {
		t.Fatalf("command did not run: mode=%q quickOpen=%v", a.Mode, a.QuickOpen)
	}
}
