package local

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"gitviewer/internal/domain"
)

const (
	maxSearchFileSize = 2 << 20
	maxSearchMatches  = 5000
)

type searchMatch struct {
	Ref      fileRef
	Relative string
	Line     int
	Column   int
	Length   int
	Preview  string
}

type searchReplacementUpdate struct {
	ref  fileRef
	text string
}

func (a *App) OpenSearch() {
	a.SearchOpen = true
	a.SearchLoading = false
	a.SearchStatus = ""
	a.SearchSelected = 0
}

func (a *App) closeSearch() {
	if a.searchCancel != nil {
		a.searchCancel()
		a.searchCancel = nil
	}
	a.searchGeneration++
	a.SearchOpen = false
	a.SearchLoading = false
}

func (a *App) searchPattern() (*regexp.Regexp, error) {
	query := a.SearchQuery
	if query == "" {
		return nil, fmt.Errorf("enter text to search")
	}
	if !a.SearchRegex {
		query = regexp.QuoteMeta(query)
	}
	if !a.SearchCase {
		query = "(?i)" + query
	}
	re, err := regexp.Compile(query)
	if err != nil {
		return nil, fmt.Errorf("invalid regular expression")
	}
	return re, nil
}

func (a *App) RunWorkspaceSearch() {
	if a.searchCancel != nil {
		a.searchCancel()
	}
	a.searchGeneration++
	generation := a.searchGeneration
	re, err := a.searchPattern()
	if err != nil {
		a.SearchResults = nil
		a.SearchStatus = err.Error()
		a.SearchLoading = false
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.searchCancel = cancel
	a.SearchLoading = true
	a.SearchStatus = "Searching…"
	roots := append([]explorerRoot(nil), a.explorerRoots()...)
	include, exclude := a.SearchInclude, a.SearchExclude
	whole := a.SearchWhole
	limit := a.Settings.SearchMaxResults
	if limit <= 0 {
		limit = 500
	}
	dispatch := a.Dispatch
	search := func() ([]searchMatch, int, error) {
		return searchRoots(ctx, a, roots, re, whole, include, exclude, limit)
	}
	if dispatch == nil {
		results, scanned, searchErr := search()
		a.finishWorkspaceSearch(generation, results, scanned, searchErr)
		return
	}
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		results, scanned, searchErr := search()
		dispatch(func() {
			if a.closed || generation != a.searchGeneration {
				return
			}
			a.finishWorkspaceSearch(generation, results, scanned, searchErr)
		})
	}()
}

func (a *App) finishWorkspaceSearch(generation uint64, results []searchMatch, scanned int, err error) {
	if generation != a.searchGeneration {
		return
	}
	a.searchCancel = nil
	a.SearchLoading = false
	if err != nil {
		if err != context.Canceled {
			a.SearchStatus = "Search failed: " + message(err)
		}
		return
	}
	a.SearchResults = results
	a.SearchSelected = 0
	if len(results) == 0 {
		a.SearchStatus = fmt.Sprintf("No results in %d files", scanned)
		return
	}
	a.SearchStatus = fmt.Sprintf("%d results in %d files", len(results), countSearchFiles(results))
}

func countSearchFiles(matches []searchMatch) int {
	seen := map[string]bool{}
	for _, match := range matches {
		seen[match.Ref.key()] = true
	}
	return len(seen)
}

type searchCandidate struct {
	ref      fileRef
	relative string
	path     string
	client   domain.RemoteFileSystem
}

