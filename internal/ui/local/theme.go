package local

import "github.com/egoist/mygo/ui"

const (
	ThemeStyleVSCode = "vscode"
	ThemeStyleClaude = "claude"
)

func appTheme(style string, base *ui.Theme) *ui.Theme {
	if style == ThemeStyleClaude {
		return claudeTheme(base)
	}
	return vscodeTheme(base)
}

// vscodeTheme restyles the base theme with the VS Code palette in both
// appearances: white editor surfaces, muted chrome, blue accents and
// near-square controls.
func vscodeTheme(base *ui.Theme) *ui.Theme {
	t := *base
	if base.Dark {
		t.Background = ui.Hex("#1f1f1f")
		t.Surface = ui.Hex("#181818")
		t.SurfaceHover = ui.Hex("#2a2d2e")
		t.SurfacePressed = ui.Hex("#313131")
		t.Border = ui.Hex("#2b2b2b")
		t.Text = ui.Hex("#cccccc")
		t.TextMuted = ui.Hex("#9d9d9d")
		t.Accent = ui.Hex("#0078d4")
		t.AccentHover = ui.Hex("#026ec1")
		t.AccentPressed = ui.Hex("#0258a8")
		t.Danger = ui.Hex("#f14c4c")
		t.Warning = ui.Hex("#cca700")
		t.Success = ui.Hex("#89d185")
		t.Selection = ui.Hex("#264f78")
		t.Focus = ui.Hex("#0078d4")
	} else {
		t.Background = ui.Hex("#ffffff")
		t.Surface = ui.Hex("#f8f8f8")
		t.SurfaceHover = ui.Hex("#e8e8e8")
		t.SurfacePressed = ui.Hex("#dcdcdc")
		t.Border = ui.Hex("#e5e5e5")
		t.Text = ui.Hex("#3b3b3b")
		t.TextMuted = ui.Hex("#616161")
		t.Accent = ui.Hex("#005fb8")
		t.AccentHover = ui.Hex("#0258a8")
		t.AccentPressed = ui.Hex("#024584")
		t.Danger = ui.Hex("#e51400")
		t.Warning = ui.Hex("#bf8803")
		t.Success = ui.Hex("#388a34")
		t.Selection = ui.Hex("#add6ff")
		t.Focus = ui.Hex("#005fb8")
	}
	t.Radius = 4
	return &t
}

// claudeTheme uses Anthropic's warm editorial palette: ivory paper, ink text,
// terracotta accents and softer corners in light mode, with a warm charcoal
// counterpart in dark mode.
func claudeTheme(base *ui.Theme) *ui.Theme {
	t := *base
	if base.Dark {
		t.Background = ui.Hex("#1f1e1d")
		t.Surface = ui.Hex("#272522")
		t.SurfaceHover = ui.Hex("#312e2a")
		t.SurfacePressed = ui.Hex("#3a3631")
		t.Border = ui.Hex("#3b3732")
		t.Text = ui.Hex("#f2eee6")
		t.TextMuted = ui.Hex("#aaa39a")
		t.Accent = ui.Hex("#d97757")
		t.AccentHover = ui.Hex("#e28768")
		t.AccentPressed = ui.Hex("#c96647")
		t.AccentText = ui.Hex("#ffffff")
		t.Danger = ui.Hex("#e07a6a")
		t.Warning = ui.Hex("#d8a657")
		t.Success = ui.Hex("#8eb38c")
		t.Selection = ui.Hex("#5a3b31")
		t.Focus = ui.Hex("#d97757")
		t.Scrollbar = ui.RGBA(242, 238, 230, 0.28)
	} else {
		t.Background = ui.Hex("#faf9f5")
		t.Surface = ui.Hex("#f0eee6")
		t.SurfaceHover = ui.Hex("#e8e4d9")
		t.SurfacePressed = ui.Hex("#ddd7ca")
		t.Border = ui.Hex("#d8d1c3")
		t.Text = ui.Hex("#242321")
		t.TextMuted = ui.Hex("#6f6a61")
		t.Accent = ui.Hex("#c96442")
		t.AccentHover = ui.Hex("#b55435")
		t.AccentPressed = ui.Hex("#99452d")
		t.AccentText = ui.Hex("#ffffff")
		t.Danger = ui.Hex("#b94a3d")
		t.Warning = ui.Hex("#a66a1f")
		t.Success = ui.Hex("#4f7d5c")
		t.Selection = ui.Hex("#ead0c4")
		t.Focus = ui.Hex("#c96442")
		t.Scrollbar = ui.RGBA(36, 35, 33, 0.26)
	}
	t.Radius = 8
	return &t
}
