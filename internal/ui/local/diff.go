package local

import (
	"fmt"
	"path"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

// diff.go owns the Inspector: unified diff preview, commit details header,
// copy support and the diff find bar. It is mounted by both the Commit and
// Git Log modes.

// diffFind holds the FindBar state for one App. The App field set is a frozen
// shared contract, so the state lives beside the App, keyed by pointer.
type diffFind struct {
	open            bool
	query           string
	current         int
	sideBySide      bool
	text            string
	cachedSide      bool
	rows            []diffRow
	indices         []int
	files           ui.ListState
	repository      string
	fileQuery       string
	searchOpen      bool
	settingsOpen    bool
	hideLineNumbers bool
	showWhitespace  bool
	checked         map[string]*bool
	allChecked      bool
}

var diffFinds sync.Map // *App -> *diffFind

func (a *App) findState() *diffFind {
	if v, ok := diffFinds.Load(a); ok {
		return v.(*diffFind)
	}
	v, _ := diffFinds.LoadOrStore(a, &diffFind{})
	return v.(*diffFind)
}

func (a *App) diffView(c *ui.Context) {
	if a.Mode == "log" {
		a.historyDiffView(c)
		return
	}
	ui.Column(c).Fill().Children(func() {
		a.commitDiffHeader(c)
		ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
			find := a.findState()
			if a.Preview.Scope != "" && !a.Preview.Limited && a.Preview.Text != "" && (find.text != a.Preview.Text || find.cachedSide != find.sideBySide || find.rows == nil) {
				find.rows, find.indices = diffRows(a.lines, find.sideBySide)
				find.text, find.cachedSide = a.Preview.Text, find.sideBySide
			}
			a.diffControlsView(c)
			if a.fileHistory.open {
				a.FileHistoryView(c, &a.fileHistory)
				return
			}
			if a.Preview.Scope == "" {
				ui.Text(c, "Select a changed file to review its diff").Padding(12).TextColor(c.Theme().TextMuted)
				return
			}
			if a.CommitDetails.Commit.OID != "" {
				a.commitDetailsView(c)
			}
			if a.Preview.Limited {
				ui.Text(c, "Large diff · preview limit exceeded (512 KiB / 10,000 lines)").Padding(12)
				return
			}
			if a.Preview.Binary {
				ui.Text(c, "Binary file · textual preview unavailable").Padding(12)
			}
			if a.Preview.Text == "" {
				ui.Text(c, "No textual changes").Padding(12)
				return
			}
			a.DiffBuiltRows = 0
			width := a.diffWidth + 100
			if find.sideBySide {
				width = 2 * (a.diffWidth + 50)
			}
			ui.ScrollHorizontal(c).TrackScroll(&a.DiffScroll).Grow(1).FillWidth().Children(func() {
				ui.List(c, &a.DiffList, len(find.rows), func(i int) {
					a.DiffBuiltRows++
					a.diffRowView(c, find.rows[i], find.sideBySide, width)
				}).Width(width).MinWidthPercent(100).FillHeight()
			})
		})
		a.diffFilesView(c)
	})
}

func (a *App) commitDiffHeader(c *ui.Context) {
	ui.Row(c).Height(48).Padding(8, 12).Gap(10).FillWidth().Background(c.Theme().Surface).Children(func() {
		if a.Preview.Path == "" {
			ui.Text(c, "Diff Preview").Bold().FontSize(13).Grow(1)
		} else {
			ui.Icon(c, changeFileIcon).Size(16, 16).Shrink(0)
			ui.Text(c, displayPath(path.Base(a.Preview.Path))).Bold().FontSize(13).SingleLine().MaxWidth(200).MinWidth(0).Shrink(1).Tooltip(pathLabel(a.Preview.Path))
			ui.Text(c, displayPath(a.Preview.Path)).FontSize(11).TextColor(c.Theme().TextMuted).SingleLine().Grow(1).MinWidth(0).Tooltip(pathLabel(a.Preview.Path))
		}
		for _, entry := range a.Entries {
			if entry.Scope == a.Preview.Scope && entry.Change.Path == a.Preview.Path {
				status, color := diffEntryStatus(c, entry)
				ui.Text(c, status).Bold().FontSize(11).TextColor(color)
				break
			}
		}
		ui.Text(c, string(a.Preview.Scope)).FontSize(10).TextColor(c.Theme().TextMuted)
		ui.Button(c, "Open in Editor").Shrink(0).Disabled(a.Preview.Path == "" || a.Repository.Repository.Location != "local").Tooltip("Open the worktree file in the default text editor").OnClick(func() { a.OpenInEditor(a.Preview.Path) })
		ui.Button(c, "⋮").Label("File actions").Shrink(0).Disabled(true).Tooltip("More file actions are not available yet")
	})
}

