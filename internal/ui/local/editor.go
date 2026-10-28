package local

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/sshfs"
)

// maxEditorFileSize keeps multi-hundred-megabyte files (or binaries) from
// locking up the UI while the editor builds its buffer.
const maxEditorFileSize = 8 << 20

var quickOpenSkipDirs = map[string]bool{
	".git": true, ".hg": true, ".svn": true, "node_modules": true,
	"vendor": true, "dist": true, "build": true, "target": true, ".next": true,
}

type folderNode struct {
	name     string
	path     string
	dir      bool
	children []*folderNode
	remote   bool
	host     string
	root     string
	loading  bool
	loaded   bool
	err      string
}

type explorerRoot struct {
	tree  *folderNode
	label string
}

type quickOpenItem struct {
	label string
	path  string
	ref   fileRef
}

// editorTab is one open file: the saved text as the diff base and the live
// editor state, kept across frames like the other panels' widgets.
type editorTab struct {
	ref      fileRef
	meta     sshfs.FileMeta
	original *editor.Buffer
	state    *editor.State
}

func (t *editorTab) path() string { return t.ref.Path }

func (t *editorTab) name() string {
	if t.ref.Location == domain.LocationSSH {
		return path.Base(t.ref.Path)
	}
	return filepath.Base(t.ref.Path)
}

func newEditorTab(ref fileRef, text string) *editorTab {
	lang := editor.LanguageFor(ref.Path)
	original := editor.NewBuffer(text)
	original.SetLanguage(lang)
	buffer := editor.NewBuffer(text)
	buffer.SetLanguage(lang)
	return &editorTab{ref: ref, original: original, state: &editor.State{Buffer: buffer, DiffBase: original}}
}