func searchRoots(ctx context.Context, app *App, roots []explorerRoot, re *regexp.Regexp, whole bool, include, exclude string, limit int) ([]searchMatch, int, error) {
	var candidates []searchCandidate
	for _, root := range roots {
		if root.tree == nil {
			continue
		}
		if root.tree.remote {
			client := app.remoteClient(root.tree.host, root.tree.path)
			if client == nil {
				continue
			}
			files, err := client.Walk(ctx, root.tree.path, quickOpenSkipDirs)
			if err != nil {
				if ctx.Err() != nil {
					return nil, 0, ctx.Err()
				}
				continue
			}
			for _, rel := range files {
				rel = filepath.ToSlash(rel)
				if !searchPathIncluded(rel, include, exclude) {
					continue
				}
				candidates = append(candidates, searchCandidate{
					ref:      remoteFileRef(root.tree.host, root.tree.path, path.Join(root.tree.path, rel)),
					relative: rel, path: path.Join(root.tree.path, rel), client: client,
				})
			}
			continue
		}
		for _, rel := range listQuickOpenFiles(root.tree.path) {
			if !searchPathIncluded(rel, include, exclude) {
				continue
			}
			full := filepath.Join(root.tree.path, filepath.FromSlash(rel))
			candidates = append(candidates, searchCandidate{
				ref: localFileRef(full, root.tree.path), relative: rel, path: full,
			})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return strings.ToLower(candidates[i].relative) < strings.ToLower(candidates[j].relative)
	})

	workerCount := min(4, max(1, len(candidates)))
	jobs := make(chan searchCandidate)
	results := make(chan []searchMatch, workerCount)
	var wg sync.WaitGroup
	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for candidate := range jobs {
				if ctx.Err() != nil {
					return
				}
				data, err := readSearchCandidate(ctx, candidate)
				if err != nil || len(data) == 0 || bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
					continue
				}
				matches := searchText(candidate.ref, candidate.relative, string(data), re, whole, limit)
				if len(matches) > 0 {
					results <- matches
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, candidate := range candidates {
			select {
			case jobs <- candidate:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	var all []searchMatch
	for batch := range results {
		remaining := limit - len(all)
		if remaining <= 0 {
			break
		}
		if len(batch) > remaining {
			batch = batch[:remaining]
		}
		all = append(all, batch...)
	}
	if ctx.Err() != nil {
		return nil, len(candidates), ctx.Err()
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].Relative != all[j].Relative {
			return strings.ToLower(all[i].Relative) < strings.ToLower(all[j].Relative)
		}
		if all[i].Line != all[j].Line {
			return all[i].Line < all[j].Line
		}
		return all[i].Column < all[j].Column
	})
	return all, len(candidates), nil
}

func readSearchCandidate(ctx context.Context, candidate searchCandidate) ([]byte, error) {
	if candidate.client != nil {
		return candidate.client.ReadFile(ctx, candidate.path, maxSearchFileSize)
	}
	linkInfo, err := os.Lstat(candidate.path)
	if err != nil || linkInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("search candidate unavailable")
	}
	file, err := os.Open(candidate.path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxSearchFileSize {
		return nil, fmt.Errorf("search candidate unavailable")
	}
	return io.ReadAll(io.LimitReader(file, maxSearchFileSize+1))
}

func searchText(ref fileRef, relative, text string, re *regexp.Regexp, whole bool, limit int) []searchMatch {
	var matches []searchMatch
	lines := strings.Split(text, "\n")
	for lineIndex, line := range lines {
		if len(matches) >= limit {
			break
		}
		for _, loc := range re.FindAllStringIndex(line, -1) {
			if loc[0] == loc[1] || whole && !wholeWordAt(line, loc[0], loc[1]) {
				continue
			}
			matches = append(matches, searchMatch{
				Ref: ref, Relative: filepath.ToSlash(relative), Line: lineIndex + 1, Column: loc[0] + 1,
				Length: loc[1] - loc[0], Preview: strings.TrimSpace(line),
			})
			if len(matches) >= limit {
				break
			}
		}
	}
	return matches
}

func wholeWordAt(text string, start, end int) bool {
	word := func(b byte) bool {
		return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
	}
	return (start == 0 || !word(text[start-1])) && (end == len(text) || !word(text[end]))
}

func searchPathIncluded(relative, include, exclude string) bool {
	relative = filepath.ToSlash(relative)
	if include != "" && !searchGlobList(include, relative) {
		return false
	}
	return exclude == "" || !searchGlobList(exclude, relative)
}

func searchGlobList(patterns, relative string) bool {
	for _, pattern := range strings.Split(patterns, ",") {
		pattern = strings.TrimSpace(filepath.ToSlash(pattern))
		if pattern == "" {
			continue
		}
		if searchGlobMatch(pattern, relative) {
			return true
		}
	}
	return false
}

func searchGlobMatch(pattern, relative string) bool {
	var out strings.Builder
	out.WriteByte('^')
	if strings.HasPrefix(pattern, "**/") {
		out.WriteString("(?:.*/)?")
		pattern = strings.TrimPrefix(pattern, "**/")
	} else if !strings.Contains(pattern, "/") {
		out.WriteString("(?:.*/)?")
	}
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				out.WriteString(".*")
				i++
			} else {
				out.WriteString("[^/]*")
			}
		case '?':
			out.WriteString("[^/]")
		default:
			out.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	out.WriteByte('$')
	re, err := regexp.Compile(out.String())
	return err == nil && re.MatchString(relative)
}