func diffEntryStatus(c *ui.Context, entry domain.StatusEntry) (string, ui.Color) {
	status, color := entry.Change.WorktreeStatus, c.Theme().Warning
	if entry.Scope == domain.StagedDiff {
		status = entry.Change.IndexStatus
	}
	if entry.Change.Untracked {
		status, color = "U", c.Theme().Success
	} else if entry.Change.Conflicted {
		status, color = "!", c.Theme().Danger
	} else if status == "A" {
		color = c.Theme().Success
	}
	return status, color
}

// selectedDiffEntries reconciles checked rows with the current repository and
// status. Each entry retains its scope and group, including duplicate paths.
func (a *App) selectedDiffEntries() []domain.StatusEntry {
	state := a.findState()
	repo := a.Repository.Repository.RepositoryID
	if state.repository != repo || repo == "" {
		state.repository = repo
		state.checked = make(map[string]*bool)
		state.files = ui.ListState{}
	}
	if state.checked == nil {
		state.checked = make(map[string]*bool)
	}
	present := make(map[string]bool, len(a.Entries))
	var selected []domain.StatusEntry
	for _, entry := range a.Entries {
		key := entryKey(entry)
		present[key] = true
		if checked := state.checked[key]; checked != nil && *checked {
			selected = append(selected, entry)
		}
	}
	for key := range state.checked {
		if !present[key] {
			delete(state.checked, key)
		}
	}
	return selected
}