// OpenEditorFile opens a file in the editor area, or activates its tab when it
// is already open, the way editors reuse tabs for the same document.
func (a *App) OpenEditorFile(path string) error {
	for i, tab := range a.EditorTabs {
		if tab.ref.Location == domain.LocationLocal && tab.ref.Path == path {
			a.EditorActive = i
			a.activateEditorMode()
			return nil
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("cannot open a directory")
	}
	if info.Size() > maxEditorFileSize {
		return fmt.Errorf("file is larger than 8 MB")
	}
	text, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if strings.IndexByte(string(text), 0) >= 0 {
		return fmt.Errorf("binary files cannot be edited")
	}
	a.EditorTabs = append(a.EditorTabs, newEditorTab(localFileRef(path, a.folderRoot()), string(text)))
	a.EditorActive = len(a.EditorTabs) - 1
	a.activateEditorMode()
	return nil
}

// OpenEditorFileDialog asks for a file off the UI thread: MyGo's dialog APIs
// pump native events and must not run inside a view callback.
func (a *App) OpenEditorFileDialog() {
	choose := a.ChooseFile
	if choose == nil {
		return
	}
	a.showDialog(func() func() {
		path, err := choose()
		return func() {
			if err != nil {
				a.Message = "Cannot open file dialog"
				return
			}
			if path != "" {
				a.Message = message(a.OpenEditorFile(path))
			}
		}
	})
}

// OpenQuickOpen shows the in-app file picker. It never calls a native dialog,
// so it cannot re-enter or block the view callback the way an OS sheet can.
func (a *App) OpenQuickOpen() {
	if len(a.RemoteRoots) > 0 {
		a.openQuickOpenAsync()
		return
	}
	roots := a.explorerRoots()
	if len(roots) == 0 {
		a.Message = "Open a folder or repository to search its files"
		return
	}
	key := a.explorerRootsKey(roots)
	if a.quickOpenKey != key {
		a.quickOpenItems = listQuickOpenItems(roots)
		a.quickOpenKey = key
	}
	a.QuickOpenQuery = ""
	a.QuickOpen = true
}

// folderRoot is the Explorer root: the opened folder, else the repository.
func (a *App) folderRoot() string {
	if a.FolderPath != "" {
		return a.FolderPath
	}
	return a.Repository.Repository.Path
}

// SetFolder opens a directory in the Explorer and switches to the editor.
func (a *App) SetFolder(path string) {
	a.closeRemoteClients()
	a.RemoteRoots = nil
	a.RemoteWorkspaceFile = ""
	a.RemoteWorkspaceHost = ""
	a.FolderPath = path
	a.resetExplorerCache()
	a.activateEditorMode()
}

// OpenFolderDialog asks for a directory off the UI thread, like the other
// native dialogs, so it cannot re-enter the view callback.
func (a *App) OpenFolderDialog() {
	choose := a.ChooseFolder
	if choose == nil {
		return
	}
	a.showDialog(func() func() {
		path, err := choose()
		return func() {
			if err != nil {
				a.Message = "Cannot open folder dialog"
				return
			}
			if path != "" {
				a.SetFolder(path)
				a.Open(path)
			}
		}
	})
}

func (a *App) explorerTree() *folderNode {
	root := a.folderRoot()
	if root == "" {
		return nil
	}
	return a.explorerTreeAt(root)
}

// explorerRoots returns the visible Explorer roots. A VS Code workspace is a
// multi-root Explorer; otherwise the opened folder (or repository) is the one
// root.
func (a *App) explorerRoots() []explorerRoot {
	var roots []explorerRoot
	if len(a.RemoteRoots) > 0 {
		seen := map[string]bool{}
		for _, folder := range a.RemoteRoots {
			key := folder.SSHHost + "\x00" + folder.Path
			if folder.Location != domain.LocationSSH || folder.SSHHost == "" || folder.Path == "" || seen[key] {
				continue
			}
			seen[key] = true
			label := folder.Name
			if label == "" {
				label = path.Base(folder.Path)
			}
			roots = append(roots, explorerRoot{tree: a.remoteExplorerRoot(folder), label: label})
		}
		return roots
	}
	if a.Workspace.FilePath != "" && len(a.Workspace.Folders) > 0 {
		seen := map[string]bool{}
		for _, folder := range a.Workspace.Folders {
			root := folder.ResolvedPath
			if root == "" {
				root = folder.Path
			}
			if root == "" || seen[root] {
				continue
			}
			seen[root] = true
			label := folder.Name
			if label == "" {
				label = filepath.Base(root)
			}
			if tree := a.explorerTreeAt(root); tree != nil {
				roots = append(roots, explorerRoot{tree: tree, label: label})
			}
		}
		return roots
	}
	if root := a.folderRoot(); root != "" {
		if tree := a.explorerTreeAt(root); tree != nil {
			roots = append(roots, explorerRoot{tree: tree, label: filepath.Base(root)})
		}
	}
	return roots
}

func (a *App) explorerRootsKey(roots []explorerRoot) string {
	parts := make([]string, 0, len(roots))
	for _, root := range roots {
		location := string(domain.LocationLocal)
		if root.tree.remote {
			location = string(domain.LocationSSH) + ":" + root.tree.host
		}
		parts = append(parts, location+"\x01"+root.tree.path+"\x01"+root.label)
	}
	return strings.Join(parts, "\x00")
}

func (a *App) explorerTreeAt(root string) *folderNode {
	if a.folderTrees == nil {
		a.folderTrees = map[string]*folderNode{}
	}
	if tree := a.folderTrees[root]; tree != nil {
		return tree
	}
	rootNode := &folderNode{name: filepath.Base(root), path: root, dir: true}
	byPath := map[string]*folderNode{"": rootNode}
	for _, rel := range listQuickOpenFiles(root) {
		parts := strings.Split(rel, "/")
		parent := rootNode
		prefix := ""
		for i, part := range parts {
			prefix = strings.TrimPrefix(prefix+"/"+part, "/")
			node := byPath[prefix]
			if node == nil {
				node = &folderNode{name: part, path: filepath.Join(root, filepath.FromSlash(prefix)), dir: i < len(parts)-1}
				byPath[prefix] = node
				parent.children = append(parent.children, node)
			}
			parent = node
		}
	}
	sortFolderTree(rootNode)
	a.folderTrees[root] = rootNode
	return rootNode
}

func (a *App) resetExplorerCache() {
	a.folderTrees = nil
	a.quickOpenKey = ""
	a.quickOpenItems = nil
}

func sortFolderTree(node *folderNode) {
	sort.SliceStable(node.children, func(i, j int) bool {
		if node.children[i].dir != node.children[j].dir {
			return node.children[i].dir
		}
		return strings.ToLower(node.children[i].name) < strings.ToLower(node.children[j].name)
	})
	for _, child := range node.children {
		if child.dir {
			sortFolderTree(child)
		}
	}
}

func listQuickOpenFiles(root string) []string {
	var files []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && quickOpenSkipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		files = append(files, filepath.ToSlash(rel))
		if len(files) >= 20000 {
			return filepath.SkipAll
		}
		return nil
	})
	sort.Strings(files)
	return files
}

