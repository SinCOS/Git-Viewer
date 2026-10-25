package local

import (
	"github.com/egoist/mygo/ui"
)

// icons.go holds the inline SVG icons for the mode rail. Drawn at 16×16,
// single-stroke, mirroring the approved HTML mockup (ui-idea-mockup.html).
var (
	commitIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><circle cx="10" cy="10" r="7"/><path d="M7 10.2l2 2 4-4.4"/></svg>`))
	logIcon    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><circle cx="6" cy="5.5" r="2.4"/><circle cx="6" cy="14.5" r="2.4"/><circle cx="14" cy="10" r="2.4"/><path d="M6 7.9v4.2M8.2 6.4c3 .8 3.4 1.6 3.6 3.6M8.2 13.6c3-.8 3.4-1.6 3.6-3.6"/></svg>`))
	wtIcon     = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M3 6.5V14a1.5 1.5 0 0 0 1.5 1.5H10"/><path d="M3 6.5A1.5 1.5 0 0 1 4.5 5H8l1.5 1.5h6A1.5 1.5 0 0 1 17 8v1.5"/><circle cx="14.5" cy="13" r="2"/><path d="M14.5 15v2.5"/></svg>`))
)
