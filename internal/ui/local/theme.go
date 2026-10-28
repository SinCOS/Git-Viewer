package local

import "github.com/egoist/mygo/ui"

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
