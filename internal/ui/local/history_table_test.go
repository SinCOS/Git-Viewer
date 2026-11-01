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

func TestHistorySelectsWholeRows(t *testing.T) {
	a, tt, commit := newHistoryTableApp(t)
	if err := tt.Click("Commit description " + commit.OID); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.HistoryList.CellSelect || a.historyListSelected != 0 || a.HistorySelected != 0 {
		t.Fatalf("row not selected: cellSelect=%t list=%d history=%d", a.HistoryList.CellSelect, a.historyListSelected, a.HistorySelected)
	}
	if got := a.HistoryList.SelectedColumns(); len(got) != 0 {
		t.Fatalf("row selection exposed cell columns: %v", got)
	}

	if err := tt.Click("Commit date " + commit.OID); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.historyListSelected != 0 || a.HistorySelected != 0 {
		t.Fatalf("clicking another cell did not keep row selection: list=%d history=%d", a.historyListSelected, a.HistorySelected)
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
	b.resetHistory()
	if !reflect.DeepEqual(b.HistoryList.Columns, want) {
		t.Fatalf("reset lost columns: %+v", b.HistoryList.Columns)
	}
}
