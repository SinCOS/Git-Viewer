package local

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

func TestDiffSideBySideVirtualFind(t *testing.T) {
	var text strings.Builder
	text.WriteString("@@ -1,300 +1,300 @@\n")
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&text, "-old %d\n", i)
	}
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&text, "+new %d\n", i)
	}
	a, tt := newDiffTestApp(t)
	a.applyPreview(domain.DiffPreview{Scope: domain.WorktreeDiff, Text: text.String()})
	tt.Frame()
	if err := tt.Click("Side-by-Side"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.DiffBuiltRows == 0 || a.DiffBuiltRows > 100 {
		t.Fatalf("built %d rows", a.DiffBuiltRows)
	}
	if err := tt.Click("Find"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Type("new 290")
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	first, last := a.DiffList.Visible()
	target := a.findState().indices[1+300+290]
	if first > target || last < target || a.DiffBuiltRows > 100 {
		t.Fatalf("find target %d visible %d..%d, built %d", target, first, last, a.DiffBuiltRows)
	}
}
