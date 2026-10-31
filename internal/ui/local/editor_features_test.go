package local

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/storage"
)

func TestParseKeybinding(t *testing.T) {
	tests := []struct {
		value string
		ok    bool
	}{
		{"Cmd+Shift+F", true},
		{"Ctrl+,", true},
		{"Alt+Up", true},
		{"Cmd+", false},
		{"Hyper+F", false},
		{"Cmd+NotAKey", false},
	}
	for _, test := range tests {
		_, _, ok := parseKeybinding(test.value)
		if ok != test.ok {
			t.Errorf("parseKeybinding(%q) ok = %t, want %t", test.value, ok, test.ok)
		}
	}
	if got := shortcutLabel("Cmd+Shift+F"); got != "⌘⇧F" {
		t.Fatalf("shortcut label = %q", got)
	}
}

func TestCommandRegistryHasUniqueCoreCommands(t *testing.T) {
	a := &App{Settings: defaultAppSettings(), ThemeStyle: ThemeStyleVSCode}
	seen := map[string]bool{}
	for _, command := range a.commandList() {
		if command.ID == "" || command.Label == "" || command.Run == nil {
			t.Fatalf("incomplete command: %+v", command)
		}
		if seen[command.ID] {
			t.Fatalf("duplicate command id %q", command.ID)
		}
		seen[command.ID] = true
	}
	for _, id := range []string{
		"file.quickOpen", "file.newFile", "file.save", "search.workspace",
		"terminal.toggle", "settings.open", "repo.refresh",
	} {
		if !seen[id] {
			t.Errorf("missing command %q", id)
		}
	}
}

func TestSearchTextWholeWordAndGlobs(t *testing.T) {
	re := regexp.MustCompile(`(?i)foo`)
	ref := localFileRef("/tmp/example.go", "/tmp")
	matches := searchText(ref, "example.go", "Foo foo food\nbar foo\n", re, true, 100)
	if len(matches) != 3 {
		t.Fatalf("matches = %+v", matches)
	}
	if matches[0].Line != 1 || matches[0].Column != 1 ||
		matches[1].Line != 1 || matches[1].Column != 5 ||
		matches[2].Line != 2 || matches[2].Column != 5 {
		t.Fatalf("unexpected match positions: %+v", matches)
	}
	if !searchPathIncluded("main.go", "**/*.go", "") {
		t.Fatal("**/*.go should match a root file")
	}
	if !searchPathIncluded("src/main.go", "*.go", "") {
		t.Fatal("*.go should match a basename")
	}
	if searchPathIncluded("vendor/lib/main.go", "**/*.go", "vendor/**") {
		t.Fatal("excluded path matched")
	}
}

func TestWriteFileAtomicPreservesMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(path, []byte("new"), 0); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("content = %q", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestValidateLocalTargetRejectsEscapeAndSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if _, err := validateLocalTarget(root, filepath.Join(root, "..", "outside.txt"), true); err == nil {
		t.Fatal("parent traversal was accepted")
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := validateLocalTarget(root, filepath.Join(link, "file.txt"), true); err == nil {
		t.Fatal("symlink escape was accepted")
	}
}

func TestDirtyCloseDiscardAndSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &App{Settings: defaultAppSettings()}
	tab := newEditorTab(localFileRef(path, filepath.Dir(path)), "package main\n")
	tab.state.Buffer.Set("package main\n\nfunc main() {}\n")
	a.EditorTabs = []*editorTab{tab}
	a.EditorActive = 0

	a.CloseEditorTab(0)
	if !a.DirtyCloseOpen || len(a.EditorTabs) != 1 {
		t.Fatal("dirty tab closed without a prompt")
	}
	a.resolveDirtyClose(false)
	if len(a.EditorTabs) != 0 {
		t.Fatal("discard did not close the tab")
	}

	tab = newEditorTab(localFileRef(path, filepath.Dir(path)), "package main\n")
	tab.state.Buffer.Set("package main\n\nfunc main() {}\n")
	a.EditorTabs = []*editorTab{tab}
	a.EditorActive = 0
	a.CloseEditorTab(0)
	a.resolveDirtyClose(true)
	if len(a.EditorTabs) != 0 {
		t.Fatal("save did not close the tab")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "func main") {
		t.Fatalf("saved content = %q", data)
	}
}

func TestTerminalToggleWithoutWindowReportsFallback(t *testing.T) {
	a := &App{Settings: defaultAppSettings()}
	a.ToggleTerminal()
	if !a.TerminalOpen || a.TerminalError == "" {
		t.Fatalf("terminal state open=%t error=%q", a.TerminalOpen, a.TerminalError)
	}
	a.ToggleTerminal()
	if a.TerminalOpen {
		t.Fatal("terminal did not close")
	}
}

func TestSettingsPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recent.json")
	service := application.LocalService{Recent: storage.RecentRepositories{Path: path}}
	a := &App{Service: service, Settings: defaultAppSettings(), TerminalHeight: 220}
	a.Settings.EditorFontSize = 18
	a.Settings.SearchRegex = true
	a.Settings.Keybindings["file.save"] = "Ctrl+S"
	a.TerminalHeight = 320
	a.savePrefs()

	b := &App{Service: service, Settings: defaultAppSettings(), TerminalHeight: 220}
	b.loadPrefs()
	if b.Settings.EditorFontSize != 18 || !b.Settings.SearchRegex || b.Settings.Keybindings["file.save"] != "Ctrl+S" {
		t.Fatalf("settings did not round trip: %+v", b.Settings)
	}
	if b.TerminalHeight != 320 {
		t.Fatalf("terminal height = %v", b.TerminalHeight)
	}
}

func TestRemoteFileCrudUsesConfinedClient(t *testing.T) {
	files := &fakeRemoteFileSystem{}
	a := newRemoteEditorApp(files)
	target := fileTarget{
		ref:  remoteFileRef("dev", "/srv/project", "/srv/project/new.go"),
		root: "/srv/project", remote: true,
	}
	if err := a.createFile(context.Background(), target); err != nil {
		t.Fatal(err)
	}
	if string(files.wrote[target.ref.Path]) != "" {
		t.Fatal("remote create did not use WriteFileAtomic")
	}
	if err := a.makeDirectory(context.Background(), fileTarget{
		ref:  remoteFileRef("dev", "/srv/project", "/srv/project/src"),
		root: "/srv/project", remote: true,
	}); err != nil {
		t.Fatal(err)
	}
	if len(files.madeDirs) != 1 {
		t.Fatalf("made dirs = %v", files.madeDirs)
	}
}

func TestExplorerMenuUsesContextMenuSemantics(t *testing.T) {
	a := &App{Settings: defaultAppSettings(), FolderPath: t.TempDir()}
	tt := ui.NewTester(func(c *ui.Context) {
		row := ui.ButtonBase(c).Label("file").Height(24).FillWidth().OnClick(func() { a.Message = "clicked" })
		row.ContextMenu(func(m *ui.Menu) { a.explorerPathMenu(m, filepath.Join(a.FolderPath, "file"), false) })
	}, 400, 200)
	if err := tt.Click("file"); err != nil {
		t.Fatal(err)
	}
	if a.Message != "clicked" {
		t.Fatalf("context menu swallowed the click: %q", a.Message)
	}
}

var _ domain.RemoteFileSystem = (*fakeRemoteFileSystem)(nil)
