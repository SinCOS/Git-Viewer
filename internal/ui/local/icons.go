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

// toolbarAction keeps the Git actions labeled alongside their icons.
func toolbarAction(c *ui.Context, label, tooltip string, icon *ui.SVG) ui.Element {
	return ui.Button(c, "").Label(label).Tooltip(tooltip).Height(34).Padding(0, 12).Radius(0).BorderWidth(0).Shrink(0).Gap(6).Children(func() {
		ui.Icon(c, icon).Size(16, 16).Shrink(0)
		ui.Text(c, label).NoWrap()
	})
}

// toolbarIconButton matches the larger standalone buttons in the top toolbar.
func toolbarIconButton(c *ui.Context, label, tooltip string, icon *ui.SVG) ui.Element {
	return ui.Button(c, "").Label(label).Tooltip(tooltip).Size(34, 34).
		Padding(0).Gap(0).AlignItems(ui.Center).Justify(ui.Center).Shrink(0).
		Children(func() { ui.Icon(c, icon).Size(18, 18).Shrink(0) })
}

var (
	gitLogoIcon     = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 20 20"><path fill="currentColor" d="M9 1.2a1.4 1.4 0 0 1 2 0l7.8 7.8a1.4 1.4 0 0 1 0 2L11 18.8a1.4 1.4 0 0 1-2 0L1.2 11a1.4 1.4 0 0 1 0-2z"/><path fill="none" stroke="white" stroke-width="1.3" d="M7 4.5l2.5 2.5v6M9.5 7l3 3"/><circle fill="white" cx="7" cy="4.5" r="1.1"/><circle fill="white" cx="9.5" cy="13" r="1.1"/><circle fill="white" cx="12.5" cy="10" r="1.1"/></svg>`))
	gitLogoDarkIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20" viewBox="0 0 20 20"><path fill="currentColor" d="M9 1.2a1.4 1.4 0 0 1 2 0l7.8 7.8a1.4 1.4 0 0 1 0 2L11 18.8a1.4 1.4 0 0 1-2 0L1.2 11a1.4 1.4 0 0 1 0-2z"/><path fill="none" stroke="#1c1c1c" stroke-width="1.3" d="M7 4.5l2.5 2.5v6M9.5 7l3 3"/><circle fill="#1c1c1c" cx="7" cy="4.5" r="1.1"/><circle fill="#1c1c1c" cx="9.5" cy="13" r="1.1"/><circle fill="#1c1c1c" cx="12.5" cy="10" r="1.1"/></svg>`))
	branchIcon      = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5"><circle cx="6" cy="4" r="2"/><circle cx="6" cy="16" r="2"/><circle cx="14" cy="5" r="2"/><path d="M6 6v8m8-7c0 4-3 5-8 5"/></svg>`))
	terminalIcon    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><path d="m4 4 6 6-6 6m8 0h4"/></svg>`))
	settingsIcon    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"><path d="m8 2-.5 2a6 6 0 0 0-1.4.8l-2-.6-2 3.4 1.5 1.5a7 7 0 0 0 0 1.8L2.1 12.4l2 3.4 2-.6c.4.3.9.6 1.4.8l.5 2h4l.5-2c.5-.2 1-.5 1.4-.8l2 .6 2-3.4-1.5-1.5a7 7 0 0 0 0-1.8l1.5-1.5-2-3.4-2 .6a6 6 0 0 0-1.4-.8L12 2z"/><circle cx="10" cy="10" r="2.5"/></svg>`))
	unifiedDiffIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.4"><path d="M3 5h14M3 10h14M3 15h14"/><path d="M7 8.5v3M13 8.5v3"/></svg>`))
	splitDiffIcon   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.4"><rect x="2.5" y="3.5" width="15" height="13" rx="1"/><path d="M10 3.5v13M5 7h2.5M12.5 7H15M5 11h2.5M12.5 11H15"/></svg>`))
	refIcon         = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M3 10l8-8h6v6l-8 9z"/><circle cx="13.5" cy="5.5" r="1.2"/></svg>`))
	starIcon        = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="currentColor"><path d="M10 1l2.8 5.7 6.2.9-4.5 4.4 1.1 6.2-5.6-3-5.6 3 1.1-6.2L1 7.6l6.2-.9z"/></svg>`))
	stageIcon       = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M11 17H4V3h7l4 4v3M11 3v4h4M14 11v7m-3-3.5h6"/></svg>`))
	unstageIcon     = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M11 17H4V3h7l4 4v3M11 3v4h4M11 14.5h6"/></svg>`))
	folderIcon      = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M2.5 6V4.5h5l2 2h8v9h-15V6Z"/></svg>`))
	plusIcon        = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M10 4v12M4 10h12"/></svg>`))
	folderOpenIcon  = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.5"><path d="M2.5 15.5v-11h5l2 2h7v3M2.5 15.5l3-6h12l-3 6h-12Z"/></svg>`))
	fetchIcon       = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M16 7a6 6 0 1 0 .2 5M16 3v4h-4"/></svg>`))
	pullIcon        = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M10 2v10m-4-4 4 4 4-4M3 12v5h14v-5"/></svg>`))
	pushIcon        = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M10 12V2m-4 4 4-4 4 4M3 12v5h14v-5"/></svg>`))
	listIcon        = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M7 5h10M7 10h10M7 15h10M3 5h1M3 10h1M3 15h1"/></svg>`))
	treeIcon        = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M4 3v11h4M4 7h4M12 7h5M12 14h5"/><rect x="8" y="5" width="4" height="4" rx=".5"/><rect x="8" y="12" width="4" height="4" rx=".5"/></svg>`))
	refreshIcon     = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M16 8a6 6 0 1 0 0 5M16 3v5h-5"/></svg>`))
	commitIcon      = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><circle cx="10" cy="10" r="7"/><path d="M7 10.2l2 2 4-4.4"/></svg>`))
	logIcon         = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><circle cx="6" cy="5.5" r="2.4"/><circle cx="6" cy="14.5" r="2.4"/><circle cx="14" cy="10" r="2.4"/><path d="M6 7.9v4.2M8.2 6.4c3 .8 3.4 1.6 3.6 3.6M8.2 13.6c3-.8 3.4-1.6 3.6-3.6"/></svg>`))
	stashIcon       = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M3 7.5 10 4l7 3.5M3 7.5v5L10 16l7-3.5v-5M3 7.5 10 11l7-3.5M10 11v5"/></svg>`))
	codeIcon        = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M6.5 6 3 10l3.5 4M13.5 6 17 10l-3.5 4M11.5 4l-3 12"/></svg>`))
)
