package local

import (
	"fmt"
	"maps"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// appSettings are user-editable preferences that change application behavior.
// Layout-only preferences stay in uiPrefs so existing files remain readable.
type appSettings struct {
	EditorFontSize    float64           `json:"editorFontSize,omitempty"`
	EditorLineNumbers bool              `json:"editorLineNumbers"`
	EditorWordWrap    bool              `json:"editorWordWrap"`
	TerminalFontSize  float64           `json:"terminalFontSize,omitempty"`
	TerminalHeight    float64           `json:"terminalHeight,omitempty"`
	SearchCase        bool              `json:"searchCase,omitempty"`
	SearchRegex       bool              `json:"searchRegex,omitempty"`
	SearchWhole       bool              `json:"searchWhole,omitempty"`
	SearchMaxResults  int               `json:"searchMaxResults,omitempty"`
	Keybindings       map[string]string `json:"keybindings,omitempty"`
}

func defaultAppSettings() appSettings {
	return appSettings{
		EditorFontSize:    13,
		EditorLineNumbers: true,
		TerminalFontSize:  13,
		TerminalHeight:    220,
		SearchMaxResults:  500,
		Keybindings: map[string]string{
			"file.openFolder":    "Cmd+O",
			"file.openWorkspace": "Cmd+Shift+O",
			"file.quickOpen":     "Cmd+P",
			"file.newFile":       "Cmd+N",
			"file.save":          "Cmd+S",
			"search.workspace":   "Cmd+Shift+F",
			"terminal.toggle":    "Cmd+J",
			"terminal.new":       "Cmd+Shift+`",
			"terminal.next":      "Cmd+Shift+]",
			"terminal.previous":  "Cmd+Shift+[",
			"settings.open":      "Cmd+,",
			"view.editor":        "Cmd+1",
			"view.log":           "Cmd+2",
			"view.commit":        "Cmd+3",
			"repo.refresh":       "Cmd+R",
		},
	}
}

func mergeAppSettings(saved appSettings) appSettings {
	out := defaultAppSettings()
	if saved.EditorFontSize > 0 {
		out.EditorFontSize = saved.EditorFontSize
	}
	if saved.TerminalFontSize > 0 {
		out.TerminalFontSize = saved.TerminalFontSize
	}
	if saved.TerminalHeight > 0 {
		out.TerminalHeight = saved.TerminalHeight
	}
	if saved.SearchMaxResults > 0 {
		out.SearchMaxResults = saved.SearchMaxResults
	}
	out.EditorLineNumbers = saved.EditorLineNumbers
	out.EditorWordWrap = saved.EditorWordWrap
	out.SearchCase = saved.SearchCase
	out.SearchRegex = saved.SearchRegex
	out.SearchWhole = saved.SearchWhole
	if saved.Keybindings != nil {
		out.Keybindings = maps.Clone(defaultAppSettings().Keybindings)
		for id, value := range saved.Keybindings {
			out.Keybindings[id] = value
		}
	}
	return out
}

func (a *App) bindingFor(id string) string {
	if a.Settings.Keybindings == nil {
		a.Settings.Keybindings = map[string]string{}
	}
	if value, ok := a.Settings.Keybindings[id]; ok {
		return value
	}
	return defaultAppSettings().Keybindings[id]
}

func (a *App) setBinding(id, value string) {
	if a.Settings.Keybindings == nil {
		a.Settings.Keybindings = map[string]string{}
	}
	a.Settings.Keybindings[id] = strings.TrimSpace(value)
	a.savePrefs()
}

// parseKeybinding accepts the small, explicit key grammar shown in Settings.
// Keeping the grammar bounded avoids silently registering surprising global
// shortcuts.
func parseKeybinding(value string) (ui.Modifiers, ui.Key, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, 0, false
	}
	parts := strings.Split(value, "+")
	var mods ui.Modifiers
	for _, part := range parts[:len(parts)-1] {
		switch strings.ToLower(strings.TrimSpace(part)) {
		case "cmd", "command", "super":
			mods |= ui.Cmd
		case "ctrl", "control":
			mods |= ui.Ctrl
		case "shift":
			mods |= ui.Shift
		case "alt", "option", "opt":
			mods |= ui.Alt
		default:
			return 0, 0, false
		}
	}
	key := keyFromName(strings.TrimSpace(parts[len(parts)-1]))
	if key == ui.KeyUnknown {
		return 0, 0, false
	}
	return mods, key, true
}

