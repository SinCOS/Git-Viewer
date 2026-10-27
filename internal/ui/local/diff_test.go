package local_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/testgit"
)

func TestLongDiffVirtualizationAndCopy(t *testing.T) {
	a, queue := setup(t)
	dir := testgit.Repo(t)
	testgit.Write(t, filepath.Join(dir, "long"), strings.Repeat("中文 long line ", 100)+"\n"+strings.Repeat("row\n", 5000))
	a.Open(dir)
	settle(t, a, queue)
	a.Select(0)
	settle(t, a, queue)
	tt := ui.NewTester(a.View, 1400, 850)
	if a.DiffBuiltRows == 0 || a.DiffBuiltRows > 100 {
		t.Fatalf("virtual diff built %d rows", a.DiffBuiltRows)
	}
	if err := tt.Click("Diff settings"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Copy diff"); err != nil {
		t.Fatal(err)
	}
	if tt.Clipboard() != a.Preview.Text {
		t.Fatal("copy did not retain full preview")
	}
	a.DiffList.ScrollTo(4900, ui.Center)
	tt.Frame()
	first, last := a.DiffList.Visible()
	if first > 4900 || last < 4900 || a.DiffBuiltRows > 100 {
		t.Fatalf("scroll: visible=%d..%d built=%d", first, last, a.DiffBuiltRows)
	}
}
