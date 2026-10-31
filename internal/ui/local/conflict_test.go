package local

import (
	"strings"
	"testing"

	"gitviewer/internal/domain"
)

func TestParseAndApplyConflictBlocks(t *testing.T) {
	text := "before\n<<<<<<< HEAD\nours one\n=======\ntheirs one\n>>>>>>> feature\nafter\n"
	blocks := parseConflictBlocks(text)
	if len(blocks) != 1 || blocks[0].StartLine != 2 || blocks[0].Ours != "ours one" || blocks[0].Theirs != "theirs one" {
		t.Fatalf("blocks = %+v", blocks)
	}
	a := &App{Conflict: conflictState{Result: text}}
	a.applyConflictChoice("ours")
	if strings.Contains(a.Conflict.Result, "<<<<<<<") || !strings.Contains(a.Conflict.Result, "ours one") ||
		strings.Contains(a.Conflict.Result, "theirs one") {
		t.Fatalf("ours result = %q", a.Conflict.Result)
	}

	a.Conflict.Result = text
	a.applyConflictChoice("theirs")
	if strings.Contains(a.Conflict.Result, "ours one") || !strings.Contains(a.Conflict.Result, "theirs one") {
		t.Fatalf("theirs result = %q", a.Conflict.Result)
	}

	a.Conflict.Result = text
	a.applyConflictChoice("both")
	if !strings.Contains(a.Conflict.Result, "ours one\ntheirs one") {
		t.Fatalf("both result = %q", a.Conflict.Result)
	}
}

func TestSaveConflictResolutionRejectsMarkers(t *testing.T) {
	a := &App{Conflict: conflictState{
		Open: true, Path: "file.txt",
		Result: "<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> branch\n",
	}}
	a.SaveConflictResolution()
	if !strings.Contains(a.Conflict.Error, "still contain markers") {
		t.Fatalf("error = %q", a.Conflict.Error)
	}
}

func TestRevealHistoryCommitSelectsVisibleRow(t *testing.T) {
	oid := "0123456789abcdef0123456789abcdef01234567"
	a := &App{Settings: defaultAppSettings(), HistorySelected: -1, historyListSelected: -1}
	a.GraphRows = a.Graph.Append([]domain.CommitRecord{{OID: oid, Subject: "subject"}})
	a.revealHistoryCommit(oid)
	if a.HistorySelected != 0 || a.historyListSelected != 0 || a.CommitInput != oid {
		t.Fatalf("selected=%d list=%d input=%q", a.HistorySelected, a.historyListSelected, a.CommitInput)
	}
}
