package local

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/domain"
)

// App and its callbacks belong to the UI thread. Workers return immutable data
// through Dispatch. Generation checks discard results from cancelled/superseded reads.
type App struct {
	Service         application.LocalService
	Dispatch        func(func())
	ChooseDirectory func() (string, error)
	PathInput       string
	CommitInput     string
	Repository      domain.RepositoryContext
	Recent          []domain.Repository
	Status          domain.GitStatus
	Preview         domain.DiffPreview
	Entries         []domain.StatusEntry
	Selected        int
	Loading         bool
	Stale           bool
	Message         string
	StoreWarning    bool
	SidebarWidth    float32
	StatusWidth     float32
	StatusList      ui.ListState
	DiffScroll      ui.ScrollState
	DiffList        ui.ListState
	DiffBuiltRows   int
	diffWidth       float32
	cancel          context.CancelFunc
	generation      uint64
	closed          bool
	workers         sync.WaitGroup
	lines           []string
}

func New(service application.LocalService) *App {
	a := &App{Service: service, Selected: -1, SidebarWidth: 240, StatusWidth: 350, Message: "Open a local repository"}
	var err error
	a.Recent, err = service.Recent.Load()
	a.StoreWarning = err != nil
	return a
}
func message(err error) string {
	var ge *domain.GitError
	if errors.As(err, &ge) {
		return string(ge.Code)
	}
	return "Operation failed"
}
func (a *App) start(work func(context.Context) (func(), error)) {
	if a.closed || a.Dispatch == nil {
		return
	}
	a.Cancel()
	a.generation++
	generation := a.generation
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.Loading = true
	a.Message = "Loading"
	dispatch := a.Dispatch
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		apply, err := work(ctx)
		dispatch(func() {
			if a.closed || generation != a.generation {
				return
			}
			a.cancel = nil
			cancel()
			a.Loading = false
			if err != nil {
				a.Stale = true
				a.Message = message(err)
				return
			}
			a.Message = "Ready"
			apply()
		})
	}()
}
func (a *App) Open(path string) {
	service := a.Service
	a.start(func(ctx context.Context) (func(), error) {
		result, err := service.Open(ctx, path)
		if err != nil {
			return nil, err
		}
		status, err := service.Status(ctx, result.Repository)
		if err != nil {
			return nil, err
		}
		return func() {
			a.Repository = result.Repository
			a.Recent = result.Recent
			a.StoreWarning = result.PersistenceError
			a.PathInput = result.Repository.Repository.Path
			a.CommitInput = ""
			a.applyStatus(status)
		}, nil
	})
}
func (a *App) applyStatus(status domain.GitStatus) {
	a.Status = status
	a.Entries = domain.StatusEntries(status)
	a.Stale = false
	a.Selected = -1
	a.StatusList = ui.ListState{}
	a.Preview = domain.DiffPreview{}
	a.lines = nil
	a.DiffScroll = ui.ScrollState{}
}
func (a *App) Refresh() {
	if a.Repository.Repository.RepositoryID == "" {
		return
	}
	service, repo := a.Service, a.Repository
	a.start(func(ctx context.Context) (func(), error) {
		status, err := service.Status(ctx, repo)
		return func() { a.applyStatus(status) }, err
	})
}
func (a *App) Select(index int) {
	if index < 0 || index >= len(a.Entries) {
		return
	}
	entry := a.Entries[index]
	a.Selected = index
	a.loadDiff(domain.DiffRequest{Scope: entry.Scope, Path: entry.Change.Path})
}
func (a *App) CommitDiff() {
	a.Selected = -1
	a.loadDiff(domain.DiffRequest{Scope: domain.CommitDiff, Commit: strings.TrimSpace(a.CommitInput)})
}
func (a *App) loadDiff(request domain.DiffRequest) {
	service, repo := a.Service, a.Repository
	a.Preview = domain.DiffPreview{}
	a.lines = nil
	a.start(func(ctx context.Context) (func(), error) {
		preview, err := service.Diff(ctx, repo, request)
		return func() {
			a.Preview = preview
			a.lines = strings.Split(preview.Text, "\n")
			a.diffWidth = 600
			for _, line := range a.lines {
				a.diffWidth = max(a.diffWidth, float32(utf8.RuneCountInString(line))*16+32)
			}
			a.DiffList = ui.ListState{}
			a.DiffScroll = ui.ScrollState{}
		}, err
	})
}
func (a *App) Cancel() {
	a.generation++
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	if a.Loading {
		a.Loading = false
		a.Stale = true
		a.Message = "OperationCancelled"
	}
}
func (a *App) Close() { a.Cancel(); a.closed = true }
func (a *App) Wait()  { a.workers.Wait() }
func (a *App) Choose() {
	if a.ChooseDirectory == nil {
		return
	}
	path, err := a.ChooseDirectory()
	if err != nil {
		a.Message = "Cannot open directory dialog"
		return
	}
	if path != "" {
		a.Open(path)
	}
}
func pathLabel(path string) string { return strconv.QuoteToGraphic(path) }
func (a *App) View(c *ui.Context) {
	c.OnShortcut(ui.Cmd, ui.KeyR, a.Refresh)
	c.OnShortcut(ui.Cmd, ui.KeyO, a.Choose)
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(8).Gap(8).Children(func() {
			ui.Text(c, "Git Viewer · Local").Bold()
			ui.Button(c, "Choose repository…").OnClick(a.Choose)
			ui.TextInput(c, &a.PathInput).Label("Repository path").Placeholder("Local directory path").Grow(1)
			ui.Button(c, "Open path").OnClick(func() { a.Open(a.PathInput) })
			ui.Button(c, "Refresh").Disabled(a.Repository.Repository.RepositoryID == "").OnClick(a.Refresh)
			ui.Button(c, "Cancel").Disabled(!a.Loading).OnClick(a.Cancel)
		})
		ui.Textf(c, "%s · loading=%t · stale=%t · fetched=%s", a.Message, a.Loading, a.Stale, a.Status.FetchedAt).Selectable().Padding(4)
		if a.StoreWarning {
			ui.Text(c, "Recent repositories could not be read or saved. Check application configuration.").TextColor(c.Theme().Danger)
		}
		ui.Split(c, &a.SidebarWidth, func() { a.sidebar(c) }, func() {
			ui.Split(c, &a.StatusWidth, func() { a.statusView(c) }, func() { a.diffView(c) }).Fill()
		}).Grow(1).FillWidth()
	})
}
func (a *App) sidebar(c *ui.Context) {
	ui.Column(c).Fill().Padding(8).Gap(8).Children(func() {
		ui.Text(c, "Recent local repositories").Bold()
		if len(a.Recent) == 0 {
			ui.Text(c, "No recent repositories")
		}
		ui.Scroll(c).Grow(1).FillWidth().Children(func() {
			ui.Column(c).FillWidth().Gap(6).Children(func() {
				for i, repo := range a.Recent {
					ui.Button(c.Key(i), pathLabel(repo.Path)).OnClick(func() { a.Open(repo.Path) })
				}
			})
		})
		if a.Repository.Repository.RepositoryID != "" {
			ui.Text(c, pathLabel(a.Repository.Repository.Path)).Selectable()
		}
		ui.Text(c, "Read only · Cmd+R refresh")
	})
}
func (a *App) statusView(c *ui.Context) {
	ui.Column(c).Fill().Padding(6).Children(func() {
		ui.Text(c, "Status").Bold()
		if a.Repository.Repository.RepositoryID == "" {
			ui.Text(c, "Open a repository to view changes")
			return
		}
		if len(a.Entries) == 0 {
			ui.Text(c, "Working tree clean")
			return
		}
		for _, group := range []string{"Staged", "Unstaged", "Untracked", "Conflicted"} {
			count := 0
			for _, entry := range a.Entries {
				if entry.Group == group {
					count++
				}
			}
			ui.Textf(c, "%s: %d", group, count)
		}
		a.StatusList.Selected = &a.Selected
		a.StatusList.Key = func(i int) any { e := a.Entries[i]; return e.Group + "\x00" + e.Change.Path }
		a.StatusList.Label = func(i int) string { e := a.Entries[i]; return e.Group + " " + pathLabel(e.Change.Path) }
		// The list owns pointer and keyboard selection for each file row.
		list := ui.List(c, &a.StatusList, len(a.Entries), func(i int) {
			entry := a.Entries[i]
			label := fmt.Sprintf("%s [%s%s] %s", entry.Group, entry.Change.IndexStatus, entry.Change.WorktreeStatus, pathLabel(entry.Change.Path))
			if entry.Change.OldPath != nil {
				label += " ← " + pathLabel(*entry.Change.OldPath)
			}
			ui.Text(c, label).Height(32).Padding(4).NoWrap()
		}).Grow(1).FillWidth()
		if list.Changed() {
			a.Select(a.Selected)
		}
	})
}
func (a *App) diffView(c *ui.Context) {
	ui.Column(c).Fill().Padding(8).Gap(6).Children(func() {
		ui.Text(c, "Unified Diff").Bold()
		ui.Row(c).Gap(6).Children(func() {
			ui.TextInput(c, &a.CommitInput).Label("Commit object ID").Placeholder("Full commit object ID").Grow(1)
			ui.Button(c, "View commit diff").Disabled(a.Repository.Repository.RepositoryID == "").OnClick(a.CommitDiff)
		})
		if a.Preview.Scope == "" {
			ui.Text(c, "Select a changed file or enter a commit object ID")
			return
		}
		ui.Textf(c, "%s · %s", a.Preview.Scope, pathLabel(a.Preview.Path)).Selectable()
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
		if ui.Button(c, "Copy diff").Clicked() {
			c.WriteClipboard(a.Preview.Text)
		}
		a.DiffBuiltRows = 0
		ui.ScrollHorizontal(c).TrackScroll(&a.DiffScroll).Grow(1).FillWidth().Children(func() {
			ui.List(c, &a.DiffList, len(a.lines), func(i int) {
				a.DiffBuiltRows++
				line := a.lines[i]
				text := ui.Text(c, line).Font("monospace").NoWrap().Selectable().Height(20)
				if strings.HasPrefix(line, "+") {
					text.TextColor(c.Theme().Accent)
				} else if strings.HasPrefix(line, "-") {
					text.TextColor(c.Theme().Danger)
				} else if strings.HasPrefix(line, "@@") {
					text.TextColor(c.Theme().TextMuted)
				}
			}).Width(a.diffWidth).FillHeight()
		})
	})
}
