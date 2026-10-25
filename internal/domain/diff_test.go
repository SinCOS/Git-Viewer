package domain

import "testing"

func TestStatusPartitions(t *testing.T) {
	status := GitStatus{Changes: []Change{
		{Path: "both", IndexStatus: "M", WorktreeStatus: "M"},
		{Path: "new", IndexStatus: "?", WorktreeStatus: "?", Untracked: true},
		{Path: "conflict", IndexStatus: "U", WorktreeStatus: "U", Conflicted: true},
	}}
	entries := StatusEntries(status)
	if len(entries) != 4 || entries[0].Group != "Staged" || entries[1].Group != "Unstaged" || entries[2].Group != "Untracked" || entries[3].Group != "Conflicted" {
		t.Fatalf("%+v", entries)
	}
}
