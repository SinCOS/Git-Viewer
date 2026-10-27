package local

import (
	"context"
	"fmt"
	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
	"gitviewer/internal/infrastructure/workspace"
	"path/filepath"
)

func (a *App) resetHistory() {
	a.HistoryPage.Commits = nil
	a.Graph = domain.GraphLayout{}
	a.GraphRows = nil
	a.HistoryLanes = 0
	a.HistoryScroll = ui.ScrollState{}
	a.HistorySelected = -1
	a.historyAttempted = false
	a.HistoryQuery = ""
	a.historyVisible = nil
	a.historyDisplayGraph = nil
	a.graphColorIDs = nil
	a.HistoryRef = ""
	a.BranchQuery = ""
	a.branchCollapsed = nil
	a.branchList = ui.ListState{}
	a.historyFilterCount = -1
	a.HistoryList = ui.ListState{}
	a.Branches = nil
	a.Tags = nil
	a.Stashes = nil
	a.Worktrees = nil
	a.WorktreeStatusText = ""
}
func (a *App) OpenSSH() {
	service, host, path := a.Service, a.SSHHost, a.SSHPath
	a.start(func(ctx context.Context) (func(), error) {
		repo, err := service.OpenSSH(ctx, host, path)
		if err != nil {
			return nil, err
		}
		status, err := service.Status(ctx, repo)
		if err != nil {
			return nil, err
		}
		locations, persistErr := service.Locations().Remember(domain.SavedLocation{Kind: "ssh", Host: host, Path: repo.Repository.Path})
		return func() {
			a.Locations = locations
			a.StoreWarning = persistErr != nil
			if a.Repository.Repository.RepositoryID != repo.Repository.RepositoryID {
				a.MessageInput = ""
				a.Amend = false
			}
			a.Repository = repo
			a.commitFormError = ""
			a.PathInput = repo.Repository.Path
			a.CommitInput = ""
			a.applyStatus(status)
			a.resetHistory()
			a.startAuto()
		}, nil
	})
}
func (a *App) LoadHistory(more bool) {
	a.loadHistory(more, a.HistoryRef)
}

// Keep the displayed scope and its snapshot together until a read succeeds.
// Failed or cancelled branch switches leave the previous view intact.
func (a *App) loadHistory(more bool, scope string) {
	if a.Repository.Repository.RepositoryID == "" || (more && a.Loading) || a.Writing || a.DialogOpen {
		return
	}
	service, repo := a.Service, a.Repository
	displayedScope, loadedCount := a.HistoryRef, len(a.GraphRows)
	anchor := ""
	offset := 0
	if more {
		anchor = a.HistoryPage.Next
		offset = a.HistoryPage.Offset
	}
	a.start(func(ctx context.Context) (func(), error) {
		var branches []gitexec.BranchRecord
		var tags []gitexec.TagRecord
		var stashes []gitexec.StashRecord
		var err error
		if !more {
			branches, err = service.Branches(ctx, repo)
			if err != nil {
				return nil, err
			}
			tags, err = service.Tags(ctx, repo)
			if err != nil {
				return nil, err
			}
			stashes, err = service.Stashes(ctx, repo)
			if err != nil {
				return nil, err
			}
			if scope != "" && scope != "HEAD" {
				for _, b := range branches {
					if b.Name == scope && b.Symbolic == "" {
						anchor = b.OID
						break
					}
				}
				if anchor == "" {
					scope = ""
				} // Deleted ref: return to all branches.
			}
		}
		var page gitexec.HistoryPage
		if !more && scope == "HEAD" {
			page, err = service.ReaderFor(repo).HeadHistory(ctx, repo, 100)
		} else {
			page, err = service.History(ctx, repo, anchor, offset)
		}
		if err != nil {
			return nil, err
		}
		// Refresh the range already loaded by the user, so refreshing while
		// scrolled beyond page one does not truncate the displayed list.
		for !more && scope == displayedScope && page.More && len(page.Commits) < loadedCount {
			next, nextErr := service.History(ctx, repo, page.Next, page.Offset)
			if nextErr != nil {
				return nil, nextErr
			}
			next.Commits = append(page.Commits, next.Commits...)
			page = next
		}
		return func() {
			sameScope := a.HistoryRef == scope
			a.Stale = false
			selectedOID := ""
			if sameScope && a.HistorySelected >= 0 && a.HistorySelected < len(a.GraphRows) {
				selectedOID = a.GraphRows[a.HistorySelected].Commit.OID
			}
			if !more {
				a.historyVisible = nil
				a.graphColorIDs = nil
				a.Branches = branches
				a.Graph = domain.GraphLayout{}
				a.HistoryLanes = 0
				// Keep the native list and column layout alive. Only an actual
				// scope change starts at the top; refresh keeps the scroll anchor.
				if !sameScope {
					a.HistoryList.ScrollTo(0, ui.Start)
				}
			}
			if !more {
				a.Tags = tags
				a.Stashes = stashes
			}
			newRows := a.Graph.Append(page.Commits)
			for _, row := range newRows {
				a.HistoryLanes = max(a.HistoryLanes, row.Width)
			}
			if more {
				a.GraphRows = append(a.GraphRows, newRows...)
				page.Commits = append(a.HistoryPage.Commits, page.Commits...)
			} else {
				a.GraphRows = newRows
				a.HistorySelected, a.historyListSelected = -1, -1
				for i, row := range a.GraphRows {
					if row.Commit.OID == selectedOID {
						a.HistorySelected = i
						break
					}
				}
				if a.HistorySelected >= 0 {
					for i, source := range a.filteredHistory() {
						if source == a.HistorySelected {
							a.historyListSelected = i
							break
						}
					}
				} else {
					a.CommitDetails = gitexec.CommitDetails{}
					a.CommitInput = ""
				}
			}
			a.HistoryRef = scope
			a.HistoryPage = page
		}, nil
	})
	if a.Loading {
		a.historyAttempted = true
		a.Pane = "History"
	}
}
func (a *App) LoadBranches() { a.loadBranches(true) }