func keyFromName(name string) ui.Key {
	name = strings.ToLower(strings.TrimSpace(name))
	if len(name) == 1 {
		switch {
		case name[0] >= 'a' && name[0] <= 'z':
			return ui.KeyA + ui.Key(name[0]-'a')
		case name[0] >= '0' && name[0] <= '9':
			return ui.Key0 + ui.Key(name[0]-'0')
		}
	}
	switch name {
	case ",":
		return ui.KeyComma
	case ".":
		return ui.KeyPeriod
	case "/":
		return ui.KeySlash
	case ";":
		return ui.KeySemicolon
	case "'":
		return ui.KeyQuote
	case "`":
		return ui.KeyBackquote
	case "[":
		return ui.KeyBracketLeft
	case "]":
		return ui.KeyBracketRight
	case "\\":
		return ui.KeyBackslash
	case "enter", "return":
		return ui.KeyEnter
	case "escape", "esc":
		return ui.KeyEscape
	case "space":
		return ui.KeySpace
	case "tab":
		return ui.KeyTab
	case "backspace":
		return ui.KeyBackspace
	case "delete":
		return ui.KeyDelete
	case "up":
		return ui.KeyUp
	case "down":
		return ui.KeyDown
	case "left":
		return ui.KeyLeft
	case "right":
		return ui.KeyRight
	}
	return ui.KeyUnknown
}

func shortcutLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	replacer := strings.NewReplacer("Cmd+", "⌘", "Shift+", "⇧", "Alt+", "⌥", "Ctrl+", "⌃", "+", "")
	return replacer.Replace(value)
}

func (a *App) registerShortcuts(c *ui.Context) {
	for _, command := range a.commandList() {
		value := a.bindingFor(command.ID)
		if value == "" {
			continue
		}
		mods, key, ok := parseKeybinding(value)
		if !ok {
			continue
		}
		command := command
		c.OnShortcut(mods, key, command.Run)
	}
}

func (a *App) OpenSettings() {
	a.SettingsOpen = true
}

func (a *App) settingsView(c *ui.Context) {
	ui.Modal(c, &a.SettingsOpen, func() {
		width, height := c.Size()
		w := min(float32(760), max(float32(520), width-80))
		h := min(float32(640), max(float32(420), height-80))
		ui.Column(c).Label("Settings panel").Width(w).MaxHeight(h).Gap(0).
			Background(c.Theme().Background).Border(1, c.Theme().Border).Radius(10).Clip().Children(func() {
			ui.Row(c).Height(46).FillWidth().Padding(0, 16).Gap(8).AlignItems(ui.Center).
				Background(c.Theme().Surface).BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Children(func() {
				ui.Text(c, "Settings").Bold().FontSize(14).Grow(1)
				ui.Button(c, "Close").Height(26).OnClick(func() { a.SettingsOpen = false })
			})
			ui.Scroll(c).Label("Settings sections").Grow(1).FillWidth().MinHeight(0).Children(func() {
				ui.Column(c).FillWidth().Padding(18).Gap(16).Children(func() {
					a.settingsEditorSection(c)
					a.settingsSearchSection(c)
					a.settingsTerminalSection(c)
					a.settingsKeybindingsSection(c)
				})
			})
			ui.Row(c).Height(36).FillWidth().Padding(0, 16).Gap(12).AlignItems(ui.Center).
				Background(c.Theme().Surface).BorderWidth(1, 0, 0, 0).BorderColor(c.Theme().Border).Children(func() {
				ui.Text(c, "Preferences are saved automatically to ui-prefs.json").FontSize(10).TextColor(c.Theme().TextMuted).Grow(1)
				ui.Button(c, "Done").Height(26).OnClick(func() { a.SettingsOpen = false })
			})
		})
	})
}