func listQuickOpenItems(roots []explorerRoot) []quickOpenItem {
	items := []quickOpenItem{}
	multiRoot := len(roots) > 1
	for _, root := range roots {
		for _, rel := range listQuickOpenFiles(root.tree.path) {
			label := rel
			if multiRoot {
				label = filepath.ToSlash(filepath.Join(root.label, filepath.FromSlash(rel)))
			}
			path := filepath.Join(root.tree.path, filepath.FromSlash(rel))
			items = append(items, quickOpenItem{label: label, path: path, ref: localFileRef(path, root.tree.path)})
			if len(items) >= 20000 {
				break
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return strings.ToLower(items[i].label) < strings.ToLower(items[j].label)
	})
	return items
}

func (a *App) quickOpenMatches() []quickOpenItem {
	query := strings.ToLower(strings.TrimSpace(a.QuickOpenQuery))
	matches := make([]quickOpenItem, 0, 20)
	for _, item := range a.quickOpenItems {
		if query != "" && !strings.Contains(strings.ToLower(item.label), query) {
			continue
		}
		matches = append(matches, item)
		if len(matches) >= 20 {
			break
		}
	}
	return matches
}

// quickOpenView renders the command-palette-style file picker.
func (a *App) quickOpenView(c *ui.Context) {
	ui.Modal(c, &a.QuickOpen, func() {
		matches := a.quickOpenMatches()
		ui.Column(c).Width(640).MaxHeight(440).Padding(8).Gap(6).
			Background(c.Theme().Surface).Border(1, c.Theme().Border).Radius(6).Children(func() {
			input := ui.TextInput(c, &a.QuickOpenQuery).Label("Quick open").Placeholder("Search files by path…").FillWidth().AutoFocus()
			if input.Submitted() && len(matches) > 0 {
				a.QuickOpen = false
				a.Message = message(a.openEditorRef(matches[0].ref))
				return
			}
			ui.Scroll(c).MaxHeight(340).FillWidth().Children(func() {
				ui.Column(c).FillWidth().Gap(2).Children(func() {
					if len(matches) == 0 {
						ui.Text(c, "No matching files").TextColor(c.Theme().TextMuted).Padding(8)
						return
					}
					for _, match := range matches {
						match := match
						ui.ButtonBase(c).Label(match.label).Tooltip(match.path).FillWidth().Height(28).Radius(2).
							Cursor(ui.CursorPointer).AlignItems(ui.Center).
							OnClick(func() {
								a.QuickOpen = false
								a.Message = message(a.openEditorRef(match.ref))
							}).Children(func() {
							ui.Text(c, match.label).NoWrap().TextColor(c.Theme().Text)
						})
					}
				})
			})
			ui.Text(c, "Enter to open · Esc to close").FontSize(11).TextColor(c.Theme().TextMuted)
		})
	})
}

// activateEditorMode switches the shell to the editor without letting
// syncMode derive another mode from a stale Pane value.
func (a *App) activateEditorMode() {
	a.Mode = "editor"
	a.Pane = "Editor"
	a.railPane = "Editor"
}

// SaveActiveEditorFile writes the active tab to disk and rebases its diff on
// the saved text so the dirty marker clears.
func (a *App) SaveActiveEditorFile() error {
	tab := a.activeEditorTab()
	if tab == nil {
		return nil
	}
	text := tab.state.Buffer.String()
	if tab.ref.Location == domain.LocationSSH {
		if err := a.saveRemoteEditorFile(tab); err != nil {
			if !errors.Is(err, errRemoteFileChanged) {
				a.Stale = true
				return err
			}
			confirm := a.ConfirmRemoteConflict
			if confirm == nil {
				return fmt.Errorf("remote file changed on disk; reload before saving")
			}
			path := tab.ref.Path
			a.showDialog(func() func() {
				choice := confirm(path)
				return func() {
					switch choice {
					case 1:
						if err := a.saveRemoteEditorFileMode(tab, true); err != nil {
							a.Message = message(err)
						} else {
							a.finishEditorSave(tab)
							a.Message = "Ready"
						}
					case 2:
						a.Message = message(a.reloadRemoteEditorFile(tab))
					default:
						a.Message = "Remote save cancelled"
					}
				}
			})
			return nil
		}
	} else if err := os.WriteFile(tab.ref.Path, []byte(text), 0o644); err != nil {
		return err
	}
	a.finishEditorSave(tab)
	return nil
}

func (a *App) finishEditorSave(tab *editorTab) {
	text := tab.state.Buffer.String()
	saved := newEditorTab(tab.ref, text)
	tab.original = saved.original
	tab.state.DiffBase = saved.original
	tab.state.Buffer.MarkSaved()
	a.requestBackgroundRefreshFor(tab.ref)
}

func (a *App) activeEditorTab() *editorTab {
	if a.EditorActive < 0 || a.EditorActive >= len(a.EditorTabs) {
		return nil
	}
	return a.EditorTabs[a.EditorActive]
}

// ActiveEditorBuffer exposes the active tab's buffer for tests and shortcuts.
func (a *App) ActiveEditorBuffer() *editor.Buffer {
	if tab := a.activeEditorTab(); tab != nil {
		return tab.state.Buffer
	}
	return nil
}

func (a *App) closeEditorTab(i int) {
	a.EditorTabs = append(a.EditorTabs[:i], a.EditorTabs[i+1:]...)
	if a.EditorActive >= len(a.EditorTabs) {
		a.EditorActive = len(a.EditorTabs) - 1
	}
	if a.EditorActive < 0 {
		a.EditorActive = 0
	}
}

// CloseEditorTab closes the i-th tab; the active tab falls back to the last
// remaining one.
func (a *App) CloseEditorTab(i int) { a.closeEditorTab(i) }

// editorSidebar is the Explorer view: open editors followed by the workspace
// folder tree.
func (a *App) editorSidebar(c *ui.Context) {
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().Gap(0).Children(func() {
			if len(a.EditorTabs) > 0 {
				ui.Text(c, "OPEN EDITORS").Bold().FontSize(10).TextColor(c.Theme().TextMuted).Padding(4, 8)
				for i, tab := range a.EditorTabs {
					label := tab.name()
					if tab.state.Buffer.Modified() {
						label += " ●"
					}
					active := i == a.EditorActive
					item := ui.ButtonBase(c).Label(label).Tooltip(tab.path()).FillWidth().Height(22).
						Padding(0, 20).Radius(2).Cursor(ui.CursorPointer).AlignItems(ui.Center).Justify(ui.Start).
						OnClick(func() { a.EditorActive = i })
					color := c.Theme().Text
					if active {
						item.Background(c.Theme().Selection)
					} else if item.Hovered() {
						item.Background(c.Theme().SurfaceHover)
					}
					item.Children(func() {
						ui.Icon(c, changeFileIcon).Size(14, 14).TextColor(c.Theme().TextMuted).Shrink(0)
						ui.Text(c, label).TextColor(color).NoWrap().Shrink(1)
					})
				}
			}
			roots := a.explorerRoots()
			if len(roots) > 0 {
				ui.Row(c).Height(30).FillWidth().Padding(0, 8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "EXPLORER").Bold().FontSize(10).TextColor(c.Theme().TextMuted).Grow(1)
					iconButton(c, "Open file…", "Open file (⌘P)", changeFileIcon).
						Size(20, 20).OnClick(a.OpenQuickOpen)
					iconButton(c, "Refresh explorer", "Refresh explorer", refreshIcon).
						Size(20, 20).OnClick(func() {
						a.resetExplorerCache()
						a.Refresh()
					})
					iconButton(c, "Collapse folders", "Collapse folders", treeIcon).
						Size(20, 20).OnClick(func() {
						if a.folderExpanded == nil {
							a.folderExpanded = map[string]bool{}
						}
						for _, root := range roots {
							a.collapseFolderTree(root.tree)
						}
					})
				})
				for _, root := range roots {
					a.folderRootRow(c, root.tree, root.label)
					if a.folderRootExpanded(root.tree) {
						a.requestRemoteDir(root.tree)
						a.folderTreeRows(c, root.tree, 1)
						a.remoteLoadingRow(c, root.tree, 1)
					}
				}
			} else {
				ui.Row(c).Height(30).FillWidth().Padding(0, 8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "EXPLORER").Bold().FontSize(10).TextColor(c.Theme().TextMuted).Grow(1)
					iconButton(c, "Open file…", "Open file (⌘P)", changeFileIcon).Size(20, 20).OnClick(a.OpenQuickOpen)
				})
				ui.Text(c, "Open a folder to browse files").FontSize(11).TextColor(c.Theme().TextMuted).Padding(8)
			}
		})
	})
}