func (a *App) diffFilesView(c *ui.Context) {
	state := a.findState()
	selected := a.selectedDiffEntries()
	ui.Column(c).FillWidth().Background(c.Theme().Surface).Children(func() {
		ui.ScrollHorizontal(c).Height(38).FillWidth().Children(func() {
			ui.Row(c).Height(38).Padding(4, 12).Gap(8).Children(func() {
				ui.Text(c, "Files").Bold().FontSize(12).TextColor(c.Theme().Accent)
				ui.Textf(c, "%d/%d", len(selected), len(a.Entries)).FontSize(11).TextColor(c.Theme().TextMuted)
				ui.MenuButton(c, "Selected actions ▾", func(m *ui.Menu) {
					if m.Item("Stage selected").Disabled(a.syncDisabled() || len(selected) == 0).Chosen() {
						a.runSelectedChanges(domain.Stage)
					}
					if m.Item("Unstage selected").Disabled(a.syncDisabled() || len(selected) == 0).Chosen() {
						a.runSelectedChanges(domain.Unstage)
					}
					if m.Item("Discard selected").Disabled(a.syncDisabled() || len(selected) == 0).Chosen() {
						a.runSelectedChanges(domain.Discard)
					}
				}).Disabled(len(selected) == 0)
				ui.Button(c, "Console").OnClick(func() { a.consoleOpen = !a.consoleOpen })
				if ui.Button(c, "Search").Clicked() {
					state.searchOpen = !state.searchOpen
					if !state.searchOpen {
						state.fileQuery = ""
					}
				}
			})
		})
		if a.consoleOpen {
			a.consoleView(c)
			return
		}
		if state.searchOpen {
			ui.TextInput(c, &state.fileQuery).Label("Search changed files").Placeholder("Search changed files…").FillWidth()
		}
		if len(a.Entries) == 0 {
			ui.Text(c, "No changed files").FontSize(11).TextColor(c.Theme().TextMuted).Padding(6, 12)
			return
		}
		query := strings.ToLower(state.fileQuery)
		visible := make([]int, 0, len(a.Entries))
		selectedIndex := -1
		for i, entry := range a.Entries {
			if strings.Contains(strings.ToLower(entry.Change.Path), query) {
				if i == a.Selected {
					selectedIndex = len(visible)
				}
				visible = append(visible, i)
			}
		}
		if len(visible) == 0 {
			ui.Text(c, "No matching changed files").FontSize(11).TextColor(c.Theme().TextMuted).Padding(6, 12)
			return
		}
		state.allChecked = true
		visibleChecks := make([]*bool, 0, len(visible))
		for _, index := range visible {
			key := entryKey(a.Entries[index])
			if state.checked[key] == nil {
				state.checked[key] = new(bool)
			}
			visibleChecks = append(visibleChecks, state.checked[key])
			if !*state.checked[key] {
				state.allChecked = false
			}
		}
		ui.Row(c).Height(24).Padding(2, 12).Gap(12).FillWidth().Children(func() {
			all := ui.Checkbox(c, &state.allChecked, "").Label("Select all visible files")
			if all.Changed() {
				for _, checked := range visibleChecks {
					*checked = state.allChecked
				}
			}
			ui.Text(c, "Status").Width(48).FontSize(10).TextColor(c.Theme().TextMuted)
			ui.Text(c, "File").FontSize(10).TextColor(c.Theme().TextMuted).Grow(1)
			ui.Text(c, "+/−").FontSize(10).TextColor(c.Theme().TextMuted).Width(48)
		})
		files := &state.files
		files.Selected = &selectedIndex
		files.Key = func(i int) any { return entryKey(a.Entries[visible[i]]) }
		files.Label = func(i int) string { return pathLabel(a.Entries[visible[i]].Change.Path) }
		baseTheme := c.Theme()
		listTheme := *baseTheme
		listTheme.Accent = baseTheme.SurfacePressed
		listTheme.AccentText = baseTheme.Text
		c.SetTheme(&listTheme)
		list := ui.List(c, files, len(visible), func(i int) {
			index := visible[i]
			entry := a.Entries[index]
			status, color := diffEntryStatus(c, entry)
			row := ui.Row(c).Label("Diff file "+entry.Group+" "+pathLabel(entry.Change.Path)).Height(30).Padding(4, 12).Gap(12).FillWidth().Tooltip(pathLabel(entry.Change.Path))
			if a.Selected == index {
				row.Background(c.Theme().SurfacePressed)
			}
			row.Children(func() {
				ui.Checkbox(c, state.checked[entryKey(entry)], "").Label("Select " + entry.Group + " " + pathLabel(entry.Change.Path))
				ui.Row(c).Gap(12).Grow(1).Cursor(ui.CursorPointer).OnClick(func() { a.Select(index) }).Children(func() {
					ui.Text(c, status).Bold().FontSize(11).TextColor(color).Width(48)
					ui.Icon(c, changeFileIcon).Size(16, 16).Shrink(0)
					ui.Text(c, displayPath(path.Base(entry.Change.Path))).FontSize(11).SingleLine().MaxWidth(180).MinWidth(0).Shrink(1).Tooltip(pathLabel(entry.Change.Path))
					ui.Text(c, displayPath(entry.Change.Path)).FontSize(10).TextColor(c.Theme().TextMuted).SingleLine().Grow(1).MinWidth(0).Tooltip(pathLabel(entry.Change.Path))
					ui.Text(c, entry.Group).FontSize(10).TextColor(c.Theme().TextMuted)
					ui.Text(c, "—").FontSize(10).TextColor(c.Theme().TextMuted).Width(48).Tooltip("Line counts are not available")
				})
			})
		}).Height(120).FillWidth()
		c.SetTheme(baseTheme)
		if list.Changed() && selectedIndex >= 0 && selectedIndex < len(visible) && visible[selectedIndex] != a.Selected {
			a.Select(visible[selectedIndex])
		}
	})
}

