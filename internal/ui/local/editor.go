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

type quickCommand struct {
	label    string
	detail   string
	shortcut string
	icon     *ui.SVG
	action   func()
}

type quickOpenEntry struct {
	section  string
	label    string
	detail   string
	shortcut string
	icon     *ui.SVG
	file     *quickOpenItem
	action   func()
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

// OpenQuickOpen shows the in-app command palette. It never calls a native
// dialog, so it cannot re-enter or block the view callback the way an OS sheet
// can.
func (a *App) OpenQuickOpen() {
	a.QuickOpenQuery = ""
	a.QuickOpenSelected = 0
	a.quickOpenRenderedQuery = ""
	a.quickOpenReveal = 0
	a.QuickOpen = true
	if len(a.RemoteRoots) > 0 {
		a.openQuickOpenAsync()
		return
	}
	roots := a.explorerRoots()
	if len(roots) == 0 {
		a.quickOpenItems = nil
		return
	}
	key := a.explorerRootsKey(roots)
	if a.quickOpenKey != key {
		a.quickOpenItems = listQuickOpenItems(roots)
		a.quickOpenKey = key
	}
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
	a.closeRemoteBrowser()
	a.SSHHost, a.SSHPath = "", ""
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
	if len(a.RemoteRoots) > 0 || a.remoteBrowser != nil {
		a.OpenRemoteFolderPicker()
		return
	}
	a.OpenLocalFolderDialog()
}

func (a *App) OpenLocalFolderDialog() {
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

func (a *App) quickOpenCommands() []quickCommand {
	commands := make([]quickCommand, 0, len(a.commandList()))
	for _, command := range a.commandList() {
		commands = append(commands, quickCommand{
			label: command.Label, detail: command.Detail, shortcut: shortcutLabel(a.bindingFor(command.ID)),
			icon: command.Icon, action: command.Run,
		})
	}
	return commands
}

func quickOpenQueryMatches(query string, values ...string) bool {
	if query == "" {
		return true
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}

func (a *App) quickOpenEntries() []quickOpenEntry {
	raw := strings.TrimSpace(a.QuickOpenQuery)
	commandsOnly := strings.HasPrefix(raw, ">")
	query := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(raw, ">")))
	entries := []quickOpenEntry{}
	for _, command := range a.quickOpenCommands() {
		if !quickOpenQueryMatches(query, command.label, command.detail) {
			continue
		}
		command := command
		entries = append(entries, quickOpenEntry{
			section: "Commands", label: command.label, detail: command.detail,
			shortcut: command.shortcut, icon: command.icon, action: command.action,
		})
	}
	if commandsOnly {
		return entries
	}
	fileCount := 0
	for i := range a.quickOpenItems {
		item := &a.quickOpenItems[i]
		if !quickOpenQueryMatches(query, item.label) {
			continue
		}
		dir, name := path.Split(filepath.ToSlash(item.label))
		dir = strings.TrimSuffix(dir, "/")
		if dir == "" {
			dir = "Workspace root"
		}
		entries = append(entries, quickOpenEntry{
			section: "Files", label: name, detail: dir, icon: changeFileIcon, file: item,
		})
		fileCount++
		if fileCount == 20 {
			break
		}
	}
	return entries
}

func (a *App) runQuickOpenEntry(entry quickOpenEntry) {
	a.QuickOpen = false
	if entry.file != nil {
		a.Message = message(a.openEditorRef(entry.file.ref))
		return
	}
	if entry.action != nil {
		entry.action()
	}
}

// quickOpenView renders the command palette with actions and files.
func (a *App) quickOpenView(c *ui.Context) {
	ui.Modal(c, &a.QuickOpen, func() {
		width, height := c.Size()
		paletteWidth := min(float32(680), max(float32(420), width-80))
		paletteHeight := min(float32(520), max(float32(300), height-100))
		ui.Column(c).Label("Quick open palette").Width(paletteWidth).MaxHeight(paletteHeight).Gap(0).
			Background(c.Theme().Background).Border(1, c.Theme().Border).Radius(12).
			Shadow(0, 18, 48, 0, ui.RGBA(0, 0, 0, 0.28)).Clip().Children(func() {
			var input ui.Element
			ui.Row(c).Height(54).FillWidth().Padding(0, 14).Gap(10).AlignItems(ui.Center).
				Background(c.Theme().Background).BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Children(func() {
				ui.Icon(c, searchIcon).Size(18, 18).TextColor(c.Theme().TextMuted).Shrink(0)
				input = ui.TextInput(c, &a.QuickOpenQuery).Label("Quick open").Placeholder("Search commands and files…").
					FillWidth().Grow(1).Height(40).FontSize(14).BorderWidth(0).Background(ui.Transparent).AutoFocus()
				ui.Text(c, "⌘P").FontSize(10).TextColor(c.Theme().TextMuted).Padding(3, 7).
					Background(c.Theme().Surface).Border(1, c.Theme().Border).Radius(5).Shrink(0)
			})
			entries := a.quickOpenEntries()
			if a.QuickOpenQuery != a.quickOpenRenderedQuery {
				a.QuickOpenSelected = 0
				a.quickOpenReveal = 0
				a.quickOpenRenderedQuery = a.QuickOpenQuery
			}
			if len(entries) == 0 {
				a.QuickOpenSelected = 0
			} else if a.QuickOpenSelected >= len(entries) {
				a.QuickOpenSelected = len(entries) - 1
			} else if a.QuickOpenSelected < 0 {
				a.QuickOpenSelected = 0
			}
			if input.Shortcut(0, ui.KeyDown) && len(entries) > 0 {
				a.QuickOpenSelected = min(a.QuickOpenSelected+1, len(entries)-1)
				a.quickOpenReveal = a.QuickOpenSelected
			}
			if input.Shortcut(0, ui.KeyUp) && len(entries) > 0 {
				a.QuickOpenSelected = max(a.QuickOpenSelected-1, 0)
				a.quickOpenReveal = a.QuickOpenSelected
			}
			if input.Submitted() && len(entries) > 0 {
				a.runQuickOpenEntry(entries[a.QuickOpenSelected])
				return
			}
			ui.Scroll(c).Label("Quick open results").Grow(1).MinHeight(0).FillWidth().MaxHeight(paletteHeight - 96).Children(func() {
				ui.Column(c).FillWidth().Padding(6).Gap(2).Children(func() {
					if len(entries) == 0 {
						ui.Column(c).FillWidth().Padding(24, 18).AlignItems(ui.Center).Gap(6).Children(func() {
							ui.Icon(c, searchIcon).Size(22, 22).TextColor(c.Theme().TextMuted)
							ui.Text(c, "No matching commands or files").FontSize(12).TextColor(c.Theme().TextMuted)
						})
						return
					}
					reveal := a.quickOpenReveal
					a.quickOpenReveal = -1
					section := ""
					for i, entry := range entries {
						if entry.section != section {
							section = entry.section
							ui.Text(c, strings.ToUpper(section)).FontSize(9).Bold().TextColor(c.Theme().TextMuted).Padding(8, 10, 4, 10)
						}
						entry := entry
						row := ui.ButtonBase(c).Label("Quick open "+entry.label).Tooltip(entry.detail).FillWidth().Height(42).
							Padding(8, 10).Gap(10).Radius(6).Cursor(ui.CursorPointer).AlignItems(ui.Center).Justify(ui.Start)
						if i == a.QuickOpenSelected {
							row.Background(c.Theme().Selection)
						} else if row.Hovered() {
							row.Background(c.Theme().SurfaceHover)
						} else {
							row.Background(ui.Transparent)
						}
						if i == reveal {
							row.ScrollIntoView()
						}
						row.OnClick(func() {
							a.QuickOpenSelected = i
							a.runQuickOpenEntry(entry)
						}).Children(func() {
							ui.Icon(c, entry.icon).Size(16, 16).TextColor(c.Theme().TextMuted).Shrink(0)
							ui.Column(c).Grow(1).MinWidth(0).AlignItems(ui.Start).Gap(1).Children(func() {
								ui.Text(c, entry.label).FontSize(12).TextColor(c.Theme().Text).NoWrap()
								ui.Text(c, entry.detail).FontSize(10).TextColor(c.Theme().TextMuted).NoWrap()
							})
							if entry.shortcut != "" {
								ui.Text(c, entry.shortcut).FontSize(10).TextColor(c.Theme().TextMuted).
									Padding(2, 6).Background(c.Theme().Surface).Border(1, c.Theme().Border).Radius(5).Shrink(0)
							}
						})
					}
				})
			})
			ui.Row(c).Height(34).FillWidth().Padding(0, 12).Gap(12).AlignItems(ui.Center).
				Background(c.Theme().Surface).BorderWidth(1, 0, 0, 0).BorderColor(c.Theme().Border).Children(func() {
				ui.Text(c, "↑↓ Navigate").FontSize(10).TextColor(c.Theme().TextMuted)
				ui.Text(c, "Enter Open").FontSize(10).TextColor(c.Theme().TextMuted)
				ui.Text(c, "Esc Close").FontSize(10).TextColor(c.Theme().TextMuted)
				ui.Box(c).Grow(1)
				ui.Text(c, "Type > for commands only").FontSize(10).TextColor(c.Theme().TextMuted)
			})
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
	return a.saveEditorTab(tab)
}

func (a *App) saveEditorTab(tab *editorTab) error {
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
	} else if err := writeFileAtomic(tab.ref.Path, []byte(text), 0); err != nil {
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
func (a *App) CloseEditorTab(i int) { a.requestCloseEditorTab(i) }

// editorSidebar is the Explorer view: open editors followed by the workspace
// folder tree. Open editors keep a bounded height so a long tab list scrolls
// instead of pushing the Explorer out of view.
func (a *App) editorSidebar(c *ui.Context) {
	ui.Column(c).Fill().MinHeight(0).Gap(0).Children(func() {
		if len(a.RemoteRoots) > 0 || a.remoteBrowser != nil {
			ui.Row(c).FillWidth().Padding(4, 8).Gap(6).Children(func() {
				if b := a.remoteBrowser; b != nil && b.loading && b.client == nil {
					ui.Text(c, "Connecting to "+b.host+"…").FontSize(11).Grow(1)
					ui.Button(c, "Cancel connection").Height(24).OnClick(func() {
						a.closeRemoteBrowser()
						a.OpenRemoteDialog()
					})
					return
				}
				host := a.SSHHost
				if host == "" && a.remoteBrowser != nil {
					host = a.remoteBrowser.host
				}
				ui.Text(c, "SSH: "+host).FontSize(11).Grow(1)
				ui.Button(c, "Open folder…").Height(24).OnClick(a.OpenRemoteFolderPicker)
			})
		}
		if len(a.EditorTabs) > 0 {
			ui.Text(c, "OPEN EDITORS").Bold().FontSize(10).TextColor(c.Theme().TextMuted).Padding(4, 8)
			ui.Scroll(c).Key("open-editors-scroll").Label("Open editors list").FillWidth().Shrink(0).
				MaxHeight(22 * 9).Children(func() {
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
			})
		}
		ui.Scroll(c).Key("explorer-scroll").Grow(1).FillWidth().MinHeight(0).Children(func() {
			roots := a.explorerRoots()
			if len(roots) > 0 {
				ui.Row(c).Height(30).FillWidth().Padding(0, 8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "EXPLORER").Bold().FontSize(10).TextColor(c.Theme().TextMuted).Grow(1)
					iconButton(c, "New file", "New file", plusIcon).
						Size(20, 20).OnClick(a.OpenNewFilePrompt)
					iconButton(c, "New folder", "New folder", folderIcon).
						Size(20, 20).OnClick(a.OpenNewFolderPrompt)
					iconButton(c, "Open file…", "Open a file with the system dialog", changeFileIcon).
						Size(20, 20).OnClick(a.OpenEditorFileDialog)
					iconButton(c, "Quick open…", "Quick open files (⌘P)", searchIcon).
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
					iconButton(c, "Open file…", "Open a file with the system dialog", changeFileIcon).Size(20, 20).OnClick(a.OpenEditorFileDialog)
					iconButton(c, "Quick open…", "Quick open files (⌘P)", searchIcon).Size(20, 20).OnClick(a.OpenQuickOpen)
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
	row.ContextMenu(func(m *ui.Menu) { a.explorerPathMenu(m, root.path, true) })
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
			row.ContextMenu(func(m *ui.Menu) { a.explorerPathMenu(m, child.path, true) })
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
			childRef = remoteFileRef(child.host, child.root, child.path)
		}
		active := activeRef.key() == childRef.key()
		row := ui.ButtonBase(c).Label(child.path).Tooltip(child.path).FillWidth().Height(22).
			Padding(0, float32(8+depth*12)).Radius(2).Cursor(ui.CursorPointer).Justify(ui.Start).
			AlignItems(ui.Center).OnClick(func() {
			a.Message = message(a.openEditorRef(childRef))
		})
		row.ContextMenu(func(m *ui.Menu) { a.explorerPathMenu(m, child.path, false) })
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
		tab.state.Font = ui.Font{Size: float32(a.Settings.EditorFontSize)}
		tab.state.SoftWrap = a.Settings.EditorWordWrap
		if a.Settings.EditorLineNumbers {
			tab.state.LineNumbers = editor.LineNumbersOn
		} else {
			tab.state.LineNumbers = editor.LineNumbersOff
		}
		editor.View(c, tab.state).Grow(1).FillWidth()
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
					OnClick(func() { a.requestCloseEditorTab(i) })
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

func (a *App) editorStatusItems(c *ui.Context, tab *editorTab) {
	pos := tab.state.CursorPos()
	lang := "Plain Text"
	if l := tab.state.Buffer.Language(); l != nil {
		lang = l.Name
	}
	status := fmt.Sprintf("Ln %d, Col %d", pos.Line+1, pos.Col+1)
	ui.Text(c, status).FontSize(11).TextColor(c.Theme().TextMuted)
	ui.Text(c, fmt.Sprintf("%d lines · %s", tab.state.Buffer.LineCount(), lang)).FontSize(11).TextColor(c.Theme().TextMuted)
	ui.Button(c, "Save").Height(20).OnClick(func() { a.Message = message(a.SaveActiveEditorFile()) })
}
