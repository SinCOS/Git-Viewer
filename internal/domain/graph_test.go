package domain

import "testing"

func TestGraphOctopusPaginationAndRoots(t *testing.T) {
	commits := []CommitRecord{{OID: "merge", Parents: []string{"a", "b", "c"}}, {OID: "a", Parents: []string{"root"}}, {OID: "b", Parents: []string{"root"}}, {OID: "c", Parents: []string{"root"}}, {OID: "root"}, {OID: "orphan"}}
	var all, split GraphLayout
	want := all.Append(commits)
	got := split.Append(commits[:2])
	got = append(got, split.Append(commits[2:])...)
	if len(got) != 6 || len(got[0].Edges) != 3 || len(split.Lanes) != 0 {
		t.Fatal(got, split.Lanes)
	}
	for i := range got {
		if got[i].Lane != want[i].Lane || got[i].Width != want[i].Width {
			t.Fatal("page changes layout", i)
		}
	}
	if len(split.Append(commits)) != 0 {
		t.Fatal("duplicate nodes")
	}
}
func BenchmarkGraph10000(b *testing.B) {
	commits := make([]CommitRecord, 10000)
	for i := range commits {
		commits[i].OID = string(rune(i + 1))
		if i+1 < len(commits) {
			commits[i].Parents = []string{string(rune(i + 2))}
		}
	}
	for b.Loop() {
		var layout GraphLayout
		layout.Append(commits)
	}
}
