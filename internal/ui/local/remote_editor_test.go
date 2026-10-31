package local

import (
	"context"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

type fakeRemoteFileSystem struct {
	info     map[string]domain.RemoteFileInfo
	files    map[string][]byte
	dirs     map[string][]domain.RemoteFileInfo
	walk     map[string][]string
	wrote    map[string][]byte
	removed  []string
	renamed  [][2]string
	madeDirs []string
}

func (f *fakeRemoteFileSystem) Canonical(_ context.Context, value string) (string, error) {
	return path.Clean(value), nil
}

func (f *fakeRemoteFileSystem) Stat(_ context.Context, value string) (domain.RemoteFileInfo, error) {
	info, ok := f.info[value]
	if !ok {
		return domain.RemoteFileInfo{}, os.ErrNotExist
	}
	return info, nil
}

func (f *fakeRemoteFileSystem) ReadDir(_ context.Context, value string) ([]domain.RemoteFileInfo, error) {
	entries, ok := f.dirs[value]
	if !ok {
		return nil, os.ErrNotExist
	}
	return entries, nil
}

func (f *fakeRemoteFileSystem) ReadFile(_ context.Context, value string, limit int64) ([]byte, error) {
	data, ok := f.files[value]
	if !ok {
		return nil, os.ErrNotExist
	}
	if int64(len(data)) > limit {
		return nil, &domain.GitError{Code: domain.OutputLimitExceeded}
	}
	return append([]byte(nil), data...), nil
}

func (f *fakeRemoteFileSystem) WriteFileAtomic(_ context.Context, value string, data []byte, _ os.FileMode) error {
	if f.wrote == nil {
		f.wrote = map[string][]byte{}
	}
	f.wrote[value] = append([]byte(nil), data...)
	return nil
}

func (f *fakeRemoteFileSystem) Mkdir(_ context.Context, value string, _ os.FileMode) error {
	f.madeDirs = append(f.madeDirs, value)
	return nil
}

func (f *fakeRemoteFileSystem) Rename(_ context.Context, from, to string) error {
	f.renamed = append(f.renamed, [2]string{from, to})
	return nil
}

func (f *fakeRemoteFileSystem) Remove(_ context.Context, value string, _ bool) error {
	f.removed = append(f.removed, value)
	return nil
}

func (f *fakeRemoteFileSystem) Walk(_ context.Context, root string, _ map[string]bool) ([]string, error) {
	return append([]string(nil), f.walk[root]...), nil
}

func (f *fakeRemoteFileSystem) Close() error { return nil }

func newRemoteEditorApp(files *fakeRemoteFileSystem) *App {
	return &App{
		remoteFiles: map[string]domain.RemoteFileSystem{remoteFilesKey("dev", "/srv/project"): files},
		RemoteRoots: []domain.EditorRoot{{Name: "project", Location: domain.LocationSSH, SSHHost: "dev", Path: "/srv/project"}},
		SSHHost:     "dev",
	}
}

func TestRemoteExplorerOpensNestedFilesWithSessionRoot(t *testing.T) {
	for _, relative := range []string{"README.md", "source/README.md", "source/src/RedisConnection.php"} {
		t.Run(relative, func(t *testing.T) {
			const rootPath = "/srv/project"
			file := path.Join(rootPath, relative)
			content := "text for " + relative + "\n"
			files := &fakeRemoteFileSystem{
				info:  map[string]domain.RemoteFileInfo{file: {Path: file, Size: int64(len(content)), Mode: 0o600}},
				files: map[string][]byte{file: []byte(content)},
			}
			a := newRemoteEditorApp(files)
			a.folderExpanded = map[string]bool{}
			root := &folderNode{path: rootPath, dir: true, remote: true, host: "dev", root: rootPath, loaded: true}
			parent := root
			parts := strings.Split(relative, "/")
			for _, name := range parts[:len(parts)-1] {
				node := &folderNode{name: name, path: path.Join(parent.path, name), dir: true, remote: true, host: "dev", root: rootPath, loaded: true}
				parent.children = append(parent.children, node)
				a.folderExpanded[node.path] = true
				parent = node
			}
			parent.children = append(parent.children, &folderNode{name: path.Base(file), path: file, remote: true, host: "dev", root: rootPath})
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Column(c).Children(func() { a.folderTreeRows(c, root, 0) })
			}, 800, 600)
			if err := tt.Click(file); err != nil {
				t.Fatal(err)
			}
			if len(a.EditorTabs) != 1 || a.ActiveEditorBuffer().String() != content {
				t.Fatalf("file was not opened: message=%q tabs=%d", a.Message, len(a.EditorTabs))
			}
			a.ActiveEditorBuffer().Set("edited\n")
			if err := a.SaveActiveEditorFile(); err != nil {
				t.Fatal(err)
			}
			if string(files.wrote[file]) != "edited\n" {
				t.Fatal("save did not use the original remote session")
			}
		})
	}
}