func (a *App) settingsEditorSection(c *ui.Context) {
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		ui.Text(c, "Editor").Bold().FontSize(13)
		ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Font size").Width(120).FontSize(12)
			if ui.Stepper(c, &a.Settings.EditorFontSize, 10, 24, 1).Width(140).Changed() {
				a.savePrefs()
			}
			if ui.Checkbox(c, &a.Settings.EditorLineNumbers, "Show line numbers").Changed() {
				a.savePrefs()
			}
			if ui.Checkbox(c, &a.Settings.EditorWordWrap, "Word wrap").Changed() {
				a.savePrefs()
			}
		})
		ui.Text(c, "Theme").FontSize(12).TextColor(c.Theme().TextMuted)
		ui.Row(c).Gap(8).Children(func() {
			if ui.Button(c, "VS Code").Height(26).Clicked() {
				a.SetThemeStyle(ThemeStyleVSCode)
			}
			if ui.Button(c, "Claude").Height(26).Clicked() {
				a.SetThemeStyle(ThemeStyleClaude)
			}
		})
	})
}

func (a *App) settingsSearchSection(c *ui.Context) {
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		ui.Text(c, "Search").Bold().FontSize(13)
		ui.Row(c).Gap(14).Children(func() {
			if ui.Checkbox(c, &a.Settings.SearchCase, "Case sensitive").Changed() {
				a.savePrefs()
			}
			if ui.Checkbox(c, &a.Settings.SearchRegex, "Regular expression").Changed() {
				a.savePrefs()
			}
			if ui.Checkbox(c, &a.Settings.SearchWhole, "Whole word").Changed() {
				a.savePrefs()
			}
		})
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Maximum results").Width(120).FontSize(12)
			maxResults := float64(a.Settings.SearchMaxResults)
			ui.Stepper(c, &maxResults, 50, 5000, 50).Width(140)
			if int(maxResults) != a.Settings.SearchMaxResults {
				a.Settings.SearchMaxResults = int(maxResults)
				a.savePrefs()
			}
		})
	})
}

func (a *App) settingsTerminalSection(c *ui.Context) {
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		ui.Text(c, "Terminal").Bold().FontSize(13)
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Font size").Width(120).FontSize(12)
			if ui.Stepper(c, &a.Settings.TerminalFontSize, 10, 24, 1).Width(140).Changed() {
				a.savePrefs()
			}
			ui.Text(c, "Panel height").Width(90).FontSize(12)
			terminalHeight := float64(a.TerminalHeight)
			if ui.Stepper(c, &terminalHeight, 140, 520, 20).Width(140).Changed() {
				a.TerminalHeight = float32(terminalHeight)
				a.Settings.TerminalHeight = terminalHeight
				a.savePrefs()
			}
		})
		ui.Text(c, "The integrated terminal runs in the active local folder. Remote terminals are not enabled yet.").FontSize(11).TextColor(c.Theme().TextMuted)
	})
}

func (a *App) settingsKeybindingsSection(c *ui.Context) {
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		ui.Text(c, "Keyboard shortcuts").Bold().FontSize(13)
		ui.Text(c, "Use Cmd/Ctrl, Shift and Alt with a letter, digit, comma, backquote, or named key.").FontSize(11).TextColor(c.Theme().TextMuted)
		for _, command := range a.commandList() {
			command := command
			ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, command.Label).FontSize(12).Grow(1)
				value := a.bindingFor(command.ID)
				input := ui.TextInput(c, &value).Label("Shortcut for " + command.Label).Width(170).Height(26).FontSize(12)
				if input.Changed() {
					a.setBinding(command.ID, value)
				}
				if value != "" {
					if _, _, ok := parseKeybinding(value); !ok {
						ui.Text(c, "invalid").FontSize(10).TextColor(c.Theme().Danger)
					}
				}
			})
		}
		ui.Button(c, "Reset shortcuts").Height(26).OnClick(func() {
			a.Settings.Keybindings = maps.Clone(defaultAppSettings().Keybindings)
			a.savePrefs()
		})
	})
}

func (a *App) settingsSummary() string {
	return fmt.Sprintf("font %s, terminal %s, %d shortcuts", strconv.FormatFloat(a.Settings.EditorFontSize, 'f', 0, 64), strconv.FormatFloat(a.Settings.TerminalHeight, 'f', 0, 64), len(a.commandList()))
}
