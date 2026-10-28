package local

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
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
	WorkspaceMenuOpen     bool
	ConnectionMenuOpen    bool
	SelectedWorktree      int
	WorktreePathInput     string
	WorktreeBranchInput   string
	WorktreeStatusText    string
	BranchPopOpen         bool
	ChangesTree           bool
	changeCheckState      map[string]*bool
	changesCollapsed      map[string]bool
	WtOpen                bool
	BrOpen                bool
	railPane              string
	HistoryPage           gitexec.HistoryPage
	CommitDetails         gitexec.CommitDetails
	Graph                 domain.GraphLayout
	GraphRows             []domain.GraphRow
	HistoryList           ui.ListState
	HistorySelected       int
	HistoryQuery          string
	HistoryRef            string
	BranchQuery           string
	BranchTreeWidth       float32
	branchCollapsed       map[string]bool
	branchList            ui.ListState
	branchRowsBuilt       int
	HistoryShowRemote     bool
	HistoryDetailsOpen    bool
	HistoryDetailsHeight  float32
	historyDisplayGraph   []domain.GraphRow
	graphColorIDs         map[string]int
	graphColorCount       int
	historyAttempted      bool
	revealGen             int
	historyFilter         string
	historyFilterCount    int
	historyVisible        []int
	historyListSelected   int
	HistoryBuiltRows      int
	HistoryScroll         ui.ScrollState
	HistoryLanes          int
	EditorTabs            []*editorTab
	EditorActive          int
	Branches              []gitexec.BranchRecord
	Tags                  []gitexec.TagRecord
	Stashes               []gitexec.StashRecord
	Worktrees             []domain.WorktreeRecord
	BranchInput           string
	MessageInput          string
	Amend                 bool
	fileHistory           FileHistoryState
	commitFormError       string
	consoleOpen           bool
	consoleLines          []string
	Writing               bool
	DialogOpen            bool
	dialogCancel          context.CancelFunc
	Workspace             domain.WorkspaceView
	WorkspaceFolders      []domain.WorkspaceFolder
	WorkspaceRepositories []domain.RepositoryContext
	workspaceChanges      map[string]int
	WorkspaceInput        string
	FolderInput           string
	EnableAuto            bool
	watchCancel           context.CancelFunc
	autoResults           chan error
	autoBackground        chan bool
	sync                  *syncScheduler
	background            bool
	syncInFlight          bool
	lastSyncError         string
	lastSyncFailedAt      time.Time

	Dispatch                func(func())
	ChooseDirectory         func() (string, error)
	ChooseFile              func() (string, error)
	PathInput               string
	CommitInput             string
	Repository              domain.RepositoryContext
	Recent                  []domain.Repository
	Status                  domain.GitStatus
	Preview                 domain.DiffPreview
	Entries                 []domain.StatusEntry
	Selected                int
	Loading                 bool
	quietLoading            bool
	Stale                   bool
	Message                 string
	StoreWarning            bool
	SidebarWidth            float32
	StatusWidth             float32
	HistoryWidth            float32
	StatusList              ui.ListState
	StatusRowSelected       int
	statusRenderedSelection int
	DiffScroll              ui.ScrollState
	DiffList                ui.ListState
	DiffBuiltRows           int
	diffWidth               float32
	cancel                  context.CancelFunc
	generation              uint64
	closed                  bool
	workers                 sync.WaitGroup
	lines                   []string
}

func New(service application.LocalService) *App {
	a := &App{Service: service, Selected: -1, StatusRowSelected: -1, statusRenderedSelection: -1, HistorySelected: -1, SidebarWidth: 260, StatusWidth: 350, HistoryWidth: 850, BranchTreeWidth: 240, HistoryShowRemote: true, HistoryDetailsHeight: 240, ChangesTree: true, Mode: "commit", Pane: "Status", WtOpen: true, BrOpen: true, Message: "Open a repository"}
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
	a.startOperation(work, false)
}

func (a *App) startQuiet(work func(context.Context) (func(), error)) {
	a.startOperation(work, true)
}

// Replacing a read is safe: cancellation does not invalidate the displayed data.
func (a *App) interruptRead() {
	if a.Loading && !a.Writing {
		stale := a.Stale
		a.Cancel()
		a.Stale = stale
		a.Message = "Ready"
	}
}

func (a *App) startOperation(work func(context.Context) (func(), error), quiet bool) {
	if a.closed || a.Dispatch == nil || a.Writing || a.DialogOpen {
		return
	}
	a.interruptRead()
	a.Cancel()
	a.generation++
	generation := a.generation
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.Loading = true
	a.quietLoading = quiet
	if !quiet {
		a.Message = "Loading"
	}
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
			a.quietLoading = false
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
			if !quiet {
				a.Message = "Ready"
			}
			apply()
		})
	}()
}