func TestRemoteEditorRejectsBinaryAndOversizeFiles(t *testing.T) {
	now := time.Now()
	files := &fakeRemoteFileSystem{
		info: map[string]domain.RemoteFileInfo{
			"/srv/project/binary": {Path: "/srv/project/binary", Size: 3, Mode: 0o644, ModTime: now},
			"/srv/project/large":  {Path: "/srv/project/large", Size: maxEditorFileSize + 1, Mode: 0o644, ModTime: now},
		},
		files: map[string][]byte{
			"/srv/project/binary": {'a', 0, 'b'},
		},
	}
	a := newRemoteEditorApp(files)
	if err := a.openRemoteEditorFile(remoteFileRef("dev", "/srv/project", "/srv/project/binary")); err == nil || !strings.Contains(err.Error(), "binary") {
		t.Fatalf("binary error = %v", err)
	}
	if err := a.openRemoteEditorFile(remoteFileRef("dev", "/srv/project", "/srv/project/large")); err == nil || !strings.Contains(err.Error(), "8 MB") {
		t.Fatalf("large error = %v", err)
	}
}

func TestRemoteEditorSaveConflictPreservesBuffer(t *testing.T) {
	now := time.Now()
	files := &fakeRemoteFileSystem{
		info: map[string]domain.RemoteFileInfo{
			"/srv/project/main.go": {Path: "/srv/project/main.go", Size: 12, Mode: 0o640, ModTime: now},
		},
		files: map[string][]byte{"/srv/project/main.go": []byte("package main\n")},
	}
	a := newRemoteEditorApp(files)
	if err := a.openRemoteEditorFile(remoteFileRef("dev", "/srv/project", "/srv/project/main.go")); err != nil {
		t.Fatal(err)
	}
	a.ActiveEditorBuffer().Set("package changed\n")
	files.info["/srv/project/main.go"] = domain.RemoteFileInfo{
		Path: "/srv/project/main.go", Size: 99, Mode: 0o640, ModTime: now.Add(time.Second),
	}
	err := a.SaveActiveEditorFile()
	if err == nil || !strings.Contains(err.Error(), "changed on disk") {
		t.Fatalf("conflict error = %v", err)
	}
	if !a.ActiveEditorBuffer().Modified() || files.wrote["/srv/project/main.go"] != nil {
		t.Fatal("conflict overwrote or cleaned the buffer")
	}
}

func TestRemoteQuickOpenStopsAtGlobalLimit(t *testing.T) {
	left := make([]string, 12000)
	right := make([]string, 12000)
	for i := range left {
		left[i] = path.Join("left", "file", string(rune('a'+i%26)), strings.Repeat("x", 4))
	}
	for i := range right {
		right[i] = path.Join("right", "file", string(rune('a'+i%26)), strings.Repeat("y", 4))
	}
	files := &fakeRemoteFileSystem{walk: map[string][]string{
		"/srv/left":  left,
		"/srv/right": right,
	}}
	roots := []remoteQuickRoot{
		{root: domain.EditorRoot{Name: "left", Location: domain.LocationSSH, SSHHost: "dev", Path: "/srv/left"}, client: files},
		{root: domain.EditorRoot{Name: "right", Location: domain.LocationSSH, SSHHost: "dev", Path: "/srv/right"}, client: files},
	}
	items, err := remoteQuickOpenItems(context.Background(), roots)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 20000 {
		t.Fatalf("items = %d", len(items))
	}
}

func TestRemoteExplorerLoadsLazily(t *testing.T) {
	files := &fakeRemoteFileSystem{dirs: map[string][]domain.RemoteFileInfo{
		"/srv/project": {
			{Path: "/srv/project/src", Name: "src", Dir: true},
			{Path: "/srv/project/.git", Name: ".git", Dir: true},
			{Path: "/srv/project/main.go", Name: "main.go"},
		},
	}}
	a := newRemoteEditorApp(files)
	queue := make(chan func(), 1)
	a.Dispatch = func(apply func()) { queue <- apply }
	node := a.remoteExplorerRoot(a.RemoteRoots[0])
	select {
	case apply := <-queue:
		apply()
	case <-time.After(time.Second):
		t.Fatal("remote directory read did not dispatch")
	}
	if !node.loaded || len(node.children) != 2 || node.children[0].name != "src" || node.children[1].name != "main.go" {
		t.Fatalf("node = %+v", node)
	}
}

func TestRefWithinRepositoryUsesBoundaries(t *testing.T) {
	remote := domain.RepositoryContext{
		Repository: domain.Repository{RepositoryID: "r1", Location: "ssh", Path: "/srv/project"},
		SSHHost:    "dev",
	}
	if !refWithinRepository(remoteFileRef("dev", "/srv/project", "/srv/project/src/main.go"), remote) {
		t.Fatal("child path not matched")
	}
	if refWithinRepository(remoteFileRef("dev", "/srv/project", "/srv/project-other/main.go"), remote) {
		t.Fatal("prefix path escaped repository")
	}
	if refWithinRepository(remoteFileRef("other", "/srv/project", "/srv/project/main.go"), remote) {
		t.Fatal("host mismatch matched")
	}
}
