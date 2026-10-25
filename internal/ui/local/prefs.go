package local

import (
	"encoding/json"
	"os"
	"path/filepath"

	"gitviewer/internal/infrastructure/storage"
)

// prefs.go persists layout preferences (splitter widths and the active mode)
// next to the recent-repositories file, with the same atomic JSON mechanism.
// Window bounds need mygo.WindowOptions at creation and are not covered here.

type uiPrefs struct {
	SidebarWidth   float32 `json:"sidebarWidth"`
	InspectorWidth float32 `json:"inspectorWidth"`
	Mode           string  `json:"mode"`
}

func (a *App) prefsPath() string {
	base := a.Service.Recent.Path
	if base == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(base), "ui-prefs.json")
}
func (a *App) loadPrefs() {
	path := a.prefsPath()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var p uiPrefs
	if json.Unmarshal(data, &p) != nil {
		return
	}
	if p.SidebarWidth >= 160 {
		a.SidebarWidth = p.SidebarWidth
	}
	if p.InspectorWidth >= 260 {
		a.StatusWidth = p.InspectorWidth
	}
	switch p.Mode {
	case "commit", "log", "worktree":
		a.Mode = p.Mode
	}
	switch a.Mode {
	case "commit":
		a.Pane = "Status"
	case "worktree":
		a.Pane = "Branches"
	default:
		a.Pane = ""
	}
}
func (a *App) savePrefs() {
	path := a.prefsPath()
	if path == "" {
		return
	}
	p := uiPrefs{SidebarWidth: a.SidebarWidth, InspectorWidth: a.StatusWidth, Mode: a.Mode}
	// Failures are silent: layout preferences must not block closing.
	_ = storage.SaveJSON(path, p)
}

// PrefsPathForTest exposes the derived preferences path for layout tests.
func (a *App) PrefsPathForTest() string { return a.prefsPath() }
