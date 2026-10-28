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
	BranchTreeWidth         float32 `json:"branchTreeWidth,omitempty"`
	HistoryDetailsOpen      bool    `json:"historyDetailsOpen,omitempty"`
	HistoryDetailsCollapsed bool    `json:"historyDetailsCollapsed,omitempty"`
	HistoryDetailsHeight    float32 `json:"historyDetailsHeight,omitempty"`
	SidebarWidth            float32 `json:"sidebarWidth"`
	InspectorWidth          float32 `json:"inspectorWidth"`
	Mode                    string  `json:"mode"`
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
	if p.BranchTreeWidth >= 160 && p.BranchTreeWidth <= 600 {
		a.BranchTreeWidth = p.BranchTreeWidth
	}
	a.HistoryDetailsOpen = p.HistoryDetailsOpen || !p.HistoryDetailsCollapsed
	if p.HistoryDetailsHeight >= 80 && p.HistoryDetailsHeight <= 600 {
		a.HistoryDetailsHeight = p.HistoryDetailsHeight
	}
	switch p.Mode {
	case "commit", "log", "editor":
		a.Mode = p.Mode
	}
	switch a.Mode {
	case "commit":
		a.Pane = "Status"
	case "editor":
		a.Pane = "Editor"
	default:
		a.Pane = ""
	}
}
func (a *App) savePrefs() {
	path := a.prefsPath()
	if path == "" {
		return
	}
	p := uiPrefs{BranchTreeWidth: a.BranchTreeWidth, HistoryDetailsOpen: a.HistoryDetailsOpen, HistoryDetailsCollapsed: !a.HistoryDetailsOpen, HistoryDetailsHeight: a.HistoryDetailsHeight, SidebarWidth: a.SidebarWidth, InspectorWidth: a.StatusWidth, Mode: a.Mode}
	// Failures are silent: layout preferences must not block closing.
	_ = storage.SaveJSON(path, p)
}

// PrefsPathForTest exposes the derived preferences path for layout tests.
func (a *App) PrefsPathForTest() string { return a.prefsPath() }
