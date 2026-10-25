package local

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
	"gitviewer/internal/infrastructure/ssh"
	"gitviewer/internal/infrastructure/workspace"
)

// App and its callbacks belong to the UI thread. Workers return immutable data
// through Dispatch. Generation checks discard results from cancelled/superseded reads.
type App struct {
	Service               application.LocalService
	ChooseWorkspace       func() (string, error)
	ChooseWorkspaceSave   func() (string, error)
	Confirm               func(string, string) bool
	SSHHost               string
	SSHPath               string
	SSHHosts              []string
	Locations             []domain.SavedLocation
	Pane                  string
	Mode                  string
	RepoMenuOpen          bool
	railPane              string
	HistoryPage           gitexec.HistoryPage
	CommitDetails         gitexec.CommitDetails
	Graph                 domain.GraphLayout
	GraphRows             []domain.GraphRow
	HistoryList           ui.ListState
	HistorySelected       int
	HistoryBuiltRows      int
	HistoryScroll         ui.ScrollState
	HistoryLanes          int
	Branches              []gitexec.BranchRecord
	Worktrees             []domain.WorktreeRecord
	BranchInput           string
	MessageInput          string
	Writing               bool
	DialogOpen            bool
	dialogCancel          context.CancelFunc
	Workspace             domain.WorkspaceView
	WorkspaceFolders      []domain.WorkspaceFolder
	WorkspaceRepositories []domain.RepositoryContext
	WorkspaceInput        string
	FolderInput           string
	EnableAuto            bool
	watchCancel           context.CancelFunc
	autoResults           chan error
	autoBackground        chan bool
	background            bool

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
	a := &App{Service: service, Selected: -1, HistorySelected: -1, SidebarWidth: 240, StatusWidth: 350, Mode: "commit", Pane: "Status", Message: "Open a repository"}
	a.SSHHosts, _ = ssh.Hosts(service.Remote.ConfigPath)
	var err error
	a.Recent, err = service.Recent.Load()
	a.StoreWarning = err != nil
	var locationErr error
	a.Locations, locationErr = service.Locations().Load()
	a.StoreWarning = a.StoreWarning || locationErr != nil
	a.loadPrefs()
	return a
}
func message(err error) string {
	if errors.Is(err, workspace.ErrConflict) {
		return "WorkspaceConflict · reload or save as a new file"
	}
	if errors.Is(err, workspace.ErrInvalid) {
		return "InvalidWorkspace · cannot preserve this document safely"
	}
	var ge *domain.GitError
	if errors.As(err, &ge) {
		if ge.Reason != "" {
			return string(ge.Code) + " · " + ge.Reason
		}
		return string(ge.Code)
	}
	return "Operation failed"
}
func (a *App) start(work func(context.Context) (func(), error)) {
	if a.closed || a.Dispatch == nil || a.Writing || a.DialogOpen {
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
			if a.autoResults != nil {
				select {
				case a.autoResults <- err:
				default:
				}
			}
			if err != nil {
				a.Stale = true
				if a.Writing {
					a.Writing = false
				}
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
			a.resetHistory()
			a.startAuto()
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
func (a *App) Refresh() { a.refresh(false) }
func (a *App) refresh(preserve bool) {
	if a.Repository.Repository.RepositoryID == "" {
		return
	}
	service, repo := a.Service, a.Repository
	previous := a.Preview
	statusList, diffList, diffScroll := a.StatusList, a.DiffList, a.DiffScroll
	request := domain.DiffRequest{Scope: previous.Scope, Path: previous.Path, Commit: a.CommitDetails.Commit.OID}
	a.start(func(ctx context.Context) (func(), error) {
		status, err := service.Status(ctx, repo)
		if err != nil {
			return nil, err
		}
		preview := previous
		selection := -1
		if preserve && request.Scope != "" && request.Scope != domain.CommitDiff {
			for i, entry := range domain.StatusEntries(status) {
				if entry.Change.Path == request.Path && entry.Scope == request.Scope {
					selection = i
					break
				}
			}
			if selection >= 0 {
				preview, err = service.Diff(ctx, repo, request)
				if err != nil {
					return nil, err
				}
			} else {
				preview = domain.DiffPreview{}
			}
		}
		return func() {
			a.applyStatus(status)
			if preserve {
				a.StatusList = statusList
				a.Selected = selection
				a.applyPreview(preview)
				if preview.Text == previous.Text {
					a.DiffList = diffList
					a.DiffScroll = diffScroll
				}
			}
		}, nil
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
	a.CommitDetails = gitexec.CommitDetails{}
	a.lines = nil
	a.start(func(ctx context.Context) (func(), error) {
		var details gitexec.CommitDetails
		if request.Scope == domain.CommitDiff {
			var err error
			details, err = service.CommitDetails(ctx, repo, request.Commit)
			if err != nil {
				return nil, err
			}
		}
		preview, err := service.Diff(ctx, repo, request)
		return func() {
			a.CommitDetails = details
			a.applyPreview(preview)
		}, err
	})
}
func (a *App) applyPreview(preview domain.DiffPreview) {
	a.Preview = preview
	a.lines = strings.Split(preview.Text, "\n")
	a.diffWidth = 600
	for _, line := range a.lines {
		a.diffWidth = max(a.diffWidth, float32(utf8.RuneCountInString(line))*16+32)
	}
	a.DiffList = ui.ListState{}
	a.DiffScroll = ui.ScrollState{}
}
func (a *App) Cancel() {
	if a.Writing {
		if a.cancel != nil {
			a.cancel()
		}
		a.Message = "Cancelling · verifying repository state"
		return
	}
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
func (a *App) Close() {
	a.Cancel()
	a.savePrefs()
	a.closed = true
	if a.dialogCancel != nil {
		a.dialogCancel()
		a.dialogCancel = nil
	}
	a.DialogOpen = false
	if a.watchCancel != nil {
		a.watchCancel()
		a.watchCancel = nil
	}
}
func (a *App) Wait() { a.workers.Wait() }
func (a *App) Choose() {
	choose := a.ChooseDirectory
	if choose == nil {
		return
	}
	a.showDialog(func() func() {
		path, err := choose()
		return func() {
			if err != nil {
				a.Message = "Cannot open directory dialog"
				return
			}
			if path != "" {
				a.Open(path)
			}
		}
	})
}
func pathLabel(path string) string { return strconv.QuoteToGraphic(path) }
