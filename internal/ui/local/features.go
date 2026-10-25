package local

import (
	"context"
	"fmt"
	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/workspace"
	"path/filepath"
	"strings"
)

func (a *App) resetHistory() {
	a.HistoryPage.Commits = nil
	a.Graph = domain.GraphLayout{}
	a.GraphRows = nil
	a.HistoryLanes = 0
	a.HistoryScroll = ui.ScrollState{}
	a.HistorySelected = -1
	a.HistoryList = ui.ListState{}
	a.Branches = nil
	a.Worktrees = nil
	a.Pane = ""
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
	if a.Repository.Repository.RepositoryID == "" {
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
		return func() {
			if !more {
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
			a.Pane = "History"
		}, nil
	})
}
func (a *App) LoadBranches() {
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
		return func() { a.Branches = branches; a.Worktrees = trees; a.Pane = "Branches" }, nil
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
func (a *App) RunWrite(op domain.Operation) {
	if a.Loading || a.Writing || a.Stale || a.Repository.Repository.RepositoryID == "" {
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
	if a.Confirm == nil || !a.Confirm("Confirm Git operation", detail) {
		return
	}
	service, repo := a.Service, a.Repository
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
	path := a.Workspace.FilePath
	if create {
		path = a.WorkspaceInput
		if a.ChooseWorkspaceSave != nil {
			chosen, err := a.ChooseWorkspaceSave()
			if err != nil {
				a.Message = "Cannot open save dialog"
				return
			}
			if chosen == "" {
				return
			}
			path = chosen
		}
	}
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
func (a *App) featureToolbar(c *ui.Context) {
	ui.Row(c).Gap(6).Padding(4).Children(func() {
		ui.Combobox(c, &a.SSHHost, a.SSHHosts).Label("SSH host").Placeholder("SSH Host alias").Width(150)
		ui.TextInput(c, &a.SSHPath).Label("Remote repository path").Placeholder("/absolute/remote/path").Grow(1)
		ui.Button(c, "Connect SSH").Disabled(a.Loading).OnClick(a.OpenSSH)
		ui.Button(c, "Status").OnClick(func() { a.Pane = "" })
		ui.Button(c, "History / Graph").Disabled(a.Loading).OnClick(func() { a.LoadHistory(false) })
		ui.Button(c, "Branches / Worktrees").Disabled(a.Loading).OnClick(a.LoadBranches)
	})
}
func (a *App) writeControls(c *ui.Context) {
	unavailable := a.Loading || a.Stale || a.Repository.Repository.RepositoryID == ""
	ui.Row(c).Gap(4).Children(func() {
		for _, op := range []domain.Operation{domain.Stage, domain.Unstage, domain.Fetch, domain.PullFFOnly, domain.Push} {
			ui.Button(c, operationLabel(op)).Disabled(unavailable).OnClick(func() { a.RunWrite(op) })
		}
	})
	ui.Row(c).Gap(4).Children(func() {
		ui.TextArea(c, &a.MessageInput).Label("Commit message").Placeholder("Commit message").Height(64).Grow(1)
		ui.Button(c, "Commit staged").Disabled(unavailable).OnClick(func() { a.RunWrite(domain.Commit) })
	})
	ui.Row(c).Gap(4).Children(func() {
		ui.TextInput(c, &a.BranchInput).Label("Local branch").Placeholder("Local branch name").Grow(1)
		ui.Button(c, "Switch branch").Disabled(unavailable).OnClick(func() { a.RunWrite(domain.SwitchBranch) })
	})
}
func (a *App) historyView(c *ui.Context) {
	a.HistoryBuiltRows = 0
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Gap(4).Children(func() {
			ui.Textf(c, "History · %d commits", len(a.GraphRows)).Bold()
			ui.Button(c, "Load more").Disabled(a.Loading || !a.HistoryPage.More).OnClick(func() { a.LoadHistory(true) })
		})
		a.HistoryList.Selected = &a.HistorySelected
		a.HistoryList.Key = func(i int) any { return a.GraphRows[i].Commit.OID }
		a.HistoryList.Label = func(i int) string { return a.GraphRows[i].Commit.Subject }
		ui.ScrollHorizontal(c).TrackScroll(&a.HistoryScroll).Grow(1).FillWidth().Children(func() {
			list := ui.List(c, &a.HistoryList, len(a.GraphRows), func(i int) {
				a.HistoryBuiltRows++
				row := a.GraphRows[i]
				accent, muted := c.Theme().Accent, c.Theme().TextMuted
				ui.Row(c).Height(48).Children(func() {
					box := ui.Box(c).Size(float32(max(2, a.HistoryLanes))*16+12, 48)
					box.Draw(func(p *ui.Painter, r ui.Rect) {
						x := func(l int) float32 { return r.X + 12 + float32(l)*16 }
						for _, e := range row.Edges {
							p.Line(x(e.From), r.Y+24, x(e.To), r.Y+48, 1, muted)
						}
						if i > 0 {
							for _, e := range a.GraphRows[i-1].Edges {
								p.Line(x(e.To), r.Y, x(e.To), r.Y+24, 1, muted)
							}
						}
						p.Fill(ui.Rect{X: x(row.Lane) - 4, Y: r.Y + 20, W: 8, H: 8}, accent, 4)
					})
					ui.Column(c).Children(func() {
						ui.Text(c, row.Commit.Subject).NoWrap()
						ui.Textf(c, "%.10s · %s · %s · %s", row.Commit.OID, row.Commit.Author, row.Commit.AuthoredAt.Format("2006-01-02"), row.Commit.Refs).NoWrap()
					})
				})
			}).Width(float32(max(2, a.HistoryLanes))*16 + 700).FillHeight()
			if list.Changed() && a.HistorySelected >= 0 && a.HistorySelected < len(a.GraphRows) {
				a.CommitInput = a.GraphRows[a.HistorySelected].Commit.OID
				a.CommitDiff()
			}
		})
	})
}
func (a *App) branchesView(c *ui.Context) {
	ui.Scroll(c).Fill().Children(func() {
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			ui.Text(c, "Branches").Bold()
			for _, b := range a.Branches {
				label := b.Name
				if b.Current {
					label = "HEAD · " + label
				}
				ui.Button(c, label+" → "+b.Upstream).OnClick(func() { a.BranchInput = strings.TrimPrefix(b.Name, "refs/heads/") })
			}
			ui.Text(c, "Worktrees").Bold()
			for _, w := range a.Worktrees {
				ui.Textf(c, "%s · %s · detached=%t · locked=%t %s · prunable=%t %s", pathLabel(w.Path), w.Branch, w.Detached, w.Locked, w.LockReason, w.Prunable, w.PruneReason).Selectable()
			}
		})
	})
}
func (a *App) workspaceSidebar(c *ui.Context) {
	ui.Text(c, "Workspace").Bold()
	if a.Workspace.FilePath != "" {
		ui.Button(c, "Close workspace").Disabled(a.Loading).OnClick(func() {
			a.Workspace = domain.WorkspaceView{}
			a.WorkspaceFolders = nil
			a.WorkspaceRepositories = nil
			a.WorkspaceInput = ""
		})
	}
	for _, loc := range a.Locations {
		label := loc.Path
		if loc.Kind == "ssh" {
			label = loc.Host + ":" + label
		}
		ui.Button(c, "Recent "+pathLabel(label)).OnClick(func() {
			if loc.Kind == "ssh" {
				a.SSHHost = loc.Host
				a.SSHPath = loc.Path
				a.OpenSSH()
			} else {
				a.OpenWorkspace(loc.Path)
			}
		})
	}
	ui.TextInput(c, &a.WorkspaceInput).Label("Workspace path").Placeholder(".code-workspace")
	ui.Row(c).Gap(4).Children(func() {
		ui.Button(c, "Open workspace").Disabled(a.Loading).OnClick(func() {
			path := a.WorkspaceInput
			if a.ChooseWorkspace != nil {
				chosen, err := a.ChooseWorkspace()
				if err != nil {
					a.Message = "Cannot open workspace dialog"
					return
				}
				if chosen == "" {
					return
				}
				path = chosen
			}
			a.OpenWorkspace(path)
		})
		ui.Button(c, "Save").Disabled(a.Loading || a.Workspace.FilePath == "").OnClick(func() { a.SaveWorkspace(false) })
	})
	ui.Button(c, "New / Save workspace as…").Disabled(a.Loading).OnClick(func() { a.SaveWorkspace(true) })
	ui.TextInput(c, &a.FolderInput).Label("Workspace folder").Placeholder("Folder path")
	ui.Button(c, "Add folder").OnClick(func() {
		if a.FolderInput != "" {
			a.WorkspaceFolders = append(a.WorkspaceFolders, domain.WorkspaceFolder{Path: a.FolderInput})
			a.FolderInput = ""
		}
	})
	for i, f := range a.WorkspaceFolders {
		ui.Row(c).Children(func() {
			ui.Text(c, pathLabel(f.Path)).Grow(1)
			ui.Button(c, fmt.Sprintf("Remove %d", i+1)).OnClick(func() { a.WorkspaceFolders = append(a.WorkspaceFolders[:i], a.WorkspaceFolders[i+1:]...) })
		})
	}
	for _, repo := range a.WorkspaceRepositories {
		ui.Button(c, "Open "+pathLabel(repo.Repository.Path)).OnClick(func() { a.Open(repo.Repository.Path) })
	}
	for _, f := range a.Workspace.Folders {
		for _, diagnostic := range f.Diagnostics {
			ui.Text(c, pathLabel(f.Path)+": "+diagnostic)
		}
	}
}
