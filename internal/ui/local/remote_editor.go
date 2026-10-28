package local

import (
	"context"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"gitviewer/internal/application"
	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/sshfs"
	"gitviewer/internal/infrastructure/workspace"
)

var errRemoteFileChanged = errors.New("remote file changed")

type fileRef struct {
	Location domain.EditorLocation
	Host     string
	Root     string
	Path     string
}

func localFileRef(path, root string) fileRef {
	return fileRef{Location: domain.LocationLocal, Root: root, Path: path}
}

func remoteFileRef(host, root, file string) fileRef {
	return fileRef{Location: domain.LocationSSH, Host: host, Root: root, Path: file}
}

func (r fileRef) key() string {
	return string(r.Location) + "\x00" + r.Host + "\x00" + r.Path
}

func remoteFilesKey(host, root string) string {
	return host + "\x00" + root
}

func (a *App) remoteClient(host, root string) domain.RemoteFileSystem {
	if a.remoteFiles == nil {
		a.remoteFiles = map[string]domain.RemoteFileSystem{}
	}
	return a.remoteFiles[remoteFilesKey(host, root)]
}

func (a *App) putRemoteClient(host, root string, client domain.RemoteFileSystem) {
	if a.remoteFiles == nil {
		a.remoteFiles = map[string]domain.RemoteFileSystem{}
	}
	key := remoteFilesKey(host, root)
	if old := a.remoteFiles[key]; old != nil && old != client {
		_ = old.Close()
	}
	a.remoteFiles[key] = client
}

func (a *App) closeRemoteClients() {
	for key, client := range a.remoteFiles {
		_ = client.Close()
		delete(a.remoteFiles, key)
	}
}

func (a *App) notifyRemoteOpened(host, remotePath string) {
	if a.OnRemoteOpened != nil {
		a.OnRemoteOpened(host, remotePath)
	}
}

// OpenRemoteFolder opens one remote root in this window. The window manager is
// responsible for deciding whether a new window is needed.
func (a *App) OpenRemoteFolder(host, dir string) {
	a.openRemoteFolder(host, dir, false)
}

func (a *App) openRemoteFolder(host, dir string, replaceEditors bool) {
	service := a.Service
	a.start(func(ctx context.Context) (func(), error) {
		result, err := service.OpenRemote(ctx, host, dir)
		if err != nil {
			return nil, err
		}
		var status domain.GitStatus
		if result.Repository != nil {
			status, err = service.Status(ctx, *result.Repository)
			if err != nil {
				_ = result.Files.Close()
				return nil, err
			}
		}
		locations, persistErr := service.Locations().Remember(domain.SavedLocation{Kind: "ssh-folder", Host: host, Path: result.Root.Path})
		return func() {
			if replaceEditors {
				a.EditorTabs, a.EditorActive = nil, -1
			}
			a.closeRemoteClients()
			a.putRemoteClient(host, result.Root.Path, result.Files)
			a.RemoteRoots = []domain.EditorRoot{result.Root}
			a.RemoteWorkspaceFile = ""
			a.RemoteWorkspaceHost = host
			a.Workspace = domain.WorkspaceView{}
			a.WorkspaceFolders = nil
			a.WorkspaceRepositories = nil
			a.FolderPath = ""
			a.resetExplorerCache()
			a.Locations = locations
			a.StoreWarning = persistErr != nil
			a.SSHHost, a.SSHPath = host, result.Root.Path
			if result.Repository != nil {
				a.Repository = *result.Repository
				a.PathInput = result.Root.Path
				a.applyStatus(status)
				a.resetHistory()
				a.startAuto()
			} else {
				a.Repository = domain.RepositoryContext{}
				a.Status = domain.GitStatus{}
				a.Entries = nil
				a.PathInput = result.Root.Path
				a.Message = "Remote folder opened · no Git repository"
			}
			a.activateEditorMode()
			a.notifyRemoteOpened(host, result.Root.Path)
		}, nil
	})
}

