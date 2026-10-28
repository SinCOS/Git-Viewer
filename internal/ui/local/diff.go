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
	cachedSingle    bool
	rows            []diffRow
	indices         []int
	repository      string
	settingsOpen    bool
	hideLineNumbers bool
	showWhitespace  bool
	checked         map[string]*bool
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
	ui.Column(c).Label("Commit diff pane").Fill().Children(func() {
		a.diffToolbar(c, false)
		pane := ui.Column(c).Grow(1).FillWidth().MinHeight(0)
		pane.Children(func() {
			find := a.findState()
			single := a.Preview.Path != ""
			if a.Preview.Scope != "" && !a.Preview.Limited && a.Preview.Text != "" && (find.text != a.Preview.Text || find.cachedSide != find.sideBySide || find.cachedSingle != single || find.rows == nil) {
				find.rows, find.indices = diffRows(a.lines, find.sideBySide)
				if a.Preview.Path != "" {
					find.rows, find.indices = singleFileRows(find.rows, find.indices)
				}
				find.text, find.cachedSide, find.cachedSingle = a.Preview.Text, find.sideBySide, single
			}
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
			if find.sideBySide {
				a.diffSideHeaders(c)
			}
			a.DiffBuiltRows = 0
			width := a.diffWidth + 100
			if find.sideBySide {
				width = 2 * (a.diffWidth + 50)
			}
			width = max(width, pane.Bounds().W)
			ui.ScrollHorizontal(c).TrackScroll(&a.DiffScroll).Grow(1).FillWidth().Children(func() {
				ui.List(c, &a.DiffList, len(find.rows), func(i int) {
					a.DiffBuiltRows++
					a.diffRowView(c, find.rows[i], find.sideBySide, width)
				}).Width(width).FillHeight()
			})
		})
	})
}

// diffToolbar is the single VS Code-style diff editor title bar. It replaces
// the previous commit header + controls pair, keeping the file identity, mode
// switch and settings in one place.
func (a *App) diffToolbar(c *ui.Context, history bool) {
	find := a.findState()
	ui.Row(c).Label("Diff toolbar").Height(40).Shrink(0).Padding(0, 8).Gap(4).
		AlignItems(ui.Center).FillWidth().Background(c.Theme().Surface).
		BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Children(func() {
		if a.Preview.Path == "" {
			ui.Text(c, "Diff Preview").Bold().FontSize(12).Padding(0, 6).Grow(1)
		} else {
			ui.Row(c).Gap(6).Grow(1).MinWidth(0).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, changeFileIcon).Size(16, 16).Shrink(0).TextColor(c.Theme().TextMuted)
				ui.Text(c, displayPath(path.Base(a.Preview.Path))).Bold().FontSize(12).
					SingleLine().MaxWidth(260).MinWidth(0).Shrink(1).Tooltip(pathLabel(a.Preview.Path))
				ui.Text(c, displayPath(path.Dir(a.Preview.Path))).FontSize(11).TextColor(c.Theme().TextMuted).
					SingleLine().MinWidth(0).Shrink(1).Tooltip(pathLabel(a.Preview.Path))
				a.diffFileStatusBadge(c)
			})
		}
		if history {
			a.diffTextAction(c, "File History", a.Preview.Path == "" || a.Repository.Repository.RepositoryID == "", func() {
				a.OpenFileHistory(&a.fileHistory, a.Preview.Path)
			})
			a.diffTextAction(c, "Blame", a.Preview.Path == "" || a.Repository.Repository.RepositoryID == "", func() {
				a.OpenFileBlame(&a.fileHistory, a.Preview.Path)
			})
		}
		a.diffModeAction(c, "Unified", unifiedDiffIcon, !find.sideBySide, func() {
			find.sideBySide = false
			a.DiffList = ui.ListState{}
		})
		a.diffModeAction(c, "Side-by-Side", splitDiffIcon, find.sideBySide, func() {
			find.sideBySide = true
			a.DiffList = ui.ListState{}
		})
		if iconButton(c, "Diff settings", "Diff settings", settingsIcon).Clicked() {
			find.settingsOpen = !find.settingsOpen
		}
		if !history {
			iconButton(c, "Open in Editor", "Open the worktree file in the editor", changeFileIcon).
				Disabled(a.Preview.Path == "" || a.Repository.Repository.Location != "local").
				OnClick(func() { a.OpenInEditor(a.Preview.Path) })
		}
	})
	if find.settingsOpen {
		a.diffSettingsView(c)
	}
	a.diffFindBar(c)
}