// startBackground runs a read whose result updates data only. A background
// sync has no busy concept: it never sets Loading, never writes Message, and
// never interrupts other work, so a watch tick cannot disable buttons, spin
// the status bar or overwrite user feedback. A successful sync clears Stale:
// the displayed status was just re-read, so writes are possible again.
// Failures are recorded silently for the muted sync hint; the result still
// feeds autoResults so the watch backoff keeps working.
func (a *App) startBackground(work func(context.Context) (func(), error)) {
	if a.closed || a.Dispatch == nil || a.Loading || a.Writing || a.DialogOpen || a.syncInFlight {
		return
	}
	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	a.generation++
	generation := a.generation
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.syncInFlight = true
	dispatch := a.Dispatch
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		apply, err := work(ctx)
		dispatch(func() {
			a.syncInFlight = false
			if a.closed || generation != a.generation {
				return
			}
			a.cancel = nil
			cancel()
			if a.autoResults != nil {
				select {
				case a.autoResults <- err:
				default:
				}
			}
			if err != nil {
				a.lastSyncError = message(err)
				a.lastSyncFailedAt = time.Now()
				return
			}
			a.lastSyncError = ""
			a.Stale = false
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
			if a.Repository.Repository.RepositoryID != result.Repository.Repository.RepositoryID {
				a.fileHistory.Release()
				a.MessageInput = ""
				a.Amend = false
			}
			a.Repository = result.Repository
			a.commitFormError = ""
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
	a.selectedDiffEntries()
	a.Stale = false
	a.Selected = -1
	a.StatusList = ui.ListState{}
	a.StatusRowSelected = -1
	a.statusRenderedSelection = -1
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
	request := domain.DiffRequest{Scope: previous.Scope, Path: previous.Path, Commit: a.CommitDetails.Commit.OID}
	start := a.start
	if preserve {
		start = a.startBackground
	}
	start(func(ctx context.Context) (func(), error) {
		status, err := service.Status(ctx, repo)
		if err != nil {
			return nil, err
		}
		entries := domain.StatusEntries(status)
		preview, replaced := previous, false
		if preserve && request.Scope != "" && request.Scope != domain.CommitDiff {
			// A commit diff is immutable and does not depend on the working
			// tree, so it survives a background sync untouched.
			if containsEntry(entries, request.Scope, request.Path) {
				preview, err = service.Diff(ctx, repo, request)
				if err != nil {
					return nil, err
				}
			} else {
				preview = domain.DiffPreview{}
			}
			replaced = true
		}
		return func() {
			if preserve {
				a.applyStatusPatch(status, entries, preview, replaced)
				return
			}
			a.applyStatus(status)
		}, nil
	})
}

// entryKey matches the StatusList row identity in changes.go, so the merged
// status, the list's key-based place keeping and the selection agree.
func entryKey(entry domain.StatusEntry) string {
	return entry.Group + "\x00" + entry.Change.Path
}

func containsEntry(entries []domain.StatusEntry, scope domain.DiffScope, path string) bool {
	for _, entry := range entries {
		if entry.Scope == scope && entry.Change.Path == path {
			return true
		}
	}
	return false
}

// applyStatusPatch merges a refreshed status into the live view without
// resetting list state. An identical snapshot changes nothing at all; rows are
// matched by their stable key, so insertions shift the selection and removals
// clear it by identity rather than by index. The preview is replaced only when
// its content changed, keeping the diff list and scroll where they are.
// Zeroing list state is reserved for semantic resets such as Open.
func (a *App) applyStatusPatch(status domain.GitStatus, entries []domain.StatusEntry, preview domain.DiffPreview, replaced bool) {
	if status.RepositoryID == a.Status.RepositoryID && reflect.DeepEqual(status.Changes, a.Status.Changes) && (!replaced || preview == a.Preview) {
		return
	}
	selected := -1
	if a.Selected >= 0 && a.Selected < len(a.Entries) {
		key := entryKey(a.Entries[a.Selected])
		for i, entry := range entries {
			if entryKey(entry) == key {
				selected = i
				break
			}
		}
	}
	a.Status = status
	a.Entries = entries
	a.selectedDiffEntries()
	if selected != a.Selected {
		a.Selected = selected
		a.statusRenderedSelection = -2 // re-derive the rendered row on the next frame
	}
	if !replaced || preview == a.Preview {
		return
	}
	if preview.Text == a.Preview.Text {
		// Same diff content: keep the diff list and scroll exactly where they are.
		a.Preview = preview
		return
	}
	a.applyPreview(preview)
}
func (a *App) Select(index int) {
	if index < 0 || index >= len(a.Entries) {
		return
	}
	entry := a.Entries[index]
	if a.fileHistory.open && a.fileHistory.path != entry.Change.Path {
		a.fileHistory.Release()
	}
	a.Selected = index
	a.loadDiff(domain.DiffRequest{Scope: entry.Scope, Path: entry.Change.Path})
}
func (a *App) CommitDiff() {
	a.fileHistory.Release()
	a.Selected = -1
	a.loadDiff(domain.DiffRequest{Scope: domain.CommitDiff, Commit: strings.TrimSpace(a.CommitInput)})
}
func (a *App) loadDiff(request domain.DiffRequest) {
	service, repo := a.Service, a.Repository
	// Keep the last preview visible until its replacement is ready.
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
			a.autoSelectFirstCommitFile()
		}, err
	})
}

// Selecting a commit opens its first changed file so the bottom inspector
// starts with a useful diff instead of a second click.
func (a *App) autoSelectFirstCommitFile() {
	if a.Mode != "log" || a.Writing || a.DialogOpen || len(a.CommitDetails.Files) == 0 || a.Preview.Path != "" {
		return
	}
	file := a.CommitDetails.Files[0]
	a.loadDiff(domain.DiffRequest{Scope: domain.CommitDiff, Commit: a.CommitDetails.Commit.OID, Path: file.Path})
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
		a.quietLoading = false
		a.Stale = true
		a.Message = "OperationCancelled"
	}
}
func (a *App) Close() {
	a.Cancel()
	a.fileHistory.Release()
	diffFinds.Delete(a)
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
