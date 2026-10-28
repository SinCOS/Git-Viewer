package local

import (
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
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

// toolbar.go owns the VS Code-style title bar: the app title on the left, a
// centered command center and a draggable remainder. Repository actions live
// in the native menu bar for now; mode navigation lives in rail.go.

func (a *App) toolbarView(c *ui.Context) {
	bar := c.TitleBar()
	t := c.Theme()
	ui.Row(c).Label("Main toolbar").Height(36).FillWidth().Padding(0, bar.Right, 0, bar.Left).
		Background(t.Surface).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
		ui.Row(c).Gap(8).Padding(0, 12).Height(36).AlignItems(ui.Center).Shrink(0).Children(func() {
			logo := gitLogoIcon
			if t.Dark {
				logo = gitLogoDarkIcon
			}
			ui.Icon(c, logo).Size(16, 16).Shrink(0)
			ui.Text(c, "Git Viewer").FontSize(12).TextColor(t.TextMuted).NoWrap()
		})
		ui.Box(c).Grow(1).FillHeight().DragWindow()
		ui.ButtonBase(c).Label("Command center").Tooltip("Quick open (⌘P)").
			Width(380).MaxWidth(420).Height(24).Radius(6).
			Background(t.Background).Border(1, t.Border).
			AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer).
			OnClick(a.OpenQuickOpen).Children(func() {
			ui.Text(c, "Search files (⌘P)").FontSize(12).TextColor(t.TextMuted).NoWrap()
		})
		ui.Box(c).Grow(1).FillHeight().DragWindow()
		ui.Row(c).Gap(4).Height(36).AlignItems(ui.Center).Shrink(0).Children(func() {
			a.workspaceMenu(c)
			a.repoMenu(c)
			context := ""
			if a.Workspace.FilePath == "" && a.FolderPath != "" {
				context = compactLabel(filepath.Base(a.FolderPath), 20)
			} else if a.Workspace.FilePath == "" {
				context = compactLabel(a.currentBranch(), 20)
			}
			if context != "" {
				ui.Text(c, context).FontSize(11).TextColor(t.TextMuted).
					Padding(0, 8).Shrink(0).NoWrap()
			}
		})
	})
}

func (a *App) workspaceName() string {
	name := filepath.Base(a.Workspace.FilePath)
	return strings.TrimSuffix(name, filepath.Ext(name))
}
