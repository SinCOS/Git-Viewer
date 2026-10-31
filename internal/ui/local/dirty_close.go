package local

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

func (a *App) hasDirtyEditorTabs() bool {
	for _, tab := range a.EditorTabs {
		if tab.state.Buffer.Modified() {
			return true
		}
	}
	return false
}

// CanCloseWindow reports whether a window close can proceed without losing
// editor buffers.
func (a *App) CanCloseWindow() bool {
	return !a.hasDirtyEditorTabs()
}

func (a *App) WindowCloseAllowed() bool {
	return a.allowWindowClose
}

// ShowWindowClosePrompt defers closing until every dirty tab is resolved.
func (a *App) ShowWindowClosePrompt(closeWindow func()) {
	var proceed func()
	proceed = func() {
		a.promptNextDirtyClose(func() {
			a.allowWindowClose = true
			if closeWindow != nil {
				closeWindow()
			}
		})
	}
	proceed()
}

func (a *App) promptNextDirtyClose(after func()) {
	for i, tab := range a.EditorTabs {
		if tab.state.Buffer.Modified() {
			a.EditorActive = i
			a.dirtyCloseTab = i
			a.dirtyCloseAction = after
			a.DirtyCloseOpen = true
			return
		}
	}
	if after != nil {
		after()
	}
}

func (a *App) requestCloseEditorTab(i int) {
	if i < 0 || i >= len(a.EditorTabs) {
		return
	}
	tab := a.EditorTabs[i]
	if !tab.state.Buffer.Modified() {
		a.closeEditorTab(i)
		return
	}
	a.EditorActive = i
	a.dirtyCloseTab = i
	a.dirtyCloseAction = func() {
		if i >= 0 && i < len(a.EditorTabs) && a.EditorTabs[i] == tab {
			a.closeEditorTab(i)
		}
	}
	a.DirtyCloseOpen = true
}

func (a *App) resolveDirtyClose(save bool) {
	i := a.dirtyCloseTab
	if i < 0 || i >= len(a.EditorTabs) {
		a.DirtyCloseOpen = false
		return
	}
	tab := a.EditorTabs[i]
	a.EditorActive = i
	after := a.dirtyCloseAction
	if save {
		if err := a.saveEditorTab(tab); err != nil {
			a.Message = message(err)
			a.DirtyCloseOpen = false
			return
		}
		// A remote conflict schedules its own prompt and leaves the buffer
		// dirty, so it is unsafe to continue the close.
		if tab.state.Buffer.Modified() {
			a.Message = "Resolve the file conflict before closing"
			a.DirtyCloseOpen = false
			return
		}
	}
	a.DirtyCloseOpen = false
	a.dirtyCloseTab = -1
	a.dirtyCloseAction = nil
	if after != nil {
		after()
	}
}

func (a *App) dirtyCloseView(c *ui.Context) {
	ui.Modal(c, &a.DirtyCloseOpen, func() {
		tab := (*editorTab)(nil)
		if a.dirtyCloseTab >= 0 && a.dirtyCloseTab < len(a.EditorTabs) {
			tab = a.EditorTabs[a.dirtyCloseTab]
		}
		name := "this file"
		path := ""
		if tab != nil {
			name = tab.name()
			path = tab.path()
		}
		width, height := c.Size()
		w := min(float32(520), max(float32(360), width-80))
		h := min(float32(260), max(float32(190), height-80))
		ui.Column(c).Label("Unsaved changes").Width(w).MaxHeight(h).Gap(0).
			Background(c.Theme().Background).Border(1, c.Theme().Border).Radius(10).Clip().Children(func() {
			ui.Row(c).Height(44).FillWidth().Padding(0, 16).AlignItems(ui.Center).
				Background(c.Theme().Surface).BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Children(func() {
				ui.Text(c, "Unsaved changes").Bold().FontSize(14).Grow(1)
			})
			ui.Column(c).Grow(1).FillWidth().Padding(16).Gap(8).Children(func() {
				ui.Text(c, fmt.Sprintf("%s has unsaved changes.", name)).FontSize(13)
				ui.Text(c, path).FontSize(11).TextColor(c.Theme().TextMuted).NoWrap()
				ui.Text(c, "Save before closing, discard the changes, or return to the editor.").FontSize(11).TextColor(c.Theme().TextMuted)
			})
			ui.Row(c).Height(48).FillWidth().Padding(0, 16).Gap(8).AlignItems(ui.Center).Justify(ui.End).
				Background(c.Theme().Surface).BorderWidth(1, 0, 0, 0).BorderColor(c.Theme().Border).Children(func() {
				ui.Button(c, "Cancel").Height(28).OnClick(func() {
					a.DirtyCloseOpen = false
					a.dirtyCloseAction = nil
					a.dirtyCloseTab = -1
				})
				ui.Button(c, "Discard").Height(28).Background(c.Theme().Danger).OnClick(func() {
					after := a.dirtyCloseAction
					a.DirtyCloseOpen = false
					a.dirtyCloseTab = -1
					a.dirtyCloseAction = nil
					if after != nil {
						after()
					}
				})
				ui.Button(c, "Save").Height(28).OnClick(func() { a.resolveDirtyClose(true) })
			})
		})
	})
}
