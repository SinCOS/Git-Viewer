package local

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestClaudeThemeUsesWarmPaletteInBothAppearances(t *testing.T) {
	light := claudeTheme(ui.LightTheme())
	if light.Background != ui.Hex("#faf9f5") || light.Accent != ui.Hex("#c96442") || light.Radius != 8 {
		t.Fatalf("unexpected Claude light theme: %+v", light)
	}
	dark := claudeTheme(ui.DarkTheme())
	if dark.Background != ui.Hex("#1f1e1d") || dark.Accent != ui.Hex("#d97757") || dark.Radius != 8 {
		t.Fatalf("unexpected Claude dark theme: %+v", dark)
	}
}

func TestAppThemeFallsBackToVSCode(t *testing.T) {
	base := ui.LightTheme()
	if got := appTheme("unknown", base); got.Background != vscodeTheme(base).Background {
		t.Fatalf("unknown theme did not fall back: %+v", got)
	}
}