func (a *App) OpenRemoteFile(host, file string, line, column int) {
	service := a.Service
	rootPath := path.Dir(file)
	a.start(func(ctx context.Context) (func(), error) {
		result, err := service.OpenRemote(ctx, host, rootPath)
		if err != nil {
			return nil, err
		}
		var status domain.GitStatus
		if result.Repository != nil {
			status, err = service.Status(ctx, *result.Repository)
			if err != nil {
				_ = result.Files.Close()
				return nil, err
			}
		}
		locations, persistErr := service.Locations().Remember(domain.SavedLocation{Kind: "ssh-folder", Host: host, Path: result.Root.Path})
		return func() {
			a.closeRemoteClients()
			a.putRemoteClient(host, result.Root.Path, result.Files)
			a.RemoteRoots = []domain.EditorRoot{result.Root}
			a.RemoteWorkspaceFile = ""
			a.RemoteWorkspaceHost = host
			a.Workspace = domain.WorkspaceView{}
			a.WorkspaceFolders = nil
			a.WorkspaceRepositories = nil
			a.FolderPath = ""
			a.resetExplorerCache()
			a.Locations = locations
			a.StoreWarning = persistErr != nil
			a.SSHHost, a.SSHPath = host, result.Root.Path
			if result.Repository != nil {
				a.Repository = *result.Repository
				a.PathInput = result.Root.Path
				a.applyStatus(status)
				a.resetHistory()
				a.startAuto()
			} else {
				a.Repository = domain.RepositoryContext{}
				a.Status = domain.GitStatus{}
				a.Entries = nil
				a.PathInput = result.Root.Path
			}
			ref := remoteFileRef(host, result.Root.Path, file)
			a.Message = message(a.openRemoteEditorFile(ref))
			if a.Message == "Ready" && line > 0 {
				a.goToActiveEditor(line, column)
			}
			a.notifyRemoteOpened(host, result.Root.Path)
		}, nil
	})
}

func (a *App) goToActiveEditor(line, column int) {
	tab := a.activeEditorTab()
	if tab == nil {
		return
	}
	line--
	column--
	if line < 0 {
		line = 0
	}
	if column < 0 {
		column = 0
	}
	tab.state.GoTo(editor.Pos{Line: line, Col: column})
}

func (a *App) GoToActiveEditor(line, column int) { a.goToActiveEditor(line, column) }

func (a *App) AddRemoteFolders(host string, dirs []string) {
	if len(dirs) == 0 {
		return
	}
	service := a.Service
	a.start(func(ctx context.Context) (func(), error) {
		type openedRoot struct {
			root      domain.EditorRoot
			client    *sshfs.Client
			repo      *domain.RepositoryContext
			canonical string
		}
		var opened []openedRoot
		closeOpened := func() {
			for _, item := range opened {
				_ = item.client.Close()
			}
		}
		seen := map[string]bool{}
		for _, dir := range dirs {
			result, err := service.OpenRemote(ctx, host, dir)
			if err != nil {
				closeOpened()
				return nil, err
			}
			if seen[result.Root.Path] {
				_ = result.Files.Close()
				continue
			}
			seen[result.Root.Path] = true
			opened = append(opened, openedRoot{root: result.Root, client: result.Files, repo: result.Repository, canonical: result.Root.Path})
		}
		return func() {
			for _, item := range opened {
				duplicate := false
				for _, existing := range a.RemoteRoots {
					if existing.Location == domain.LocationSSH && existing.SSHHost == host && existing.Path == item.canonical {
						duplicate = true
						break
					}
				}
				if duplicate {
					_ = item.client.Close()
					continue
				}
				a.putRemoteClient(host, item.canonical, item.client)
				a.RemoteRoots = append(a.RemoteRoots, item.root)
				a.WorkspaceFolders = append(a.WorkspaceFolders, domain.WorkspaceFolder{Path: item.root.Path, Name: item.root.Name})
				a.Workspace.Folders = append(a.Workspace.Folders, domain.WorkspaceFolderView{
					Path: item.root.Path, Name: item.root.Name, ResolvedPath: item.root.Path,
					Location: string(domain.LocationSSH), SSHHost: host, RepositoryID: item.root.RepositoryID, Diagnostics: []string{},
				})
				if item.repo != nil {
					found := false
					for _, repo := range a.WorkspaceRepositories {
						if repo.Repository.RepositoryID == item.repo.Repository.RepositoryID {
							found = true
							break
						}
					}
					if !found {
						a.WorkspaceRepositories = append(a.WorkspaceRepositories, *item.repo)
					}
					if a.Repository.Repository.RepositoryID == "" {
						a.Repository = *item.repo
						a.PathInput = item.repo.Repository.Path
						a.resetHistory()
						a.startAuto()
					}
				}
			}
			a.RemoteWorkspaceHost = host
			a.Workspace.Location = string(domain.LocationSSH)
			a.Workspace.SSHHost = host
			a.SSHHost, a.SSHPath = host, a.RemoteRoots[0].Path
			a.resetExplorerCache()
			a.activateEditorMode()
			a.Message = "Added remote folder to window"
			a.notifyRemoteOpened(host, a.RemoteRoots[0].Path)
		}, nil
	})
}