func (a *App) folderRootExpanded(root *folderNode) bool {
	expanded, ok := a.folderExpanded[root.path]
	return !ok || expanded
}

func (a *App) folderRootRow(c *ui.Context, root *folderNode, label string) {
	expanded := a.folderRootExpanded(root)
	folder := folderIcon
	arrow := "›"
	if expanded {
		folder = folderOpenIcon
		arrow = "⌄"
	}
	row := ui.ButtonBase(c).Label("Toggle folder "+root.path).Tooltip(root.path).FillWidth().Height(22).
		Padding(0, 8).Radius(2).Cursor(ui.CursorPointer).AlignItems(ui.Center).Justify(ui.Start).
		OnClick(func() {
			if a.folderExpanded == nil {
				a.folderExpanded = map[string]bool{}
			}
			a.folderExpanded[root.path] = !expanded
		})
	if row.Hovered() {
		row.Background(c.Theme().SurfaceHover)
	}
	row.Children(func() {
		ui.Text(c, arrow).FontSize(11).TextColor(c.Theme().TextMuted).Width(12)
		ui.Icon(c, folder).Size(15, 15).Shrink(0).TextColor(c.Theme().TextMuted)
		ui.Text(c, compactLabel(label, 30)).Bold().FontSize(12).TextColor(c.Theme().Text).NoWrap().Shrink(1)
	})
}

