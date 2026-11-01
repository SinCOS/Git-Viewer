package local

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
)

// shell.go owns the window frame: the top toolbar, the narrow mode rail,
// panel mounting and the status bar. Panels are mounted here and must not
// mount each other.
// Modes map onto Pane values shared with the toolbar buttons:
// "commit"→"Status", "log"→"" (history), "editor"→"Editor".

// UIState is layout-only UI state. Business data stays on App.
type UIState struct {
	SidebarWidth   float32
	InspectorWidth float32
}

func (a *App) View(c *ui.Context) {
	c.SetTheme(appTheme(a.ThemeStyle, c.Theme()))
	a.registerShortcuts(c)
	a.syncMode()
	a.ensureHistory()
	ui.Column(c).Fill().Children(func() {
		a.toolbarView(c)
		if a.StoreWarning {
			ui.Text(c, "Recent repositories could not be read or saved. Check application configuration.").TextColor(c.Theme().Danger)
		}
		ui.Row(c).Label("Workspace body").Grow(1).FillWidth().Children(func() {
			a.modeRail(c)
			a.primarySidebar(c)
			ui.Column(c).Fill().Children(func() { a.mainView(c) }).Grow(1)
		})
		a.remoteStatusView(c)
	})
	a.quickOpenView(c)
	a.remoteOpenView(c)
	a.passwordPromptView(c)
	a.searchView(c)
	a.settingsView(c)
	a.conflictView(c)
	a.filePromptView(c)
	a.dirtyCloseView(c)
}

// syncMode derives the active mode before any panels are mounted.
func (a *App) syncMode() {
	// Sync the toolbar from Pane only when Pane changed outside the toolbar, so
	// shortcuts update the mode buttons without fighting them.
	if a.Pane != a.railPane {
		a.railPane = a.Pane
		switch a.Pane {
		case "Status":
			a.Mode = "commit"
			// Source Control owns the worktree preview. A commit diff belongs
			// to the Git Log view and must not leak into the next status mode.
			if a.Preview.Scope == domain.CommitDiff || a.CommitDetails.Commit.OID != "" {
				a.CommitDetails = gitexec.CommitDetails{}
				a.historyFileSelected = -1
				a.Preview = domain.DiffPreview{}
				a.resetNativeDiff()
				a.lines = nil
				a.DiffList = ui.ListState{}
				a.DiffScroll = ui.ScrollState{}
			}
			a.requestBackgroundRefresh()
		case "Editor":
			a.Mode = "editor"
		default:
			a.Mode = "log"
		}
	}
}

// setMode is the single entry point for mode switches from the toolbar. The
// reverse direction (Pane changed elsewhere) is reconciled by syncMode.
func (a *App) setMode(mode string) {
	if a.Mode == mode {
		return
	}
	switch mode {
	case "commit":
		a.Pane = "Status"
		if a.Preview.Scope == domain.CommitDiff || a.CommitDetails.Commit.OID != "" {
			a.CommitDetails = gitexec.CommitDetails{}
			a.historyFileSelected = -1
			a.Preview = domain.DiffPreview{}
			a.resetNativeDiff()
			a.lines = nil
			a.DiffList = ui.ListState{}
			a.DiffScroll = ui.ScrollState{}
		}
		a.requestBackgroundRefresh()
	case "editor":
		a.Pane = "Editor"
	default:
		a.Pane = ""
	}
	a.Mode = mode
	a.railPane = a.Pane
	a.ensureHistory()
}
func (a *App) mainView(c *ui.Context) {
	ui.Column(c).Fill().Children(func() {
		ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
			switch a.Mode {
			case "editor":
				a.editorView(c)
			case "log":
				a.historyWorkspace(c)
			default:
				ui.Column(c).Fill().Children(func() { a.diffView(c) }).Grow(1)
			}
		})
		if a.TerminalOpen {
			a.terminalView(c)
		}
	})
}

// primarySidebar mounts the VS Code-style primary side bar: the mode's
// navigation panel followed by a shared resize divider.
func (a *App) primarySidebar(c *ui.Context) {
	width := clampSidebarWidth(a.SidebarWidth)
	ui.Column(c).Key("primary-sidebar").Width(width).FillHeight().Shrink(0).
		Background(c.Theme().Surface).Children(func() {
		ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
			switch a.Mode {
			case "log":
				a.branchNavigation(c)
			case "editor":
				a.editorSidebar(c)
			default:
				a.commitSidebarPane(c)
			}
		})
	})
	a.sidebarDivider(c, width)
}

// remoteBranchMenu lists remote-tracking branches and reports whether any
// were available for a Pull from / Push to action.
func (a *App) remoteBranchMenu(m *ui.Menu, op domain.Operation) bool {
	count := 0
	for _, branch := range a.Branches {
		if !strings.HasPrefix(branch.Name, "refs/remotes/") || branch.Symbolic != "" {
			continue
		}
		name := strings.TrimPrefix(branch.Name, "refs/remotes/")
		remote, ref, ok := strings.Cut(name, "/")
		if !ok || remote == "" || ref == "" || ref == "HEAD" {
			continue
		}
		if m.Item(name).Disabled(a.syncDisabled()).Chosen() {
			a.runRemoteRef(op, remote, ref)
		}
		count++
	}
	return count > 0
}