func (a *App) diffFileStatusBadge(c *ui.Context) {
	for _, entry := range a.Entries {
		if entry.Scope == a.Preview.Scope && entry.Change.Path == a.Preview.Path {
			status, color := diffEntryStatus(c, entry)
			ui.Box(c).Size(18, 18).Radius(3).Background(c.Theme().Surface.Mix(color, 0.14)).Center().Shrink(0).Children(func() {
				ui.Text(c, status).Bold().Font("monospace").FontSize(10).TextColor(color)
			})
			return
		}
	}
	if status := a.commitFileStatus(a.Preview.Path); status != "" {
		ui.Text(c, status).Bold().Font("monospace").FontSize(10).
			TextColor(commitStatusColor(c, status)).Shrink(0)
	}
}

func (a *App) diffTextAction(c *ui.Context, label string, disabled bool, action func()) {
	item := ui.ButtonBase(c).Label(label).Height(24).Padding(0, 7).Radius(3).
		Cursor(ui.CursorPointer).AlignItems(ui.Center).Disabled(disabled).OnClick(action)
	if item.Hovered() {
		item.Background(c.Theme().SurfaceHover)
	}
	item.Children(func() { ui.Text(c, label).FontSize(11).TextColor(c.Theme().TextMuted).NoWrap() })
}

func (a *App) diffModeAction(c *ui.Context, label string, icon *ui.SVG, active bool, action func()) {
	item := iconButton(c, label, label, icon).Size(26, 26).Radius(3)
	if active {
		item.Background(c.Theme().SurfacePressed)
	} else if item.Hovered() {
		item.Background(c.Theme().SurfaceHover)
	}
	if item.Clicked() {
		action()
	}
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

func (a *App) historyDiffView(c *ui.Context) {
	pane := ui.Column(c).Label("History diff").Fill().Gap(0)
	pane.Children(func() {
		a.diffToolbar(c, true)
		if a.Preview.Scope == "" {
			ui.Text(c, "Select a changed file to review its diff").Padding(12).TextColor(c.Theme().TextMuted)
			return
		}
		find := a.findState()
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
		single := a.Preview.Path != ""
		if find.text != a.Preview.Text || find.cachedSide != find.sideBySide || find.cachedSingle != single || find.rows == nil {
			find.rows, find.indices = diffRows(a.lines, find.sideBySide)
			if a.Preview.Path != "" {
				find.rows, find.indices = singleFileRows(find.rows, find.indices)
			}
			find.text, find.cachedSide, find.cachedSingle = a.Preview.Text, find.sideBySide, single
		}
		if a.fileHistory.open {
			a.FileHistoryView(c, &a.fileHistory)
			return
		}
		if find.sideBySide {
			a.diffSideHeaders(c)
		}
		a.DiffBuiltRows = 0
		width := a.diffWidth + 100
		if find.sideBySide {
			width = 2 * (a.diffWidth + 50)
		}
		width = max(width, pane.Bounds().W)
		ui.ScrollHorizontal(c).TrackScroll(&a.DiffScroll).Grow(1).FillWidth().Children(func() {
			ui.List(c, &a.DiffList, len(find.rows), func(i int) {
				a.DiffBuiltRows++
				a.diffRowView(c, find.rows[i], find.sideBySide, width)
			}).Width(width).FillHeight()
		})
	})
}

func (a *App) commitFileStatus(path string) string {
	for _, file := range a.CommitDetails.Files {
		if file.Path == path {
			return file.Status
		}
	}
	return ""
}

func commitStatusColor(c *ui.Context, status string) ui.Color {
	switch {
	case strings.HasPrefix(status, "A"):
		return c.Theme().Success
	case strings.HasPrefix(status, "D"):
		return c.Theme().Danger
	case strings.HasPrefix(status, "R"), strings.HasPrefix(status, "C"):
		return c.Theme().Warning
	default:
		return c.Theme().Accent
	}
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
			a.diffSideCell(c, row.left, width/2, settings, display)
			ui.Box(c).Width(1).FillHeight().Background(c.Theme().Border).Shrink(0)
			a.diffSideCell(c, row.right, width/2, settings, display)
		})
		return
	}
	cell := row.left
	if cell.source < 0 {
		cell = row.right
	}
	foreground, background := diffCellColors(c.Theme(), cell)
	if sideBySide || row.left.line == 0 && row.right.line == 0 {
		padding := float32(8)
		if !sideBySide && !settings.hideLineNumbers && diffHunkHeader.MatchString(cell.text) {
			padding = 96
		}
		ui.Text(c, display(cell.text)).Font("monospace").FontSize(12).NoWrap().Selectable().Height(22).
			FillWidth().Padding(0, padding).TextColor(foreground).Background(background)
		return
	}
	ui.Row(c).Height(22).FillWidth().Background(background).Children(func() {
		if !settings.hideLineNumbers {
			a.diffGutter(c, row.left.line, background)
			a.diffGutter(c, row.right.line, background)
		}
		marker, text := diffLineBody(cell.text)
		ui.Text(c, marker).Font("monospace").FontSize(12).Width(20).TextColor(foreground)
		ui.Text(c, display(text)).Font("monospace").FontSize(12).NoWrap().Selectable().Grow(1).TextColor(foreground)
	})
}