func (a *App) loadBranches(showWorktrees bool) {
	if a.Repository.Repository.RepositoryID == "" {
		return
	}
	service, repo := a.Service, a.Repository
	a.start(func(ctx context.Context) (func(), error) {
		branches, err := service.Branches(ctx, repo)
		if err != nil {
			return nil, err
		}
		tags, err := service.Tags(ctx, repo)
		if err != nil {
			return nil, err
		}
		stashes, err := service.Stashes(ctx, repo)
		if err != nil {
			return nil, err
		}
		trees, err := service.Worktrees(ctx, repo)
		if err != nil {
			return nil, err
		}
		return func() {
			a.Branches = branches
			a.Tags = tags
			a.Stashes = stashes
			a.Worktrees = trees
			a.SelectedWorktree = 0
			if showWorktrees {
				a.Pane = "Branches"
			}
		}, nil
	})
}
func operationLabel(op domain.Operation) string {
	switch op {
	case domain.Stage:
		return "Stage file"
	case domain.Unstage:
		return "Unstage file"
	case domain.Fetch:
		return "Fetch"
	case domain.PullFFOnly:
		return "Pull (fast-forward)"
	case domain.Push:
		return "Push"
	case domain.Commit:
		return "Commit staged changes"
	case domain.Amend:
		return "Amend last commit"
	case domain.Discard:
		return "Discard unstaged changes"
	case domain.SwitchBranch:
		return "Switch branch"
	}
	return "Git operation"
}
func (a *App) RunWrite(op domain.Operation) { a.runWrite(op, false) }

func (a *App) runWrite(op domain.Operation, pushAfterCommit bool) {
	if a.Writing || a.DialogOpen || a.Stale || a.Repository.Repository.RepositoryID == "" {
		return
	}
	request := domain.GitOperation{RepositoryID: a.Repository.Repository.RepositoryID, Operation: op}
	switch op {
	case domain.Stage, domain.Unstage, domain.Discard:
		if a.Selected < 0 || a.Selected >= len(a.Entries) {
			return
		}
		request.Paths = []string{a.Entries[a.Selected].Change.Path}
	case domain.Commit, domain.Amend:
		request.Message = a.MessageInput
	case domain.SwitchBranch:
		request.Ref = a.BranchInput
	}
	a.confirmWrite(request, pushAfterCommit)
}

