package local

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/application"
	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
)

func TestHistoryBranchAndRemoteFilters(t *testing.T) {
	a := New(application.LocalService{})
	t.Cleanup(func() { a.Close(); a.Wait() })
	a.GraphRows = a.Graph.Append([]domain.CommitRecord{
		{OID: "main-head", Subject: "local-only commit", Parents: []string{"base"}, Refs: "HEAD -> main, origin/main"},
		{OID: "remote-head", Subject: "remote-only commit", Parents: []string{"base"}, Refs: "origin/feature"},
		{OID: "base", Subject: "common ancestor"},
	})
	a.Branches = []gitexec.BranchRecord{
		{Name: "refs/heads/main", OID: "main-head", Current: true},
		{Name: "refs/remotes/origin/main", OID: "main-head"},
		{Name: "refs/remotes/origin/feature", OID: "remote-head"},
	}
	tt := ui.NewTester(a.historyView, 1100, 700)
	if !tt.HasText("local-only commit") || !tt.HasText("remote-only commit") {
		t.Fatal(tt.Texts())
	}
	if err := tt.Click("Show Remote Branches"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("remote-only commit") || !tt.HasText("local-only commit") || !tt.HasText("common ancestor") {
		t.Fatal(tt.Texts())
	}
	if _, ok := tt.Find("Branch label origin/main"); ok {
		t.Fatal("remote ref badge remained visible")
	}
	if err := tt.Click("Show Remote Branches"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	a.HistoryBranch = "origin/feature (remote)"
	tt.Frame()
	if tt.HasText("local-only commit") || !tt.HasText("remote-only commit") || !tt.HasText("common ancestor") {
		t.Fatal(tt.Texts())
	}
	if len(a.historyVisible) != 2 || a.historyVisible[0] != 1 {
		t.Fatal("filtered row mapping changed", a.historyVisible)
	}
	a.HistoryQuery = "common"
	tt.Frame()
	if len(a.historyVisible) != 1 || a.historyVisible[0] != 2 {
		t.Fatal("search ignored branch filter", a.historyVisible)
	}
	a.HistoryShowRemote = false
	a.HistoryQuery = ""
	tt.Frame()
	if a.HistoryBranch != "Show All" || tt.HasText("remote-only commit") {
		t.Fatal("hidden remote selection was not reset", tt.Texts())
	}
	if a.Loading || a.Writing {
		t.Fatal("local filtering unexpectedly started Git work")
	}
}