func unifiedWholeFileDiff(leftPath, rightPath, oldText, newText string) string {
	oldLines := strings.Split(oldText, "\n")
	newLines := strings.Split(newText, "\n")
	if len(oldLines) > 0 && oldLines[len(oldLines)-1] == "" {
		oldLines = oldLines[:len(oldLines)-1]
	}
	if len(newLines) > 0 && newLines[len(newLines)-1] == "" {
		newLines = newLines[:len(newLines)-1]
	}
	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n+++ %s\n", leftPath, rightPath)
	fmt.Fprintf(&out, "@@ -1,%d +1,%d @@\n", len(oldLines), len(newLines))
	for i := 0; i < max(len(oldLines), len(newLines)); i++ {
		switch {
		case i < len(oldLines) && i < len(newLines) && oldLines[i] == newLines[i]:
			out.WriteByte(' ')
			out.WriteString(oldLines[i])
			out.WriteByte('\n')
		default:
			if i < len(oldLines) {
				out.WriteByte('-')
				out.WriteString(oldLines[i])
				out.WriteByte('\n')
			}
			if i < len(newLines) {
				out.WriteByte('+')
				out.WriteString(newLines[i])
				out.WriteByte('\n')
			}
		}
	}
	return out.String()
}

func (a *App) OpenRemoteDiff(host, left, right string) {
	service := a.Service
	a.start(func(ctx context.Context) (func(), error) {
		clients := map[string]*sshfs.Client{}
		roots := map[string]domain.EditorRoot{}
		closeClients := func() {
			for _, client := range clients {
				_ = client.Close()
			}
		}
		openFile := func(file string) (string, error) {
			dir := path.Dir(file)
			if clients[dir] == nil {
				result, err := service.OpenRemote(ctx, host, dir)
				if err != nil {
					return "", err
				}
				clients[dir] = result.Files
				roots[dir] = result.Root
			}
			data, err := clients[dir].ReadFile(ctx, file, maxEditorFileSize)
			if err != nil {
				return "", err
			}
			if strings.IndexByte(string(data), 0) >= 0 {
				return "", fmt.Errorf("binary files cannot be compared")
			}
			return string(data), nil
		}
		leftText, err := openFile(left)
		if err != nil {
			closeClients()
			return nil, err
		}
		rightText, err := openFile(right)
		if err != nil {
			closeClients()
			return nil, err
		}
		preview := domain.DiffPreview{
			Scope: domain.WorktreeDiff, Path: right,
			Text: unifiedWholeFileDiff(left, right, leftText, rightText),
		}
		return func() {
			a.closeRemoteClients()
			a.RemoteRoots = nil
			for dir, client := range clients {
				a.putRemoteClient(host, dir, client)
				root := roots[dir]
				root.Location = domain.LocationSSH
				root.SSHHost = host
				root.Path = dir
				if root.Name == "" {
					root.Name = path.Base(dir)
				}
				a.RemoteRoots = append(a.RemoteRoots, root)
			}
			sort.Slice(a.RemoteRoots, func(i, j int) bool { return a.RemoteRoots[i].Path < a.RemoteRoots[j].Path })
			a.RemoteWorkspaceHost = host
			a.RemoteWorkspaceFile = ""
			a.FolderPath = ""
			a.Workspace = domain.WorkspaceView{}
			a.WorkspaceFolders = nil
			a.WorkspaceRepositories = nil
			a.resetExplorerCache()
			a.Pane = "Status"
			a.Mode = "commit"
			a.railPane = a.Pane
			a.applyPreview(preview)
			a.notifyRemoteOpened(host, a.RemoteRoots[0].Path)
		}, nil
	})
}

