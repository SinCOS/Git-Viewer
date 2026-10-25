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
	a.HistoryBranch = "Show All"
	a.historyFilterCount = -1
	a.HistoryList = ui.ListState{}
	a.Branches = nil
	a.Worktrees = nil
	a.Pane = "Status"
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
			a.Repository = repo
			a.PathInput = repo.Repository.Path
			a.CommitInput = ""
			a.applyStatus(status)
			a.resetHistory()
			a.startAuto()
		}, nil
	})
}
func (a *App) LoadHistory(more bool) {
	if a.Repository.Repository.RepositoryID == "" || a.Loading || a.Writing || a.DialogOpen {
		return
	}
	service, repo := a.Service, a.Repository
	anchor := ""
	offset := 0
	if more {
		anchor = a.HistoryPage.Next
		offset = a.HistoryPage.Offset
	}
	a.start(func(ctx context.Context) (func(), error) {
		page, err := service.History(ctx, repo, anchor, offset)
		if err != nil {
			return nil, err
		}
		var branches []gitexec.BranchRecord
		if !more {
			branches, err = service.Branches(ctx, repo)
			if err != nil {
				return nil, err
			}
		}
		return func() {
			if !more {
				a.historyVisible = nil
				a.graphColorIDs = nil
				a.historyListSelected = -1
				a.CommitDetails = gitexec.CommitDetails{}
				a.Branches = branches
				a.Graph = domain.GraphLayout{}
				a.GraphRows = nil
				a.HistoryLanes = 0
				a.HistoryScroll = ui.ScrollState{}
				a.HistoryList = ui.ListState{}
				a.HistorySelected = -1
			}
			newRows := a.Graph.Append(page.Commits)
			for _, row := range newRows {
				a.HistoryLanes = max(a.HistoryLanes, row.Width)
			}
			a.GraphRows = append(a.GraphRows, newRows...)
			if more {
				page.Commits = append(a.HistoryPage.Commits, page.Commits...)
			}
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
		trees, err := service.Worktrees(ctx, repo)
		if err != nil {
			return nil, err
		}
		return func() {
			a.Branches = branches
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
	case domain.SwitchBranch:
		return "Switch branch"
	}
	return "Git operation"
}
func (a *App) RunWrite(op domain.Operation) { a.runWrite(op, false) }

func (a *App) runWrite(op domain.Operation, pushAfterCommit bool) {
	if a.Loading || a.Writing || a.DialogOpen || a.Stale || a.Repository.Repository.RepositoryID == "" {
		return
	}
	request := domain.GitOperation{RepositoryID: a.Repository.Repository.RepositoryID, Operation: op}
	switch op {
	case domain.Stage, domain.Unstage:
		if a.Selected < 0 || a.Selected >= len(a.Entries) {
			return
		}
		request.Paths = []string{a.Entries[a.Selected].Change.Path}
	case domain.Commit:
		request.Message = a.MessageInput
	case domain.SwitchBranch:
		request.Ref = a.BranchInput
	}
	detail := fmt.Sprintf("%s\n%s\n%s", a.Repository.Repository.DisplayName, pathLabel(a.Repository.Repository.Path), operationLabel(op))
	if len(request.Paths) > 0 {
		detail += "\n" + pathLabel(request.Paths[0])
	}
	if request.Ref != "" {
		detail += "\n" + request.Ref
	}
	confirm := a.Confirm
	if confirm == nil {
		return
	}
	service, repo := a.Service, a.Repository
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
				if result.Unknown {
					a.Stale = true
				}
			} else {
				a.Message = "Completed · repository refreshed"
				if op == domain.Commit {
					a.MessageInput = ""
				}
			}
			a.resetHistory()
			if op == domain.Commit && pushAfterCommit && err == nil && result.Verified {
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
		}, nil
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
		}, err
	})
}