func (a *App) collapseFolderTree(node *folderNode) {
	if !node.dir {
		return
	}
	if a.folderExpanded == nil {
		a.folderExpanded = map[string]bool{}
	}
	a.folderExpanded[node.path] = false
	for _, child := range node.children {
		a.collapseFolderTree(child)
	}
}

func (a *App) folderTreeRows(c *ui.Context, node *folderNode, depth int) {
	for _, child := range node.children {
		child := child
		if child.dir {
			expanded := a.folderExpanded[child.path]
			mark := "⌄"
			if !expanded {
				mark = "›"
			}
			folder := folderIcon
			if expanded {
				folder = folderOpenIcon
			}
			row := ui.ButtonBase(c).Label(child.path).Tooltip(child.path).FillWidth().Height(22).
				Padding(0, float32(8+depth*12)).Radius(2).Cursor(ui.CursorPointer).Justify(ui.Start).
				AlignItems(ui.Center).OnClick(func() {
				if a.folderExpanded == nil {
					a.folderExpanded = map[string]bool{}
				}
				a.folderExpanded[child.path] = !expanded
			})
			if row.Hovered() {
				row.Background(c.Theme().SurfaceHover)
			}
			row.Children(func() {
				ui.Text(c, mark).FontSize(11).TextColor(c.Theme().TextMuted).Width(12)
				ui.Icon(c, folder).Size(15, 15).Shrink(0).TextColor(c.Theme().TextMuted)
				ui.Text(c, child.name).TextColor(c.Theme().Text).FontSize(12).NoWrap()
			})
			if expanded {
				a.requestRemoteDir(child)
				a.folderTreeRows(c, child, depth+1)
				a.remoteLoadingRow(c, child, depth+1)
			}
			continue
		}
		activeRef := fileRef{}
		if tab := a.activeEditorTab(); tab != nil {
			activeRef = tab.ref
		}
		childRef := localFileRef(child.path, node.path)
		if child.remote {
			childRef = remoteFileRef(child.host, node.path, child.path)
		}
		active := activeRef.key() == childRef.key()
		row := ui.ButtonBase(c).Label(child.path).Tooltip(child.path).FillWidth().Height(22).
			Padding(0, float32(8+depth*12)).Radius(2).Cursor(ui.CursorPointer).Justify(ui.Start).
			AlignItems(ui.Center).OnClick(func() {
			a.Message = message(a.openEditorRef(childRef))
		})
		if active {
			row.Background(c.Theme().Selection)
		} else if row.Hovered() {
			row.Background(c.Theme().SurfaceHover)
		}
		row.Children(func() {
			ui.Box(c).Width(12).Shrink(0)
			ui.Icon(c, changeFileIcon).Size(15, 15).Shrink(0).TextColor(c.Theme().TextMuted)
			ui.Text(c, child.name).TextColor(c.Theme().Text).FontSize(12).NoWrap()
		})
	}
}

