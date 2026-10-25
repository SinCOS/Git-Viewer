package poc

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

func TestVirtualList(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.HistoryView, 600, 700)
	if a.BuiltRows >= 100 || a.BuiltRows == 0 {
		t.Fatalf("built %d of 10,000", a.BuiltRows)
	}
	if !tt.HasText("Commit 00000") {
		t.Fatal(tt.Texts())
	}
	if err := tt.Click("Commit 00002"); err != nil {
		t.Fatal(err)
	}
	if a.Selected != 2 {
		t.Fatal(a.Selected)
	}
	if err := tt.Click("Jump to 9999"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Commit 09999") {
		t.Fatal(tt.Texts())
	}
	if err := tt.Click("Commit 09999"); err != nil {
		t.Fatal(err)
	}
	if a.Selected != 9999 {
		t.Fatal(a.Selected)
	}
	tt.Scroll(300, 300, 0, -500)
	tt.Frame()
	if tt.HasText("Commit 09999") {
		t.Fatal("scroll did not move away from end")
	}
	if a.BuiltRows >= 100 {
		t.Fatal("virtualization lost")
	}
	t.Logf("built %d rows per pass for 10,000 commits", a.BuiltRows)
}
func TestThreePaneDragAndShortcut(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.View, 1400, 850)
	before := a.SidebarWidth
	// Outer splitter is at sidebar width; toolbar occupies the top.
	tt.Press(before, 400)
	tt.Move(before+60, 400)
	tt.Release(before+60, 400)
	if a.SidebarWidth <= before {
		t.Fatalf("splitter did not move: %v", a.SidebarWidth)
	}
	callbacks := make(chan func(), 2)
	a.Tasks.Dispatch = func(fn func()) { callbacks <- fn }
	tt.Key(ui.Cmd, ui.KeyR)
	select {
	case fn := <-callbacks:
		fn()
	case <-time.After(5 * time.Second):
		t.Fatal("shortcut did not start health")
	}
	a.Tasks.Wait()
	tt.Frame()
	if a.HealthChecks != 1 || a.Tasks.Connection != "online" {
		t.Fatalf("%+v", &a.Tasks)
	}
	a.Tasks.Close()
}
func TestGraphDrawingHitAndScroll(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.GraphView, 900, 450)
	// All 200 nodes are visible in this independent POC viewport.
	tt.ClickAt(20, 30)
	if a.GraphSelected != 0 {
		t.Fatal(a.GraphSelected)
	}
	tt.ClickAt(780, 372)
	if a.GraphSelected != 199 {
		t.Fatal(a.GraphSelected)
	}
	if tt.Image() == nil {
		t.Fatal("missing graph raster")
	}
	tt.SetSize(300, 200)
	tt.Scroll(100, 100, 400, 200)
	if a.GraphScroll.X <= 0 || a.GraphScroll.Y <= 0 {
		t.Fatalf("scroll=%+v", a.GraphScroll)
	}
	t.Logf("200-node graph scroll x=%.1f y=%.1f", a.GraphScroll.X, a.GraphScroll.Y)
	if folder := os.Getenv("GITVIEWER_ARTIFACTS"); folder != "" {
		if err := os.MkdirAll(folder, 0755); err != nil {
			t.Fatal(err)
		}
		tt.SetSize(900, 450)
		f, err := os.Create(filepath.Join(folder, "graph-poc.png"))
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
func TestDiffSelectionAndBinary(t *testing.T) {
	a := New()
	tt := ui.NewTester(a.DiffView, 600, 400)
	if !tt.HasText("+ new text 新内容") {
		t.Fatal(tt.Texts())
	}
	r, ok := tt.Find("+ new text 新内容")
	if !ok {
		t.Fatal("text missing")
	}
	tt.ClickAt(r.X+20, r.Y+r.H/2)
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Key(ui.Cmd, ui.KeyC)
	if tt.Clipboard() == "" {
		t.Fatal("selectable diff did not copy text")
	}
	tt.Scroll(r.X+20, r.Y+r.H/2, 300, 0)
	if a.DiffScroll.X <= 0 {
		t.Fatal("long diff line did not scroll horizontally")
	}
	if err := tt.Click("Binary / text"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Binary file · preview unavailable") {
		t.Fatal(tt.Texts())
	}
}
func TestTaskCancellationAndStaleResults(t *testing.T) {
	callbacks := make(chan func(), 4)
	tasks := Tasks{Dispatch: func(fn func()) { callbacks <- fn }}
	started := make(chan struct{})
	tasks.Start(func(ctx context.Context) (string, error) { close(started); <-ctx.Done(); return "", ctx.Err() })
	<-started
	if !tasks.Loading {
		t.Fatal("no loading state")
	}
	tasks.Cancel()
	tasks.Wait()
	(<-callbacks)()
	if tasks.Loading || tasks.Message != "OperationCancelled" || !tasks.Stale {
		t.Fatalf("%+v", &tasks)
	}
	tasks.Start(func(context.Context) (string, error) { return "", &domain.GitError{Code: domain.RemoteUnavailable} })
	tasks.Wait()
	(<-callbacks)()
	if tasks.Connection != "offline" || !tasks.Stale || tasks.Message != "RemoteUnavailable" {
		t.Fatalf("%+v", &tasks)
	}
	tasks.Start(func(context.Context) (string, error) { return "recovered", nil })
	tasks.Wait()
	(<-callbacks)()
	if tasks.Connection != "online" || tasks.Stale {
		t.Fatalf("%+v", &tasks)
	}
	tasks.Start(func(ctx context.Context) (string, error) { <-ctx.Done(); return "late", ctx.Err() })
	tasks.Close()
	tasks.Wait()
	(<-callbacks)()
	if !tasks.closed || tasks.Message != "OperationCancelled" {
		t.Fatal("closed window accepted completion")
	}
}
func BenchmarkVirtualList10000(b *testing.B) {
	a := New()
	tt := ui.NewTester(a.HistoryView, 600, 700)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.List.ScrollTo((i*37)%CommitCount, ui.Center)
		tt.Frame()
	}
}
func BenchmarkGraph200(b *testing.B) {
	a := New()
	tt := ui.NewTester(a.GraphView, 900, 450)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tt.Frame()
	}
}
