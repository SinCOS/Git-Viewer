package local

import (
	"strings"
	"testing"

	"gitviewer/internal/domain"
)

func TestDiffRowsUnifiedAndSideBySide(t *testing.T) {
	lines := strings.Split("diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -4,3 +4,4 @@ heading\n same\n-old one\n-old two\n+new one\n+new two\n+new three\n\\ No newline at end of file\n@@ -20 +21 @@\n-last\n+next\n", "\n")
	unified, mapping := diffRows(lines, false)
	if len(unified) != len(lines) || mapping[8] != 8 {
		t.Fatalf("unified rows: %d, mapping %v", len(unified), mapping)
	}
	if unified[4].left.line != 4 || unified[4].right.line != 4 || unified[5].left.line != 5 || unified[7].right.line != 5 {
		t.Fatalf("incorrect line numbers: %+v %+v %+v", unified[4], unified[5], unified[7])
	}
	side, mapping := diffRows(lines, true)
	if mapping[5] != mapping[7] || mapping[6] != mapping[8] || mapping[9] == mapping[8] {
		t.Fatalf("incorrect pairing: %v", mapping)
	}
	row := side[mapping[5]]
	if row.left.text != "-old one" || row.right.text != "+new one" || row.left.line != 5 || row.right.line != 5 {
		t.Fatalf("paired row: %+v", row)
	}
	if side[mapping[9]].left.source != -1 || side[mapping[9]].right.line != 7 {
		t.Fatalf("unpaired addition: %+v", side[mapping[9]])
	}
	if side[mapping[10]].left.text != lines[10] || side[mapping[12]].left.line != 20 || side[mapping[13]].right.line != 21 {
		t.Fatalf("metadata or second hunk lost: %+v", side)
	}
}

func TestDiffRowsZeroCountsAndMetadata(t *testing.T) {
	lines := strings.Split("diff --git a/f b/f\n@@ -0,0 +1,2 @@\n+++actual content\n+other\n--- metadata\n", "\n")
	rows, mapping := diffRows(lines, true)
	if rows[mapping[2]].right.text != "+++actual content" || rows[mapping[2]].right.line != 1 {
		t.Fatalf("addition misclassified: %+v", rows[mapping[2]])
	}
	if !rows[mapping[4]].shared || rows[mapping[4]].left.text != "--- metadata" {
		t.Fatalf("metadata misclassified: %+v", rows[mapping[4]])
	}
}

func TestSingleFileRowsHideHeaderButKeepSourceMapping(t *testing.T) {
	lines := strings.Split("diff --git a/f b/f\nindex 1..2\n--- a/f\n+++ b/f\n@@ -1 +1 @@\n-old\n+new", "\n")
	rows, mapping := diffRows(lines, false)
	rows, mapping = singleFileRows(rows, mapping)
	if len(rows) != 3 || rows[0].left.text != lines[4] || mapping[5] != 1 || mapping[6] != 2 {
		t.Fatalf("rows=%+v mapping=%v", rows, mapping)
	}
	metadata, indices := diffRows([]string{"diff --git a/f b/f", "Binary files differ"}, false)
	kept, _ := singleFileRows(metadata, indices)
	if len(kept) != 2 {
		t.Fatal("metadata-only diff must remain visible")
	}
	rows, mapping = diffRows([]string{"diff --git a/f b/g", "rename from f", "rename to g", "@@ -1 +1 @@", "-old", "+new"}, false)
	rows, mapping = singleFileRows(rows, mapping)
	if rows[0].left.text != "rename from f" || rows[1].left.text != "rename to g" || mapping[4] != 3 {
		t.Fatalf("rename metadata lost: rows=%+v mapping=%v", rows, mapping)
	}
}

func TestDiffSideBySideControlsAndCopy(t *testing.T) {
	a, tt := newDiffTestApp(t)
	text := "diff --git a/f b/f\n@@ -1,2 +1,2 @@\n-old\n+new\n same\n"
	a.applyPreview(domain.DiffPreview{Scope: domain.WorktreeDiff, Path: "f", Text: text})
	tt.Frame()
	if err := tt.Click("Side-by-Side"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.findState().sideBySide || a.findState().indices[2] != a.findState().indices[3] {
		t.Fatal("side-by-side view did not pair changes")
	}
	if err := tt.Click("Diff settings"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("Copy diff"); err != nil {
		t.Fatal(err)
	}
	if tt.Clipboard() != text {
		t.Fatal("copy changed the original diff")
	}
	if err := tt.Click("Unified"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.findState().sideBySide || a.findState().indices[2] == a.findState().indices[3] {
		t.Fatal("unified view did not restore separate rows")
	}
}