func (a *App) openSearchMatch(match searchMatch) {
	a.closeSearch()
	if err := a.openEditorRef(match.Ref); err != nil {
		a.Message = message(err)
		return
	}
	a.goToActiveEditor(match.Line, match.Column)
}

func (a *App) searchView(c *ui.Context) {
	ui.Modal(c, &a.SearchOpen, func() {
		width, height := c.Size()
		w := min(float32(900), max(float32(560), width-80))
		h := min(float32(700), max(float32(420), height-80))
		ui.Column(c).Label("Search in files").Width(w).MaxHeight(h).Gap(0).
			Background(c.Theme().Background).Border(1, c.Theme().Border).Radius(10).Clip().Children(func() {
			ui.Row(c).Height(50).FillWidth().Padding(0, 12).Gap(8).AlignItems(ui.Center).
				Background(c.Theme().Surface).BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Children(func() {
				ui.Icon(c, searchIcon).Size(16, 16).TextColor(c.Theme().TextMuted)
				input := ui.TextInput(c, &a.SearchQuery).Label("Search text").Placeholder("Search workspace…").
					FillWidth().Grow(1).Height(32).AutoFocus()
				if input.Submitted() {
					a.RunWorkspaceSearch()
				}
				ui.Button(c, "Find").Height(28).Disabled(a.SearchLoading).OnClick(a.RunWorkspaceSearch)
				ui.Button(c, "Close").Height(28).OnClick(a.closeSearch)
			})
			ui.Row(c).FillWidth().Padding(8, 12).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Checkbox(c, &a.SearchCase, "Aa")
				ui.Checkbox(c, &a.SearchRegex, ".*")
				ui.Checkbox(c, &a.SearchWhole, "Word")
				ui.Button(c, "Replace").Height(24).OnClick(func() { a.SearchReplaceOpen = !a.SearchReplaceOpen })
				ui.Box(c).Grow(1)
				ui.Text(c, "files to include").FontSize(10).TextColor(c.Theme().TextMuted)
				ui.TextInput(c, &a.SearchInclude).Label("Files to include").Placeholder("*.go, src/**").
					Width(150).Height(25).FontSize(11)
				ui.Text(c, "exclude").FontSize(10).TextColor(c.Theme().TextMuted)
				ui.TextInput(c, &a.SearchExclude).Label("Files to exclude").Placeholder("vendor/**, *.min.js").
					Width(150).Height(25).FontSize(11)
			})
			if a.SearchReplaceOpen {
				ui.Row(c).FillWidth().Padding(0, 12, 8, 12).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Replace with").FontSize(11).TextColor(c.Theme().TextMuted)
					ui.TextInput(c, &a.SearchReplace).Label("Replacement text").FillWidth().Grow(1).Height(27)
					ui.Button(c, "Replace").Height(25).Disabled(len(a.SearchResults) == 0).OnClick(func() {
						a.replaceSearchMatches(false)
					})
					ui.Button(c, "Replace All").Height(25).Disabled(len(a.SearchResults) == 0).OnClick(func() {
						a.OpenReplaceAllConfirmation()
					})
				})
			}
			if a.SearchLoading || a.SearchStatus != "" {
				ui.Row(c).FillWidth().Height(25).Padding(0, 12).AlignItems(ui.Center).
					BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Children(func() {
					ui.Text(c, a.SearchStatus).FontSize(10).TextColor(c.Theme().TextMuted)
				})
			}
			ui.Scroll(c).Label("Search results").Grow(1).MinHeight(0).FillWidth().Children(func() {
				ui.Column(c).FillWidth().Padding(5).Gap(1).Children(func() {
					if len(a.SearchResults) == 0 {
						ui.Column(c).FillWidth().Padding(24, 18).AlignItems(ui.Center).Gap(6).Children(func() {
							ui.Icon(c, searchIcon).Size(22, 22).TextColor(c.Theme().TextMuted)
							ui.Text(c, "Run a search across the Explorer roots").FontSize(12).TextColor(c.Theme().TextMuted)
						})
						return
					}
					lastFile := ""
					for i, match := range a.SearchResults {
						match := match
						if match.Ref.key() != lastFile {
							lastFile = match.Ref.key()
							ui.Text(c, match.Relative).Bold().FontSize(11).TextColor(c.Theme().Text).
								Padding(6, 8, 2, 8).NoWrap()
						}
						row := ui.ButtonBase(c).Label(fmt.Sprintf("%s:%d", match.Relative, match.Line)).
							Tooltip(match.Preview).FillWidth().Height(30).Padding(8, 22).Radius(4).
							Cursor(ui.CursorPointer).AlignItems(ui.Center).Justify(ui.Start).
							OnClick(func() { a.openSearchMatch(match) })
						if i == a.SearchSelected {
							row.Background(c.Theme().Selection)
						} else if row.Hovered() {
							row.Background(c.Theme().SurfaceHover)
						}
						row.Children(func() {
							ui.Text(c, fmt.Sprintf("%d", match.Line)).FontSize(10).TextColor(c.Theme().TextMuted).Width(42)
							ui.Text(c, compactLabel(match.Preview, 120)).FontSize(11).TextColor(c.Theme().TextMuted).NoWrap().Grow(1)
						})
					}
				})
			})
		})
	})
}