// editorView is the editor area: a VS Code-style tab strip, the code editor
// and a status line with cursor position and language.
func (a *App) editorView(c *ui.Context) {
	ui.Column(c).Label("Editor workspace").Fill().Children(func() {
		a.editorTabs(c)
		tab := a.activeEditorTab()
		if tab == nil {
			ui.Column(c).Grow(1).FillWidth().Center().Children(func() {
				ui.Text(c, "No file open").FontSize(18)
				ui.Text(c, "Open a file from the side bar, or press ⌘S to save the active editor.").FontSize(12).TextColor(c.Theme().TextMuted)
			})
			return
		}
		editor.View(c, tab.state).Grow(1).FillWidth()
		a.editorStatus(c, tab)
	})
}

func (a *App) editorTabs(c *ui.Context) {
	ui.ScrollHorizontal(c).Label("Editor tabs").Height(35).FillWidth().Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Stretch).Children(func() {
			for i, tab := range a.EditorTabs {
				active := i == a.EditorActive
				name := tab.name()
				item := ui.ButtonBase(c).Label(name).Tooltip(tab.path()).Height(35).
					Padding(0, 10).Radius(0).Cursor(ui.CursorPointer).AlignItems(ui.Center).
					OnClick(func() { a.EditorActive = i })
				if active {
					item.Background(c.Theme().Background)
				} else {
					item.Background(c.Theme().Surface)
				}
				item.Children(func() {
					ui.Text(c, name).TextColor(c.Theme().Text).NoWrap()
					if tab.state.Buffer.Modified() {
						ui.Text(c, "●").TextColor(c.Theme().Accent)
					}
				})
				close := ui.ButtonBase(c).Label("Close " + name).Tooltip("Close tab").Height(35).
					Width(28).Radius(0).Cursor(ui.CursorPointer).AlignItems(ui.Center).Justify(ui.Center).
					OnClick(func() { a.closeEditorTab(i) })
				if active {
					close.Background(c.Theme().Background)
				} else {
					close.Background(c.Theme().Surface)
				}
				close.Children(func() { ui.Text(c, "×").TextColor(c.Theme().TextMuted) })
			}
		})
	})
}

func (a *App) editorStatus(c *ui.Context, tab *editorTab) {
	pos := tab.state.CursorPos()
	lang := "Plain Text"
	if l := tab.state.Buffer.Language(); l != nil {
		lang = l.Name
	}
	status := fmt.Sprintf("Ln %d, Col %d", pos.Line+1, pos.Col+1)
	ui.Row(c).Height(24).FillWidth().Padding(0, 12).Gap(16).AlignItems(ui.Center).
		BorderWidth(1, 0, 0, 0).BorderColor(c.Theme().Border).Background(c.Theme().Surface).Children(func() {
		ui.Text(c, status).FontSize(11).TextColor(c.Theme().TextMuted)
		ui.Text(c, fmt.Sprintf("%d lines · %s", tab.state.Buffer.LineCount(), lang)).FontSize(11).TextColor(c.Theme().TextMuted)
		ui.Box(c).Grow(1)
		ui.Button(c, "Save").Height(20).OnClick(func() { a.Message = message(a.SaveActiveEditorFile()) })
	})
}
