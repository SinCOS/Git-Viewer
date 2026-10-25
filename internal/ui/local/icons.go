package local

import (
	"github.com/egoist/mygo/ui"
)

// icons.go holds the inline SVG icons for the mode rail. Drawn at 16×16,
// single-stroke, mirroring the approved HTML mockup (ui-idea-mockup.html).
// iconButton clears text-button padding so the SVG stays centered and unclipped.
func iconButton(c *ui.Context, label, tooltip string, icon *ui.SVG) ui.Element {
	return ui.Button(c, "").Label(label).Tooltip(tooltip).Size(28, 28).
		Padding(0).Gap(0).AlignItems(ui.Center).Justify(ui.Center).Shrink(0).
		Children(func() { ui.Icon(c, icon).Label(tooltip+" icon").Size(16, 16).Shrink(0) })
}

var (
	stageIcon      = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M11 17H4V3h7l4 4v3M11 3v4h4M14 11v7m-3-3.5h6"/></svg>`))
	unstageIcon    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M11 17H4V3h7l4 4v3M11 3v4h4M11 14.5h6"/></svg>`))
	folderIcon     = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M2.5 6V4.5h5l2 2h8v9h-15V6Z"/></svg>`))
	folderOpenIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M2.5 15.5v-11h5l2 2h7v3M2.5 15.5l3-6h12l-3 6h-12Z"/></svg>`))
	fetchIcon      = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M6 12H5a3 3 0 0 1-.4-6 5 5 0 0 1 9.6 0A3 3 0 0 1 15 12h-1M10 9v8m-3-3 3 3 3-3"/></svg>`))
	pullIcon       = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M10 2v10m-4-4 4 4 4-4M3 12v5h14v-5"/></svg>`))
	pushIcon       = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M10 12V2m-4 4 4-4 4 4M3 12v5h14v-5"/></svg>`))
	listIcon       = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M7 5h10M7 10h10M7 15h10M3 5h1M3 10h1M3 15h1"/></svg>`))
	treeIcon       = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M4 3v11h4M4 7h4M12 7h5M12 14h5"/><rect x="8" y="5" width="4" height="4" rx=".5"/><rect x="8" y="12" width="4" height="4" rx=".5"/></svg>`))
	refreshIcon    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M16 8a6 6 0 1 0 0 5M16 3v5h-5"/></svg>`))
	commitIcon     = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><circle cx="10" cy="10" r="7"/><path d="M7 10.2l2 2 4-4.4"/></svg>`))
	logIcon        = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><circle cx="6" cy="5.5" r="2.4"/><circle cx="6" cy="14.5" r="2.4"/><circle cx="14" cy="10" r="2.4"/><path d="M6 7.9v4.2M8.2 6.4c3 .8 3.4 1.6 3.6 3.6M8.2 13.6c3-.8 3.4-1.6 3.6-3.6"/></svg>`))
	wtIcon         = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M3 6.5V14a1.5 1.5 0 0 0 1.5 1.5H10"/><path d="M3 6.5A1.5 1.5 0 0 1 4.5 5H8l1.5 1.5h6A1.5 1.5 0 0 1 17 8v1.5"/><circle cx="14.5" cy="13" r="2"/><path d="M14.5 15v2.5"/></svg>`))
)
