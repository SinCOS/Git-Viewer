package local

import (
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/storage"
)

func newHistoryTableApp(t *testing.T) (*App, *ui.Tester, domain.CommitRecord) {
	t.Helper()
	a := New(application.LocalService{})
	t.Cleanup(func() { a.Close(); a.Wait() })
	commit := domain.CommitRecord{
		OID:        "0123456789abcdef0123456789abcdef01234567",
		Subject:    "table selection",
		Author:     "Alice",
		Refs:       "HEAD -> main",
		AuthoredAt: time.Date(2026, 10, 28, 9, 30, 0, 0, time.UTC),
	}
	a.GraphRows = a.Graph.Append([]domain.CommitRecord{commit})
	tt := ui.NewTester(a.historyView, 1000, 500)
	tt.Frame()
	return a, tt, commit
}

func TestHistoryCellSelectionAndRowHeader(t *testing.T) {
	a, tt, commit := newHistoryTableApp(t)
	if err := tt.Click("Commit description " + commit.OID); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.historyCellSelection.Has(commit.OID) || !a.HistoryList.CellSelected(0, 1) || a.HistoryList.CellSelected(0, 2) {
		t.Fatalf("cell selection: chosen=%v cells=%+v", a.historyCellSelection.Len(), a.HistoryList.Cells)
	}
	if got := a.HistoryList.SelectedColumns(); !reflect.DeepEqual(got, []string{"description"}) {
		t.Fatalf("selected columns: %v", got)
	}

	if err := tt.Click("Commit graph " + commit.OID); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.HistoryList.Cells.All || !a.HistoryList.CellSelected(0, 5) {
		t.Fatalf("row header did not choose the whole row: %+v", a.HistoryList.Cells)
	}
	if got := a.HistoryList.SelectedColumns(); len(got) != 5 {
		t.Fatalf("whole-row columns: %v", got)
	}
}

func TestHistoryCellMenuCopiesCellAndHash(t *testing.T) {
	_, tt, commit := newHistoryTableApp(t)
	if err := tt.RightClick("Commit description " + commit.OID); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Copy cell"); err != nil {
		t.Fatal(err)
	}
	if got := tt.Clipboard(); got != commit.Subject {
		t.Fatalf("copied cell %q", got)
	}
	if err := tt.RightClick("Commit date " + commit.OID); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Copy commit hash"); err != nil {
		t.Fatal(err)
	}
	if got := tt.Clipboard(); got != commit.OID {
		t.Fatalf("copied hash %q", got)
	}
	if err := tt.RightClick("Commit graph " + commit.OID); err != nil {
		t.Fatal(err)
	}
	if menu := tt.Menu(); menu != nil {
		t.Fatalf("row header opened a cell menu: %v", menu)
	}
}

func TestHistoryColumnPreferencesPersistAndResetKeepsThem(t *testing.T) {
	service := application.LocalService{Recent: storage.RecentRepositories{Path: filepath.Join(t.TempDir(), "recent.json")}}
	a := New(service)
	want := ui.TableLayout{
		Order:  []string{"description", "author", "branches", "date", "commit"},
		Widths: map[string]float32{"author": 96, "description": 280},
	}
	a.HistoryList.Columns = want
	a.savePrefs()
	a.Close()

	b := New(service)
	t.Cleanup(func() { b.Close(); b.Wait() })
	if !reflect.DeepEqual(b.HistoryList.Columns, want) {
		t.Fatalf("loaded columns: %+v", b.HistoryList.Columns)
	}
	b.historyCellSelection.Add("stale")
	b.resetHistory()
	if !reflect.DeepEqual(b.HistoryList.Columns, want) || b.historyCellSelection.Len() != 0 {
		t.Fatalf("reset lost columns or kept cells: %+v chosen=%d", b.HistoryList.Columns, b.historyCellSelection.Len())
	}
}
