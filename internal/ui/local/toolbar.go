package local

import (
	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

// compactLabel keeps context controls usable at narrow window widths; the
// complete value remains available in the control's tooltip.
func compactLabel(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit-1]) + "…"
}

// toolbar.go owns the unified top toolbar: workspace and repository context,
// branch switcher and the global git actions. Mode navigation lives in rail.go.
// All writes use the existing confirmation and serialization flow.

func (a *App) toolbarView(c *ui.Context) {
	bar := c.TitleBar()
	ui.Row(c).Height(52).FillWidth().Padding(0, bar.Right, 0, bar.Left).Background(c.Theme().Surface).Children(func() {
		ui.ScrollHorizontal(c).Label("Main toolbar").Height(52).Grow(1).MinWidth(0).Children(func() {
			ui.Row(c).Gap(12).Padding(6, 12).Height(52).FillWidth().AlignItems(ui.Center).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
					logo := gitLogoIcon
					if c.Theme().Dark {
						logo = gitLogoDarkIcon
					}
					ui.Icon(c, logo).Size(20, 20).Shrink(0)
					ui.Text(c, "Git Viewer").Bold().NoWrap()
				})
				workspace := workspaceLabel(a.Workspace.FilePath)
				ui.Popover(c, ui.Button(c, "Workspace: "+compactLabel(workspace, 28)+" ▾").Height(34).Tooltip(workspace), &a.WorkspaceMenuOpen, func() {
					ui.Scroll(c).Width(300).MaxHeight(380).Children(func() {
						ui.Column(c).FillWidth().Padding(8).Gap(6).Children(func() { a.workspaceSidebar(c) })
					})
				})
				a.repoMenu(c)
				branch := a.currentBranch()
				branchButton := ui.Button(c, "").Label("Branch: " + branch).Tooltip(branch).Height(34).Gap(8).Shrink(0).Children(func() {
					ui.Icon(c, branchIcon).Size(16, 16).Shrink(0)
					ui.Text(c, compactLabel(branch, 20)).NoWrap()
					ui.Text(c, "▾").TextColor(c.Theme().TextMuted)
				})
				ui.Popover(c, branchButton, &a.BranchPopOpen, func() {
					ui.Scroll(c).Width(260).MaxHeight(380).Children(func() {
						ui.Column(c).FillWidth().Padding(8).Children(func() { a.logBranches(c) })
					})
				})
				ui.Box(c).Grow(1).Height(34).DragWindow()
				ui.Row(c).Gap(0).Border(1, c.Theme().Border).Radius(c.Theme().Radius).Clip().Shrink(0).Children(func() {
					toolbarAction(c, "Fetch", "Fetch", fetchIcon).Disabled(a.syncDisabled()).OnClick(func() { a.RunWrite(domain.Fetch) })
					toolbarAction(c, "Pull", "Pull (fast-forward only)", pullIcon).BorderWidth(0, 0, 0, 1).BorderColor(c.Theme().Border).Label("Pull (fast-forward)").Disabled(a.syncDisabled()).OnClick(func() { a.RunWrite(domain.PullFFOnly) })
					toolbarAction(c, "Push", "Push", pushIcon).BorderWidth(0, 0, 0, 1).BorderColor(c.Theme().Border).Disabled(a.syncDisabled()).OnClick(func() { a.RunWrite(domain.Push) })
				})
				toolbarIconButton(c, "Refresh", "Refresh", refreshIcon).OnClick(a.Refresh)
				toolbarIconButton(c, "Terminal", "Terminal is not available yet", terminalIcon).Disabled(true)
				ui.Popover(c, toolbarIconButton(c, "Connection / Git settings", "Connection / Git", settingsIcon), &a.ConnectionMenuOpen, func() {
					ui.Scroll(c).Width(380).MaxHeight(380).Children(func() {
						ui.Column(c).FillWidth().Padding(8).Gap(6).Children(func() {
							ui.TextInput(c, &a.PathInput).Label("Repository path").Placeholder("Local directory path").Width(260)
							ui.Button(c, "Open path").OnClick(func() { a.Open(a.PathInput) })
							ui.Combobox(c, &a.SSHHost, a.SSHHosts).Label("SSH host").Placeholder("SSH Host alias").Width(150)
							ui.TextInput(c, &a.SSHPath).Label("Remote repository path").Placeholder("/absolute/remote/path").Grow(1)
							ui.Button(c, "Connect SSH").Disabled(a.Loading).OnClick(a.OpenSSH)
							ui.Button(c, "Status").OnClick(func() { a.Pane = "Status" })
							ui.Button(c, "History / Graph").Disabled(a.Loading).OnClick(func() { a.LoadHistory(false) })
							ui.Button(c, "Branches / Worktrees").Disabled(a.Loading).OnClick(a.LoadBranches)
						})
					})
				})
			})
		})
		ui.Box(c).Label("Drag window").Width(16).FillHeight().Shrink(0).DragWindow()
	})
}