func (a *App) replaceSearchMatches(all bool) {
	re, err := a.searchPattern()
	if err != nil {
		a.SearchStatus = err.Error()
		return
	}
	if !all {
		if a.SearchSelected < 0 || a.SearchSelected >= len(a.SearchResults) {
			a.SearchStatus = "Select a result to replace"
			return
		}
	}
	targets := map[string]searchMatch{}
	if all {
		for _, match := range a.SearchResults {
			targets[match.Ref.key()] = match
		}
	} else {
		match := a.SearchResults[a.SearchSelected]
		targets[match.Ref.key()] = match
	}
	replacement := a.SearchReplace
	whole := a.SearchWhole
	regexMode := a.SearchRegex
	dirtyTabs := map[string]bool{}
	for _, tab := range a.EditorTabs {
		if tab.state.Buffer.Modified() {
			dirtyTabs[tab.ref.key()] = true
		}
	}
	type replacementResult struct {
		count   int
		updates []searchReplacementUpdate
		errs    []error
	}
	dispatch := a.Dispatch
	apply := func() replacementResult {
		replaced := 0
		var errs []error
		var updates []searchReplacementUpdate
		for _, match := range targets {
			changed, text, err := a.replaceInFile(match.Ref, match, re, replacement, whole, regexMode, !all, dirtyTabs[match.Ref.key()])
			if err != nil {
				errs = append(errs, err)
				continue
			}
			replaced += changed
			if changed > 0 {
				updates = append(updates, searchReplacementUpdate{ref: match.Ref, text: text})
			}
		}
		return replacementResult{count: replaced, updates: updates, errs: errs}
	}
	if dispatch == nil {
		result := apply()
		a.finishSearchReplace(result.count, result.errs, result.updates)
		return
	}
	a.SearchStatus = "Replacing…"
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		result := apply()
		dispatch(func() {
			if !a.closed {
				a.finishSearchReplace(result.count, result.errs, result.updates)
			}
		})
	}()
}