func (a *App) diffSideCell(c *ui.Context, cell diffCell, width float32, settings *diffFind, display func(string) string) {
	foreground, background := diffCellColors(c.Theme(), cell)
	ui.Row(c).Width(width).Height(22).Background(background).Children(func() {
		if cell.source < 0 {
			ui.Box(c).Fill().Background(background)
			return
		}
		if !settings.hideLineNumbers {
			a.diffGutter(c, cell.line, background)
		}
		marker, text := diffLineBody(cell.text)
		ui.Text(c, marker).Font("monospace").FontSize(12).Width(18).
			Height(22).Padding(0, 3).TextColor(foreground)
		ui.Text(c, display(text)).Font("monospace").FontSize(12).NoWrap().Selectable().
			Height(22).Padding(0, 4).Grow(1).TextColor(foreground)
	})
}

func (a *App) diffGutter(c *ui.Context, number int, background ui.Color) {
	gutter := background.Mix(c.Theme().Border, 0.42)
	ui.Text(c, diffNumber(number)).Font("monospace").FontSize(11).Width(46).
		Height(22).Padding(0, 6).TextColor(c.Theme().TextMuted).Background(gutter)
}

func diffLineBody(line string) (string, string) {
	if len(line) == 0 || line[0] != '+' && line[0] != '-' && line[0] != ' ' {
		return "", line
	}
	marker := ""
	if line[0] != ' ' {
		marker = line[:1]
	}
	return marker, line[1:]
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
	added, removed, hunk := ui.Hex("#e6ffec"), ui.Hex("#ffebe9"), ui.Hex("#f6f8fa")
	hunkText := ui.Hex("#526f96")
	if theme.Dark {
		added, removed, hunk = ui.Hex("#1f3d2b"), ui.Hex("#3d2024"), ui.Hex("#22272e")
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

// diffSettingsView is the compact settings strip opened from the diff title
// bar. It stays attached to the title bar so the document keeps the full area.
func (a *App) diffSettingsView(c *ui.Context) {
	find := a.findState()
	ui.Row(c).Label("Diff settings strip").Height(34).FillWidth().Padding(0, 12).Gap(14).
		AlignItems(ui.Center).Background(c.Theme().Surface).
		BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Children(func() {
		ui.Checkbox(c, &find.hideLineNumbers, "Hide line numbers")
		ui.Checkbox(c, &find.showWhitespace, "Show whitespace")
		ui.Box(c).Grow(1)
		if ui.Button(c, "Copy diff").Height(24).Disabled(a.Preview.Scope == "" || a.Preview.Limited || a.Preview.Text == "").Clicked() {
			c.WriteClipboard(a.Preview.Text)
		}
		if ui.Button(c, "Find").Height(24).Disabled(a.Preview.Scope == "" || a.Preview.Limited || a.Preview.Text == "").Clicked() {
			find.open = true
		}
	})
}

// diffFindBar mounts the native FindBar and scrolls the virtual diff list to
// the active match. a.lines is searched case-insensitively.
func (a *App) diffFindBar(c *ui.Context) {
	find := a.findState()
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

func (a *App) diffSideHeaders(c *ui.Context) {
	left, right := "Original", "Modified"
	switch a.Preview.Scope {
	case domain.CommitDiff:
		left, right = "Parent", "Commit"
	case domain.StagedDiff:
		left, right = "HEAD", "Index"
	case domain.WorktreeDiff:
		left, right = "Index", "Working Tree"
	}
	ui.Row(c).Label("Diff side headers").Height(28).FillWidth().
		Background(c.Theme().Surface).BorderWidth(0, 0, 1, 0).
		BorderColor(c.Theme().Border).Children(func() {
		a.diffSideHeaderCell(c, left)
		ui.Box(c).Width(1).FillHeight().Background(c.Theme().Border).Shrink(0)
		a.diffSideHeaderCell(c, right)
	})
}

func (a *App) diffSideHeaderCell(c *ui.Context, label string) {
	ui.Row(c).Grow(1).FillHeight().Padding(0, 10).Gap(6).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, changeFileIcon).Size(14, 14).TextColor(c.Theme().TextMuted).Shrink(0)
		ui.Text(c, label).FontSize(11).Bold().TextColor(c.Theme().TextMuted).NoWrap()
		if a.Preview.Path != "" {
			ui.Text(c, displayPath(path.Base(a.Preview.Path))).FontSize(11).
				TextColor(c.Theme().TextMuted).NoWrap().Shrink(1)
		}
	})
}