func (a *App) historyDiffView(c *ui.Context) {
	ui.Column(c).Fill().Padding(8).Gap(6).Children(func() {
		ui.Text(c, "Diff Preview").Bold().FontSize(12)
		if a.Preview.Scope == "" {
			ui.Text(c, "Select a changed file to review its diff").TextColor(c.Theme().TextMuted)
			return
		}
		if a.CommitDetails.Commit.OID != "" && a.Mode != "log" {
			a.commitDetailsView(c)
		}
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, displayPath(a.Preview.Path)).FontSize(12).Selectable().Grow(1)
			ui.Text(c, string(a.Preview.Scope)).FontSize(10).TextColor(c.Theme().TextMuted)
		})
		if a.Preview.Limited {
			ui.Text(c, "Large diff · preview limit exceeded (512 KiB / 10,000 lines)")
			return
		}
		if a.Preview.Binary {
			ui.Text(c, "Binary file · textual preview unavailable")
		}
		if a.Preview.Text == "" {
			ui.Text(c, "No textual changes")
			return
		}
		find := a.findState()
		if find.text != a.Preview.Text || find.cachedSide != find.sideBySide || find.rows == nil {
			find.rows, find.indices = diffRows(a.lines, find.sideBySide)
			find.text, find.cachedSide = a.Preview.Text, find.sideBySide
		}
		a.diffControlsView(c)
		if a.fileHistory.open {
			a.FileHistoryView(c, &a.fileHistory)
			return
		}
		a.DiffBuiltRows = 0
		width := a.diffWidth + 100
		if find.sideBySide {
			width = 2 * (a.diffWidth + 50)
		}
		ui.ScrollHorizontal(c).TrackScroll(&a.DiffScroll).Grow(1).FillWidth().Children(func() {
			ui.List(c, &a.DiffList, len(find.rows), func(i int) {
				a.DiffBuiltRows++
				a.diffRowView(c, find.rows[i], find.sideBySide, width)
			}).Width(width).MinWidthPercent(100).FillHeight()
		})
	})
}

func diffNumber(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprint(n)
}

func (a *App) diffRowView(c *ui.Context, row diffRow, sideBySide bool, width float32) {
	settings := a.findState()
	display := func(text string) string {
		if !settings.showWhitespace {
			return text
		}
		return strings.NewReplacer(" ", "·", "\t", "→   ").Replace(text)
	}
	if sideBySide && !row.shared {
		ui.Row(c).Height(22).FillWidth().Children(func() {
			for _, cell := range []diffCell{row.left, row.right} {
				text := ""
				if cell.source >= 0 {
					text = display(cell.text)
					if !settings.hideLineNumbers {
						text = fmt.Sprintf("%5s  %s", diffNumber(cell.line), text)
					}
				}
				foreground, background := diffCellColors(c.Theme(), cell)
				ui.Text(c, text).Font("monospace").FontSize(12).NoWrap().Selectable().Height(22).
					Width(width/2).Padding(0, 8).TextColor(foreground).Background(background)
			}
		})
		return
	}
	cell := row.left
	if cell.source < 0 {
		cell = row.right
	}
	text := display(cell.text)
	if !settings.hideLineNumbers && !sideBySide && (row.left.line != 0 || row.right.line != 0) {
		text = fmt.Sprintf("%5s %5s  %s", diffNumber(row.left.line), diffNumber(row.right.line), text)
	}
	foreground, background := diffCellColors(c.Theme(), cell)
	ui.Text(c, text).Font("monospace").FontSize(12).NoWrap().Selectable().Height(22).
		FillWidth().Padding(0, 8).TextColor(foreground).Background(background)
}

func diffCellColors(theme *ui.Theme, cell diffCell) (ui.Color, ui.Color) {
	if cell.kind == '+' {
		return diffLineColors(theme, "+")
	}
	if cell.kind == '-' {
		return diffLineColors(theme, "-")
	}
	return diffLineColors(theme, cell.text)
}

// diffLineColors uses restrained editor-style fills; code retains the theme's
// normal foreground instead of turning entire added/removed lines blue/red.
func diffLineColors(theme *ui.Theme, line string) (ui.Color, ui.Color) {
	added, removed, hunk := ui.Hex("#e6f2e3"), ui.Hex("#fbe8ea"), ui.Hex("#edf2fa")
	hunkText := ui.Hex("#526f96")
	if theme.Dark {
		added, removed, hunk = ui.Hex("#24372b"), ui.Hex("#3c262b"), ui.Hex("#263343")
		hunkText = ui.Hex("#a2bad8")
	}
	switch {
	case strings.HasPrefix(line, "@@"):
		return hunkText, hunk
	case strings.HasPrefix(line, "diff --git "), strings.HasPrefix(line, "index "),
		strings.HasPrefix(line, "--- "), strings.HasPrefix(line, "+++ "),
		strings.HasPrefix(line, "\\ No newline"):
		return theme.TextMuted, theme.Surface
	case strings.HasPrefix(line, "+"):
		return theme.Text, added
	case strings.HasPrefix(line, "-"):
		return theme.Text, removed
	default:
		return theme.Text, theme.Background
	}
}