func (a *App) finishSearchReplace(replaced int, errs []error, updates []searchReplacementUpdate) {
	for _, update := range updates {
		for _, tab := range a.EditorTabs {
			if tab.ref.key() != update.ref.key() {
				continue
			}
			tab.original = editor.NewBuffer(update.text)
			tab.original.SetLanguage(editor.LanguageFor(update.ref.Path))
			tab.state.Buffer.Set(update.text)
			tab.state.Buffer.MarkSaved()
			tab.state.DiffBase = tab.original
			break
		}
	}
	if len(errs) > 0 {
		a.SearchStatus = fmt.Sprintf("Replaced %d occurrences; %d files failed", replaced, len(errs))
		a.Message = "Some files could not be replaced"
	} else {
		a.SearchStatus = fmt.Sprintf("Replaced %d occurrences", replaced)
		a.Message = a.SearchStatus
	}
	a.resetExplorerCache()
	a.RunWorkspaceSearch()
}

func (a *App) replaceInFile(ref fileRef, match searchMatch, re *regexp.Regexp, replacement string, whole, regexMode, currentOnly, dirty bool) (int, string, error) {
	if dirty {
		return 0, "", fmt.Errorf("%s has unsaved changes", match.Relative)
	}
	var data []byte
	var mode os.FileMode = 0o644
	if ref.Location == domain.LocationSSH {
		client := a.remoteClient(ref.Host, ref.Root)
		if client == nil {
			return 0, "", fmt.Errorf("remote connection is unavailable")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		info, err := client.Stat(ctx, ref.Path)
		if err != nil {
			return 0, "", err
		}
		mode = info.Mode
		data, err = client.ReadFile(ctx, ref.Path, maxSearchFileSize)
		if err != nil {
			return 0, "", err
		}
	} else {
		info, err := os.Stat(ref.Path)
		if err != nil {
			return 0, "", err
		}
		mode = info.Mode()
		data, err = os.ReadFile(ref.Path)
		if err != nil {
			return 0, "", err
		}
	}
	text := string(data)
	count := 0
	if currentOnly {
		lines := strings.SplitAfter(text, "\n")
		if match.Line < 1 || match.Line > len(lines) {
			return 0, "", fmt.Errorf("match is no longer available")
		}
		line := lines[match.Line-1]
		start := match.Column - 1
		end := start + match.Length
		if start < 0 || end > len(line) || !re.MatchString(line[start:end]) {
			return 0, "", fmt.Errorf("match changed on disk")
		}
		replacementText := replacement
		if regexMode {
			replacementText = re.ReplaceAllString(line[start:end], replacement)
		}
		lines[match.Line-1] = line[:start] + replacementText + line[end:]
		text = strings.Join(lines, "")
		count = 1
	} else {
		var replaced strings.Builder
		last := 0
		for _, loc := range re.FindAllStringIndex(text, -1) {
			if whole && !wholeWordAt(text, loc[0], loc[1]) {
				continue
			}
			replaced.WriteString(text[last:loc[0]])
			if regexMode {
				replaced.Write(re.ExpandString(nil, replacement, text, loc))
			} else {
				replaced.WriteString(replacement)
			}
			last = loc[1]
			count++
		}
		replaced.WriteString(text[last:])
		text = replaced.String()
	}
	if text == string(data) {
		return 0, text, nil
	}
	if ref.Location == domain.LocationSSH {
		client := a.remoteClient(ref.Host, ref.Root)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := client.WriteFileAtomic(ctx, ref.Path, []byte(text), mode); err != nil {
			return 0, "", err
		}
	} else if err := writeFileAtomic(ref.Path, []byte(text), mode); err != nil {
		return 0, "", err
	}
	return count, text, nil
}

func (a *App) OpenReplaceAllConfirmation() {
	a.FilePromptOpen = true
	a.FilePromptKind = "replace-all"
	a.FilePromptTitle = "Replace all matches?"
	a.FilePromptDetail = fmt.Sprintf("Replace %d visible matches in the current workspace. Open files with unsaved changes are skipped.", len(a.SearchResults))
	a.FilePromptPath = "Current workspace"
	a.FilePromptValue = ""
	a.FilePromptError = ""
}