func (a *App) applyRemoteWorkspace(result application.RemoteWorkspaceOpenResult, host, file string, status *domain.GitStatus) {
	a.closeRemoteClients()
	for root, client := range result.Files {
		a.putRemoteClient(host, root, client)
	}
	a.RemoteRoots = append([]domain.EditorRoot(nil), result.Roots...)
	a.RemoteWorkspaceFile = file
	a.RemoteWorkspaceHost = host
	a.Workspace = result.View
	a.WorkspaceInput = file
	a.WorkspaceFolders = append([]domain.WorkspaceFolder(nil), result.Document.Folders...)
	a.WorkspaceRepositories = result.Repositories
	a.FolderPath = ""
	a.resetExplorerCache()
	a.SSHHost, a.SSHPath = host, file
	if len(result.Repositories) > 0 {
		repo := result.Repositories[0]
		a.Repository = repo
		a.PathInput = repo.Repository.Path
		if status != nil {
			a.applyStatus(*status)
		}
		a.loadWorkspaceStatuses(true)
		a.resetHistory()
		a.startAuto()
	} else {
		a.Repository = domain.RepositoryContext{}
		a.Status = domain.GitStatus{}
		a.Entries = nil
		a.Message = "Remote workspace opened · no Git repository"
	}
	a.activateEditorMode()
	target := file
	if target == "" && len(result.Roots) > 0 {
		target = result.Roots[0].Path
	}
	a.notifyRemoteOpened(host, target)
}

func (a *App) OpenRemoteWorkspace(host, file string) {
	a.openRemoteWorkspace(host, file, false)
}

func (a *App) openRemoteWorkspace(host, file string, replaceEditors bool) {
	service := a.Service
	a.start(func(ctx context.Context) (func(), error) {
		result, err := service.OpenRemoteWorkspace(ctx, host, file)
		if err != nil {
			return nil, err
		}
		var status *domain.GitStatus
		if len(result.Repositories) > 0 {
			value, statusErr := service.Status(ctx, result.Repositories[0])
			if statusErr != nil {
				for _, client := range result.Files {
					_ = client.Close()
				}
				return nil, statusErr
			}
			status = &value
		}
		locations, persistErr := service.Locations().Remember(domain.SavedLocation{Kind: "ssh-workspace", Host: host, Path: file})
		return func() {
			if replaceEditors {
				a.EditorTabs, a.EditorActive = nil, -1
			}
			a.Locations = locations
			a.StoreWarning = persistErr != nil
			a.applyRemoteWorkspace(result, host, file, status)
		}, nil
	})
}

func (a *App) OpenRemoteFolders(host string, dirs []string) {
	if len(dirs) == 0 {
		return
	}
	service := a.Service
	a.start(func(ctx context.Context) (func(), error) {
		files := map[string]*sshfs.Client{}
		var roots []domain.EditorRoot
		var repos []domain.RepositoryContext
		seen := map[string]bool{}
		seenRepos := map[string]bool{}
		closeFiles := func() {
			for _, client := range files {
				_ = client.Close()
			}
		}
		for _, dir := range dirs {
			result, err := service.OpenRemote(ctx, host, dir)
			if err != nil {
				closeFiles()
				return nil, err
			}
			if seen[result.Root.Path] {
				_ = result.Files.Close()
				continue
			}
			seen[result.Root.Path] = true
			files[result.Root.Path] = result.Files
			roots = append(roots, result.Root)
			if result.Repository != nil && !seenRepos[result.Repository.Repository.RepositoryID] {
				seenRepos[result.Repository.Repository.RepositoryID] = true
				repos = append(repos, *result.Repository)
			}
		}
		view := domain.WorkspaceView{Location: string(domain.LocationSSH), SSHHost: host, Folders: []domain.WorkspaceFolderView{}}
		folders := make([]domain.WorkspaceFolder, 0, len(roots))
		for _, root := range roots {
			view.Folders = append(view.Folders, domain.WorkspaceFolderView{
				Path: root.Path, Name: root.Name, ResolvedPath: root.Path, Location: string(domain.LocationSSH), SSHHost: host,
				RepositoryID: root.RepositoryID, Diagnostics: []string{},
			})
			folders = append(folders, domain.WorkspaceFolder{Path: root.Path, Name: root.Name})
		}
		document := workspace.Document{Text: []byte("{\n  \"folders\": []\n}\n"), Folders: folders}
		var status *domain.GitStatus
		if len(repos) > 0 {
			value, statusErr := service.Status(ctx, repos[0])
			if statusErr != nil {
				closeFiles()
				return nil, statusErr
			}
			status = &value
		}
		return func() {
			a.applyRemoteWorkspace(application.RemoteWorkspaceOpenResult{
				Document: document, View: view, Repositories: repos, Roots: roots, Files: files,
			}, host, "", status)
		}, nil
	})
}