// runBatchChanges uses the same confirmation and validated writer as individual
// changes. Capture paths before opening the asynchronous confirmation dialog.
func (a *App) runBatchChanges(op domain.Operation) {
	if a.syncDisabled() || (op != domain.Stage && op != domain.Unstage) {
		return
	}
	request := domain.GitOperation{RepositoryID: a.Repository.Repository.RepositoryID, Operation: op}
	seen := make(map[string]bool)
	for _, entry := range a.Entries {
		if (op == domain.Unstage) != (entry.Scope == domain.StagedDiff) || seen[entry.Change.Path] {
			continue
		}
		seen[entry.Change.Path] = true
		request.Paths = append(request.Paths, entry.Change.Path)
	}
	if len(request.Paths) > 0 {
		a.confirmWrite(request, false)
	}
}

func (a *App) confirmWrite(request domain.GitOperation, pushAfterCommit bool) {
	detail := fmt.Sprintf("%s\n%s\n%s", a.Repository.Repository.DisplayName, pathLabel(a.Repository.Repository.Path), operationLabel(request.Operation))
	if len(request.Paths) > 0 {
		detail += fmt.Sprintf("\n%d file(s)", len(request.Paths))
		for _, path := range request.Paths {
			detail += "\n" + pathLabel(path)
		}
	}
	if request.Ref != "" {
		detail += "\n" + request.Ref
	}
	confirm := a.Confirm
	if confirm == nil {
		return
	}
	service, repo := a.Service, a.Repository
	a.interruptRead()
	a.showDialog(func() func() {
		accepted := confirm("Confirm Git operation", detail)
		if !accepted {
			return nil
		}
		return func() { a.runConfirmedWrite(service, repo, request, pushAfterCommit) }
	})
}
func (a *App) runConfirmedWrite(service application.LocalService, repo domain.RepositoryContext, request domain.GitOperation, pushAfterCommit bool) {
	if a.closed || a.Stale || a.Repository.Repository.RepositoryID != repo.Repository.RepositoryID {
		return
	}
	op := request.Operation
	formWrite := op == domain.Commit || op == domain.Amend || op == domain.Stage || op == domain.Unstage || op == domain.Discard
	if formWrite {
		a.commitFormError = ""
	}
	a.start(func(ctx context.Context) (func(), error) {
		result, err := service.Write(ctx, repo, request)
		return func() {
			a.Writing = false
			if result.Verified {
				a.applyStatus(result.Status)
			} else {
				a.Stale = true
			}
			if err != nil {
				a.Message = message(err)
				if formWrite {
					a.commitFormError = operationLabel(op) + " failed · " + message(err)
				}
				if result.Unknown {
					a.Stale = true
				}
			} else {
				a.Message = "Completed · repository refreshed"
				if op == domain.Commit || op == domain.Amend {
					a.MessageInput = ""
					a.Amend = false
				}
			}
			a.resetHistory()
			if (op == domain.Commit || op == domain.Amend) && pushAfterCommit && err == nil && result.Verified {
				a.RunWrite(domain.Push)
			}
		}, nil
	})
	if a.Loading {
		a.Writing = true
		a.Message = "Running " + operationLabel(op) + " · waiting for Git"
	}
}
func (a *App) OpenWorkspace(path string) {
	reader, locationsStore := a.Service.Reader, a.Service.Locations()
	a.start(func(ctx context.Context) (func(), error) {
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}
		d, err := workspace.Read(abs)
		if err != nil {
			return nil, err
		}
		view, repos, err := workspace.View(ctx, abs, d, reader)
		if err != nil {
			return nil, err
		}
		locations, persistErr := locationsStore.Remember(domain.SavedLocation{Kind: "workspace", Path: abs})
		return func() {
			a.Locations = locations
			a.StoreWarning = persistErr != nil
			a.Workspace = view
			a.WorkspaceInput = abs
			a.WorkspaceFolders = append([]domain.WorkspaceFolder(nil), d.Folders...)
			a.WorkspaceRepositories = repos
			a.loadWorkspaceStatuses(true)
		}, nil
	})
}

