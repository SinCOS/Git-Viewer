package local

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
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
	native := a.nativeDiffState()
	if native == nil || native.Layout != editor.DiffSplit || a.DiffBuiltRows != 1 {
		t.Fatalf("native diff not active: %+v built=%d", native, a.DiffBuiltRows)
	}
	if err := tt.Click("Diff settings"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Find"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Type("new 290")
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if !tt.HasText("1 of 1") {
		t.Fatalf("native find did not select the only match: %v", tt.Texts())
	}
}