func (a *App) commitSidebarPane(c *ui.Context) {
	pane := ui.Column(c).Fill()
	pane.Children(func() {
		ui.Row(c).Height(35).FillWidth().Padding(0, 8).Gap(2).AlignItems(ui.Center).Background(c.Theme().Surface).
			BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Children(func() {
			ui.Text(c, "SOURCE CONTROL").FontSize(11).TextColor(c.Theme().TextMuted).Grow(1).NoWrap()
			sourceControlIconButton(c, "Refresh source control", "Refresh source control", refreshIcon).
				Disabled(a.Writing || a.DialogOpen).OnClick(a.Refresh)
			sourceControlIconButton(c, "Commit staged changes", "Commit staged changes", commitIcon).
				Disabled(a.commitUnavailable()).OnClick(func() { a.RunWrite(a.commitOperation()) })
			sourceControlIconButton(c, "GitLens actions", "File history, blame and file actions", refIcon).
				Disabled(a.selectedChangePath() == "").Menu(func(m *ui.Menu) {
				path := a.selectedChangePath()
				unavailable := path == "" || a.Repository.Repository.RepositoryID == ""
				if m.Item("File History").Disabled(unavailable).Chosen() {
					a.OpenFileHistory(&a.fileHistory, path)
				}
				if m.Item("Blame").Disabled(unavailable).Chosen() {
					a.OpenFileBlame(&a.fileHistory, path)
				}
				m.Separator()
				if m.Item("Open File").Disabled(unavailable || a.Repository.Repository.Location != "local").Chosen() {
					a.openRepoFileInEditor(path)
				}
				if m.Item("Reveal in Finder").Disabled(unavailable || a.Repository.Repository.Location != "local").Chosen() {
					a.RevealInFinder(path)
				}
			})
			more := sourceControlIconButton(c, "More Source Control actions", "More Source Control actions", moreHorizontalIcon)
			more.Menu(func(m *ui.Menu) {
				m.Submenu("View & Sort", func(m *ui.Menu) {
					if m.Item("List View").Chosen() {
						a.setChangesTree(false)
					}
					if m.Item("Tree View").Chosen() {
						a.setChangesTree(true)
					}
				})
				m.Separator()
				if m.Item("Pull").Disabled(a.syncDisabled()).Chosen() {
					a.RunWrite(domain.PullFFOnly)
				}
				if m.Item("Push").Disabled(a.syncDisabled()).Chosen() {
					a.RunWrite(domain.Push)
				}
				m.Item("Clone").Disabled(true)
				m.Submenu("Checkout to...", func(m *ui.Menu) {
					count := 0
					for _, branch := range a.Branches {
						if !strings.HasPrefix(branch.Name, "refs/heads/") || branch.Current {
							continue
						}
						name := strings.TrimPrefix(branch.Name, "refs/heads/")
						if m.Item(name).Chosen() {
							a.BranchInput = name
							a.RunWrite(domain.SwitchBranch)
						}
						count++
					}
					if count == 0 {
						m.Item("No other local branches").Disabled(true)
					}
				})
				if m.Item("Fetch").Disabled(a.syncDisabled()).Chosen() {
					a.RunWrite(domain.Fetch)
				}
				m.Separator()
				m.Item("Compose Commits (Preview)...").Disabled(true)
				m.Item("Generate Commit Message with GitLens").Disabled(true)
				if m.Item("Add Co-authors...").Chosen() {
					if a.MessageInput != "" && !strings.HasSuffix(a.MessageInput, "\n") {
						a.MessageInput += "\n"
					}
					a.MessageInput += "\nCo-authored-by: "
				}
				m.Separator()
				m.Submenu("Commit", func(m *ui.Menu) {
					if m.Item("Commit (Amend)").Disabled(a.syncDisabled()).Chosen() {
						a.Amend = true
					}
					if m.Item("Commit").Disabled(a.commitUnavailable()).Chosen() {
						a.RunWrite(a.commitOperation())
					}
					if m.Item("Commit & Push").Disabled(a.commitUnavailable()).Chosen() {
						a.CommitAndPush()
					}
					if m.Item("Undo Last Commit").Disabled(a.syncDisabled()).Chosen() {
						a.RunWrite(domain.UndoCommit)
					}
				})
				m.Submenu("Changes", func(m *ui.Menu) {
					if m.Item("Stage All Changes").Disabled(a.syncDisabled() || !a.hasUnstagedChanges()).Chosen() {
						a.runBatchChanges(domain.Stage)
					}
					if m.Item("Unstage All Changes").Disabled(a.syncDisabled() || a.stagedCount() == 0).Chosen() {
						a.runBatchChanges(domain.Unstage)
					}
					discardPaths, discardReason := a.discardAllPaths()
					if m.Item("Discard All Changes").Disabled(a.syncDisabled() || discardReason != "" || len(discardPaths) == 0).Chosen() {
						a.discardAll()
					}
					if m.Item("Refresh").Chosen() {
						a.Refresh()
					}
				})
				m.Submenu("Pull, Push", func(m *ui.Menu) {
					if m.Item("Sync").Disabled(a.syncDisabled()).Chosen() {
						a.RunWrite(domain.Sync)
					}
					m.Separator()
					if m.Item("Pull").Disabled(a.syncDisabled()).Chosen() {
						a.RunWrite(domain.PullFFOnly)
					}
					if m.Item("Pull (Rebase)").Disabled(a.syncDisabled()).Chosen() {
						a.RunWrite(domain.PullRebase)
					}
					m.Submenu("Pull from...", func(m *ui.Menu) {
						if !a.remoteBranchMenu(m, domain.PullFrom) {
							if m.Item("Load Branches").Chosen() {
								a.loadBranches()
							}
						}
					})
					m.Separator()
					if m.Item("Push").Disabled(a.syncDisabled()).Chosen() {
						a.RunWrite(domain.Push)
					}
					m.Submenu("Push to...", func(m *ui.Menu) {
						if !a.remoteBranchMenu(m, domain.PushTo) {
							if m.Item("Load Branches").Chosen() {
								a.loadBranches()
							}
						}
					})
					m.Separator()
					if m.Item("Fetch").Disabled(a.syncDisabled()).Chosen() {
						a.RunWrite(domain.Fetch)
					}
					if m.Item("Fetch (Prune)").Disabled(a.syncDisabled()).Chosen() {
						a.RunWrite(domain.FetchPrune)
					}
					if m.Item("Fetch From All Remotes").Disabled(a.syncDisabled()).Chosen() {
						a.RunWrite(domain.FetchAll)
					}
				})
				m.Submenu("Branch", func(m *ui.Menu) {
					if m.Item("Show Branches").Chosen() {
						a.setMode("log")
					}
					m.Item("Branch management is in Git Log").Disabled(true)
				})
				m.Submenu("Remote", func(m *ui.Menu) {
					m.Item("Remote management is not available").Disabled(true)
				})
				m.Submenu("Stash", func(m *ui.Menu) {
					if m.Item("Stash Changes").Disabled(a.syncDisabled()).Chosen() {
						a.RunWrite(domain.StashPush)
					}
					if m.Item("Stash Changes (Include Untracked)").Disabled(a.syncDisabled()).Chosen() {
						a.RunWrite(domain.StashUntracked)
					}
					if m.Item("Pop Latest Stash").Disabled(a.syncDisabled()).Chosen() {
						a.RunWrite(domain.StashPop)
					}
					m.Separator()
					if m.Item("Show Stashes").Chosen() {
						a.setMode("log")
					}
				})
				m.Submenu("Tags", func(m *ui.Menu) {
					m.Item("Tag management is not available").Disabled(true)
				})
				m.Separator()
				if m.Item("Show Commit Graph").Chosen() {
					a.setMode("log")
				}
				if m.Item("Visualize Repository History").Chosen() {
					a.setMode("log")
				}
				m.Separator()
				if m.Item("Show Git Output").Chosen() {
					a.consoleOpen = true
				}
			})
		})
		formHeight := float32(160)
		if height := pane.Bounds().H; height > 0 {
			formHeight = min(160, max(100, height*0.4))
		}
		ui.Scroll(c).Label("Commit form scroll").MaxHeight(formHeight).FillWidth().Shrink(0).Children(func() { a.commitSidebar(c) })
		ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() { a.statusView(c) })
		if a.consoleOpen {
			a.consoleView(c)
		}
	})
}

func clampSidebarWidth(width float32) float32 {
	return max(220, min(480, width))
}

// sidebarDivider is the side bar's resize handle, mirroring the manual
// splitter used for the history details panel.
func (a *App) sidebarDivider(c *ui.Context, width float32) {
	divider := ui.Box(c).Key("tree-divider").Label("Resize branch tree").Role(ui.RoleSplitter).
		Width(5).FillHeight().Shrink(0).Focusable().Cursor(ui.CursorResizeEW)
	divider.Dragged() // Register drag input before applying this frame's events.
	left, right := divider.Shortcut(0, ui.KeyLeft), divider.Shortcut(0, ui.KeyRight)
	dx, _, held := divider.Dragged()
	next := width
	if left {
		next -= 10
	}
	if right {
		next += 10
	}
	if held {
		divider.Focus()
		next += dx
	}
	next = clampSidebarWidth(next)
	if next != width {
		a.SidebarWidth = next
	}
	divider.Draw(func(p *ui.Painter, r ui.Rect) {
		r.X += 2
		r.W = 1
		p.Fill(r, c.Theme().Border, 0)
	})
}