// CloseWorkspace clears the active workspace document; loaded repositories stay open.
func (a *App) CloseWorkspace() {
	a.Workspace = domain.WorkspaceView{}
	a.WorkspaceFolders = nil
	a.WorkspaceRepositories = nil
	a.workspaceChanges = nil
	a.WorkspaceInput = ""
}

// loadWorkspaceStatuses refreshes the per-repository change counts shown on
// the workspace bar. It runs as a background read: no busy state, no message
// writes, and a failing repository just drops its badge until the next load.
// With openFirst, the first workspace repository opens once the counts are
// in — opening earlier would supersede this read and cancel it.
func (a *App) loadWorkspaceStatuses(openFirst bool) {
	if len(a.WorkspaceRepositories) == 0 || a.closed || a.Dispatch == nil {
		return
	}
	repos := append([]domain.RepositoryContext(nil), a.WorkspaceRepositories...)
	svc := a.Service
	a.startBackground(func(ctx context.Context) (func(), error) {
		counts := make(map[string]int, len(repos))
		for _, repo := range repos {
			status, err := svc.Status(ctx, repo)
			if err != nil {
				continue
			}
			counts[repo.Repository.RepositoryID] = len(status.Changes)
		}
		return func() {
			a.workspaceChanges = counts
			if openFirst {
				a.openFirstWorkspaceRepository(repos)
			}
		}, nil
	})
}

// AddToWorkspace queues a folder for the active workspace document; saving
// persists it. Duplicates and empty paths are ignored.
func (a *App) AddToWorkspace(path string) {
	if path == "" {
		return
	}
	for _, f := range a.WorkspaceFolders {
		if f.Path == path {
			return
		}
	}
	a.WorkspaceFolders = append(a.WorkspaceFolders, domain.WorkspaceFolder{Path: path})
	a.Message = "Added to workspace · save to persist"
}

// openFirstWorkspaceRepository opens the first workspace repository when
// nothing is open yet, so a multi-root workspace lands like VS Code: the
// document loads with its first root visible. An already-open repository is
// left alone.
func (a *App) openFirstWorkspaceRepository(repos []domain.RepositoryContext) {
	if a.Repository.Repository.RepositoryID != "" || len(repos) == 0 {
		return
	}
	a.Open(repos[0].Repository.Path)
}

// AddFolderToWorkspace picks a local directory with the OS dialog and queues
// it for the active workspace document; saving persists it.
func (a *App) AddFolderToWorkspace() {
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
				a.AddToWorkspace(path)
			}
		}
	})
}
func (a *App) SaveWorkspace(create bool) {
	if a.closed || a.Loading || a.Writing || a.DialogOpen {
		return
	}
	if create && a.ChooseWorkspaceSave != nil {
		choose := a.ChooseWorkspaceSave
		a.showDialog(func() func() {
			path, err := choose()
			return func() {
				if err != nil {
					a.Message = "Cannot open save dialog"
					return
				}
				if path != "" {
					a.saveWorkspace(path, true)
				}
			}
		})
		return
	}
	path := a.Workspace.FilePath
	if create {
		path = a.WorkspaceInput
	}
	a.saveWorkspace(path, create)
}
func (a *App) saveWorkspace(path string, create bool) {
	if path == "" {
		return
	}
	folders := append([]domain.WorkspaceFolder{}, a.WorkspaceFolders...)
	revision := a.Workspace.Revision
	reader, locationsStore := a.Service.Reader, a.Service.Locations()
	a.start(func(ctx context.Context) (func(), error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var d workspace.Document
		var err error
		if create {
			d, err = workspace.Create(path, folders)
		} else {
			d, err = workspace.Save(path, revision, folders)
		}
		if err != nil {
			return nil, err
		}
		view, repos, err := workspace.View(ctx, path, d, reader)
		if err != nil {
			return nil, err
		}
		locations, persistErr := locationsStore.Remember(domain.SavedLocation{Kind: "workspace", Path: path})
		return func() {
			a.Locations = locations
			a.StoreWarning = persistErr != nil
			a.Workspace = view
			a.WorkspaceInput = path
			a.WorkspaceRepositories = repos
			a.loadWorkspaceStatuses(true)
		}, err
	})
}