func (a *App) saveRemoteWorkspace(create bool) {
	host, file := a.RemoteWorkspaceHost, a.RemoteWorkspaceFile
	if host == "" || file == "" {
		a.Message = "Remote workspace needs a .code-workspace path"
		return
	}
	folders := append([]domain.WorkspaceFolder(nil), a.WorkspaceFolders...)
	revision := a.Workspace.Revision
	service := a.Service
	a.start(func(ctx context.Context) (func(), error) {
		result, err := service.SaveRemoteWorkspace(ctx, host, file, revision, folders, create)
		if err != nil {
			return nil, err
		}
		var status *domain.GitStatus
		if len(result.Repositories) > 0 {
			value, statusErr := service.Status(ctx, result.Repositories[0])
			if statusErr != nil {
				for _, client := range result.Files {
					_ = client.Close()
				}
				return nil, statusErr
			}
			status = &value
		}
		locations, persistErr := service.Locations().Remember(domain.SavedLocation{Kind: "ssh-workspace", Host: host, Path: file})
		return func() {
			a.Locations = locations
			a.StoreWarning = persistErr != nil
			a.applyRemoteWorkspace(result, host, file, status)
		}, nil
	})
}

func (a *App) remoteRootFor(host, file string) string {
	for _, root := range a.RemoteRoots {
		if root.Location == domain.LocationSSH && root.SSHHost == host && (file == root.Path || strings.HasPrefix(file, strings.TrimSuffix(root.Path, "/")+"/")) {
			return root.Path
		}
	}
	return ""
}