// commitDetailsView groups the commit header: author, time, message and the
// first changed files. All data is read-only state already loaded on App.
func (a *App) commitDetailsView(c *ui.Context) {
	ui.Column(c).Gap(2).FillWidth().Children(func() {
		ui.Text(c, "COMMIT DETAILS").Bold()
		ui.Textf(c, "Author: %s", a.CommitDetails.Commit.Author).Selectable()
		ui.Textf(c, "Authored: %s", a.CommitDetails.Commit.AuthoredAt.Format("2006-01-02 15:04:05")).Selectable()
		ui.Scroll(c).MaxHeight(100).FillWidth().Children(func() {
			ui.Text(c, a.CommitDetails.Message).Selectable()
		})
		ui.Textf(c, "Changed files: %d", len(a.CommitDetails.Files))
		for i, file := range a.CommitDetails.Files {
			if i == 5 {
				ui.Text(c, "More files are shown in the diff below")
				break
			}
			ui.Text(c, file.Status+" "+pathLabel(file.Path)).Selectable()
		}
	})
}

// diffControlsView mounts copy, the find toggle and the FindBar. The FindBar
// state lives in a.findState(); a.lines is searched case-insensitively and a
// match move scrolls the diff list to that line.
func (a *App) diffControlsView(c *ui.Context) {
	find := a.findState()
	ui.ScrollHorizontal(c).Height(36).FillWidth().Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, "Diff").Bold().FontSize(12).TextColor(c.Theme().Accent).Padding(8)
			ui.Button(c, "File History").Disabled(a.Preview.Path == "" || a.Repository.Repository.RepositoryID == "").OnClick(func() { a.OpenFileHistory(&a.fileHistory, a.Preview.Path) })
			ui.Button(c, "Blame").Disabled(a.Preview.Path == "" || a.Repository.Repository.RepositoryID == "").OnClick(func() { a.OpenFileBlame(&a.fileHistory, a.Preview.Path) })
			unified := ui.Button(c, "Unified")
			if !find.sideBySide {
				unified.Background(c.Theme().Accent).TextColor(c.Theme().AccentText)
			}
			if unified.Clicked() && find.sideBySide {
				find.sideBySide = false
				a.DiffList = ui.ListState{}
			}
			side := ui.Button(c, "Side-by-Side")
			if find.sideBySide {
				side.Background(c.Theme().Accent).TextColor(c.Theme().AccentText)
			}
			if side.Clicked() && !find.sideBySide {
				find.sideBySide = true
				a.DiffList = ui.ListState{}
			}
			if ui.Button(c, "Diff settings").Clicked() {
				find.settingsOpen = !find.settingsOpen
			}
			if ui.Button(c, "Copy diff").Disabled(a.Preview.Scope == "" || a.Preview.Limited || a.Preview.Text == "").Clicked() {
				c.WriteClipboard(a.Preview.Text)
			}
			if ui.Button(c, "Find").Disabled(a.Preview.Scope == "" || a.Preview.Limited || a.Preview.Text == "").Clicked() {
				find.open = true
			}
		})
	})
	if find.settingsOpen {
		ui.Row(c).Gap(12).Padding(4, 8).Children(func() {
			ui.Checkbox(c, &find.hideLineNumbers, "Hide line numbers")
			ui.Checkbox(c, &find.showWhitespace, "Show whitespace")
		})
	}
	var matches []int
	if q := strings.ToLower(find.query); q != "" {
		for i, line := range a.lines {
			if strings.Contains(strings.ToLower(line), q) {
				matches = append(matches, i)
			}
		}
	}
	bar := ui.FindBar(c, &find.open, &find.query, len(matches), &find.current)
	if bar.Changed() && find.text == a.Preview.Text && !a.Preview.Limited && len(matches) > 0 && matches[min(find.current, len(matches)-1)] < len(find.indices) {
		a.DiffList.ScrollTo(find.indices[matches[min(find.current, len(matches)-1)]], ui.Center)
	}
}
