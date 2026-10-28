package local

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
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
}

// editorTab is one open file: the saved text as the diff base and the live
// editor state, kept across frames like the other panels' widgets.
type editorTab struct {
	path     string
	original *editor.Buffer
	state    *editor.State
}

func (t *editorTab) name() string { return filepath.Base(t.path) }

func newEditorTab(path, text string) *editorTab {
	lang := editor.LanguageFor(path)
	original := editor.NewBuffer(text)
	original.SetLanguage(lang)
	buffer := editor.NewBuffer(text)
	buffer.SetLanguage(lang)
	return &editorTab{path: path, original: original, state: &editor.State{Buffer: buffer, DiffBase: original}}
}

// OpenEditorFile opens a file in the editor area, or activates its tab when it
// is already open, the way editors reuse tabs for the same document.
func (a *App) OpenEditorFile(path string) error {
	for i, tab := range a.EditorTabs {
		if tab.path == path {
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
	a.EditorTabs = append(a.EditorTabs, newEditorTab(path, string(text)))
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
	root := a.folderRoot()
	if root == "" {
		a.Message = "Open a folder or repository to search its files"
		return
	}
	if a.quickOpenRepo != root {
		a.quickOpenFiles = listQuickOpenFiles(root)
		a.quickOpenRepo = root
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
	a.FolderPath = path
	a.folderTree = nil
	a.folderTreeRoot = ""
	a.quickOpenRepo = ""
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
			}
		}
	})
}

func (a *App) explorerTree() *folderNode {
	root := a.folderRoot()
	if root == "" {
		return nil
	}
	if a.folderTree != nil && a.folderTreeRoot == root {
		return a.folderTree
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
	a.folderTree = rootNode
	a.folderTreeRoot = root
	return rootNode
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

func (a *App) quickOpenMatches() []string {
	query := strings.ToLower(strings.TrimSpace(a.QuickOpenQuery))
	matches := make([]string, 0, 20)
	for _, path := range a.quickOpenFiles {
		if query != "" && !strings.Contains(strings.ToLower(path), query) {
			continue
		}
		matches = append(matches, path)
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
				a.Message = message(a.OpenEditorFile(filepath.Join(a.quickOpenRepo, matches[0])))
				return
			}
			ui.Scroll(c).MaxHeight(340).FillWidth().Children(func() {
				ui.Column(c).FillWidth().Gap(2).Children(func() {
					if len(matches) == 0 {
						ui.Text(c, "No matching files").TextColor(c.Theme().TextMuted).Padding(8)
						return
					}
					for _, rel := range matches {
						rel := rel
						ui.ButtonBase(c).Label(rel).Tooltip(rel).FillWidth().Height(28).Radius(2).
							Cursor(ui.CursorPointer).AlignItems(ui.Center).
							OnClick(func() {
								a.QuickOpen = false
								a.Message = message(a.OpenEditorFile(filepath.Join(a.quickOpenRepo, rel)))
							}).Children(func() {
							ui.Text(c, rel).NoWrap().TextColor(c.Theme().Text)
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
	if err := os.WriteFile(tab.path, []byte(text), 0o644); err != nil {
		return err
	}
	saved := newEditorTab(tab.path, text)
	tab.original = saved.original
	tab.state.DiffBase = saved.original
	tab.state.Buffer.MarkSaved()
	return nil
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

// editorSidebar is the Explorer view: open editors and the repository's
// changed files, one click away from a tab.
func (a *App) editorSidebar(c *ui.Context) {
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().Padding(8).Gap(8).Children(func() {
			ui.Button(c, "Open file…").OnClick(func() { a.OpenQuickOpen() })
			ui.Text(c, "OPEN EDITORS").Bold().FontSize(11).TextColor(c.Theme().TextMuted)
			if len(a.EditorTabs) == 0 {
				ui.Text(c, "No open files").TextColor(c.Theme().TextMuted)
			}
			for i, tab := range a.EditorTabs {
				label := tab.name()
				if tab.state.Buffer.Modified() {
					label += " ●"
				}
				active := i == a.EditorActive
				item := ui.ButtonBase(c).Label(label).Tooltip(tab.path).FillWidth().Height(26).
					Radius(2).Cursor(ui.CursorPointer).AlignItems(ui.Center).
					OnClick(func() { a.EditorActive = i })
				color := c.Theme().Text
				if active {
					item.Background(c.Theme().Selection)
				} else if item.Hovered() {
					item.Background(c.Theme().SurfaceHover)
				}
				item.Children(func() {
					ui.Text(c, label).TextColor(color).NoWrap().Shrink(1)
				})
			}
			if tree := a.explorerTree(); tree != nil {
				ui.Text(c, "EXPLORER").Bold().FontSize(11).TextColor(c.Theme().TextMuted)
				ui.Text(c, tree.name).FontSize(11).TextColor(c.Theme().TextMuted).NoWrap()
				a.folderTreeRows(c, tree, 0)
			}
			ui.Text(c, "CHANGED FILES").Bold().FontSize(11).TextColor(c.Theme().TextMuted)
			seen := map[string]bool{}
			shown := 0
			for _, entry := range a.Entries {
				path := strings.TrimPrefix(entry.Change.Path, `"`)
				if seen[path] {
					continue
				}
				seen[path] = true
				shown++
				full := filepath.Join(a.Repository.Repository.Path, path)
				ui.ButtonBase(c).Label(path).Tooltip(path).FillWidth().Height(26).
					Radius(2).Cursor(ui.CursorPointer).AlignItems(ui.Center).
					OnClick(func() { a.Message = message(a.OpenEditorFile(full)) }).
					Children(func() {
						ui.Text(c, path).TextColor(c.Theme().Text).NoWrap().Shrink(1)
					})
			}
			if shown == 0 {
				ui.Text(c, "No changes").TextColor(c.Theme().TextMuted)
			}
		})
	})
}

func (a *App) folderTreeRows(c *ui.Context, node *folderNode, depth int) {
	for _, child := range node.children {
		child := child
		if child.dir {
			expanded := a.folderExpanded[child.path]
			mark := "▾"
			if !expanded {
				mark = "▸"
			}
			ui.ButtonBase(c).Label(child.path).Tooltip(child.path).FillWidth().Height(24).
				Padding(0, float32(depth*12)).Radius(2).Cursor(ui.CursorPointer).
				AlignItems(ui.Center).OnClick(func() {
				if a.folderExpanded == nil {
					a.folderExpanded = map[string]bool{}
				}
				a.folderExpanded[child.path] = !expanded
			}).Children(func() {
				ui.Text(c, mark+" "+child.name).TextColor(c.Theme().Text).NoWrap()
			})
			if expanded {
				a.folderTreeRows(c, child, depth+1)
			}
			continue
		}
		ui.ButtonBase(c).Label(child.path).Tooltip(child.path).FillWidth().Height(24).
			Padding(0, float32(depth*12)).Radius(2).Cursor(ui.CursorPointer).
			AlignItems(ui.Center).OnClick(func() {
			a.Message = message(a.OpenEditorFile(child.path))
		}).Children(func() {
			ui.Text(c, child.name).TextColor(c.Theme().Text).NoWrap()
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
				item := ui.ButtonBase(c).Label(name).Tooltip(tab.path).Height(35).
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
