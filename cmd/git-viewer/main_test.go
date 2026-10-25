package main

import (
	"github.com/egoist/mygo/ui"
	"gitviewer/internal/ui/poc"
	"testing"
)

func TestNativeShell(t *testing.T) {
	a := poc.New()
	tt := ui.NewTester(a.View, 1400, 850)
	if !tt.HasText("Git Viewer · Phase 0") {
		t.Fatal(tt.Texts())
	}
	if err := tt.Click("SSH fixture"); err != nil {
		t.Fatal(err)
	}
	if a.Repository != 1 {
		t.Fatal("mock repository selection failed")
	}
	if err := tt.Click("Graph / Diff"); err != nil {
		t.Fatal(err)
	}
	if !a.GraphMode {
		t.Fatal("graph toggle failed")
	}
}