func (a *App) remoteMeta(ref fileRef) (sshfs.FileMeta, error) {
	client := a.remoteClient(ref.Host, ref.Root)
	if client == nil {
		return sshfs.FileMeta{}, fmt.Errorf("remote connection is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return client.Stat(ctx, ref.Path)
}

func (a *App) openRemoteEditorFile(ref fileRef) error {
	for i, tab := range a.EditorTabs {
		if tab.ref.key() == ref.key() {
			a.EditorActive = i
			a.activateEditorMode()
			return nil
		}
	}
	client := a.remoteClient(ref.Host, ref.Root)
	if client == nil {
		return fmt.Errorf("remote connection is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	info, err := client.Stat(ctx, ref.Path)
	if err != nil {
		return err
	}
	if info.Dir {
		return fmt.Errorf("cannot open a directory")
	}
	if info.Size > maxEditorFileSize {
		return fmt.Errorf("file is larger than 8 MB")
	}
	data, err := client.ReadFile(ctx, ref.Path, maxEditorFileSize)
	if err != nil {
		return err
	}
	if strings.IndexByte(string(data), 0) >= 0 {
		return fmt.Errorf("binary files cannot be edited")
	}
	tab := newEditorTab(ref, string(data))
	tab.meta = info
	a.EditorTabs = append(a.EditorTabs, tab)
	a.EditorActive = len(a.EditorTabs) - 1
	a.activateEditorMode()
	return nil
}

func (a *App) saveRemoteEditorFile(tab *editorTab) error {
	return a.saveRemoteEditorFileMode(tab, false)
}

func (a *App) saveRemoteEditorFileMode(tab *editorTab, force bool) error {
	client := a.remoteClient(tab.ref.Host, tab.ref.Root)
	if client == nil {
		return fmt.Errorf("remote connection is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if tab.meta.Path != "" && !force {
		current, err := client.Stat(ctx, tab.ref.Path)
		if err != nil {
			return err
		}
		if current.Size != tab.meta.Size || !current.ModTime.Equal(tab.meta.ModTime) {
			return errRemoteFileChanged
		}
	}
	if err := client.WriteFileAtomic(ctx, tab.ref.Path, []byte(tab.state.Buffer.String()), tab.meta.Mode); err != nil {
		return err
	}
	info, err := client.Stat(ctx, tab.ref.Path)
	if err == nil {
		tab.meta = info
	}
	return nil
}

func (a *App) reloadRemoteEditorFile(tab *editorTab) error {
	client := a.remoteClient(tab.ref.Host, tab.ref.Root)
	if client == nil {
		return fmt.Errorf("remote connection is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	info, err := client.Stat(ctx, tab.ref.Path)
	if err != nil {
		return err
	}
	data, err := client.ReadFile(ctx, tab.ref.Path, maxEditorFileSize)
	if err != nil {
		return err
	}
	if strings.IndexByte(string(data), 0) >= 0 {
		return fmt.Errorf("binary files cannot be edited")
	}
	text := string(data)
	tab.original = editor.NewBuffer(text)
	tab.original.SetLanguage(editor.LanguageFor(tab.ref.Path))
	tab.state.Buffer.Set(text)
	tab.state.Buffer.MarkSaved()
	tab.state.DiffBase = tab.original
	tab.meta = info
	return nil
}

func (a *App) remoteExplorerRoot(root domain.EditorRoot) *folderNode {
	if a.folderTrees == nil {
		a.folderTrees = map[string]*folderNode{}
	}
	key := "remote:" + root.SSHHost + ":" + root.Path
	if tree := a.folderTrees[key]; tree != nil {
		return tree
	}
	node := &folderNode{name: root.Name, path: root.Path, dir: true, remote: true, host: root.SSHHost, root: root.Path}
	a.folderTrees[key] = node
	a.requestRemoteDir(node)
	return node
}

func (a *App) requestRemoteDir(node *folderNode) {
	if node == nil || !node.remote || node.loaded || node.loading {
		return
	}
	if a.remoteLoading == nil {
		a.remoteLoading = map[string]bool{}
	}
	loadingKey := node.host + "\x00" + node.path
	if a.remoteLoading[loadingKey] {
		return
	}
	client := a.remoteClient(node.host, node.root)
	if client == nil {
		node.err = "Remote connection is unavailable"
		return
	}
	node.loading = true
	a.remoteLoading[loadingKey] = true
	dispatch := a.Dispatch
	if dispatch == nil {
		node.loading = false
		delete(a.remoteLoading, loadingKey)
		return
	}
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		entries, err := client.ReadDir(ctx, node.path)
		dispatch(func() {
			node.loading = false
			delete(a.remoteLoading, loadingKey)
			if err != nil {
				node.err = "Cannot read remote folder"
				return
			}
			node.err = ""
			node.loaded = true
			node.children = nil
			for _, entry := range entries {
				if entry.Name == ".git" || entry.Name == ".hg" || entry.Name == ".svn" {
					continue
				}
				node.children = append(node.children, &folderNode{
					name: entry.Name, path: entry.Path, dir: entry.Dir, remote: true, host: node.host, root: node.root,
				})
			}
			sortFolderTree(node)
		})
	}()
}

type remoteQuickRoot struct {
	root   domain.EditorRoot
	client domain.RemoteFileSystem
}

func remoteQuickOpenItems(ctx context.Context, roots []remoteQuickRoot) ([]quickOpenItem, error) {
	items := []quickOpenItem{}
	multiRoot := len(roots) > 1
	limitReached := false
	for _, item := range roots {
		files, err := item.client.Walk(ctx, item.root.Path, quickOpenSkipDirs)
		if err != nil {
			return nil, err
		}
		for _, rel := range files {
			label := rel
			if multiRoot {
				label = path.Join(item.root.Name, rel)
			}
			file := path.Join(item.root.Path, rel)
			items = append(items, quickOpenItem{label: label, path: file, ref: remoteFileRef(item.root.SSHHost, item.root.Path, file)})
			if len(items) >= 20000 {
				limitReached = true
				break
			}
		}
		if limitReached {
			break
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return strings.ToLower(items[i].label) < strings.ToLower(items[j].label)
	})
	return items, nil
}

func (a *App) openQuickOpenAsync() {
	if len(a.RemoteRoots) == 0 {
		a.OpenQuickOpen()
		return
	}
	roots := make([]remoteQuickRoot, 0, len(a.RemoteRoots))
	for _, root := range a.RemoteRoots {
		if client := a.remoteClient(root.SSHHost, root.Path); client != nil {
			roots = append(roots, remoteQuickRoot{root: root, client: client})
		}
	}
	if len(roots) == 0 {
		a.Message = "Remote connection is unavailable"
		return
	}
	a.startQuiet(func(ctx context.Context) (func(), error) {
		items, err := remoteQuickOpenItems(ctx, roots)
		if err != nil {
			return nil, err
		}
		return func() {
			a.quickOpenItems = items
			a.quickOpenKey = "remote"
			a.QuickOpenQuery = ""
			a.QuickOpen = true
		}, nil
	})
}

func (a *App) openEditorRef(ref fileRef) error {
	if ref.Location == domain.LocationSSH {
		return a.openRemoteEditorFile(ref)
	}
	return a.OpenEditorFile(ref.Path)
}

func (a *App) remoteLoadingRow(c *ui.Context, node *folderNode, depth int) {
	if node.loading {
		ui.Text(c, "Loading…").FontSize(11).TextColor(c.Theme().TextMuted).Padding(0, float32(8+depth*12))
	} else if node.err != "" {
		ui.Text(c, node.err).FontSize(11).TextColor(c.Theme().Danger).Padding(0, float32(8+depth*12))
	}
}
