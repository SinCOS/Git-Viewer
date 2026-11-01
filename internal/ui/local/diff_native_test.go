package local

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui/editor"
	"gitviewer/internal/domain"
)

const multiFilePatch = `diff --git a/a.txt b/a.txt
--- a/a.txt
+++ b/a.txt
@@ -1 +1 @@
-old-a
+new-a
diff --git a/b.txt b/b.txt
--- a/b.txt
+++ b/b.txt
@@ -1 +1 @@
-old-b
+new-b
`

func TestNativeDiffSelectsMatchingFile(t *testing.T) {
	a, _ := newDiffTestApp(t)
	a.applyPreview(domain.DiffPreview{
		Scope: domain.WorktreeDiff,
		Path:  "b.txt",
		Text:  multiFilePatch,
	})
	state := a.nativeDiffState()
	if state == nil {
		t.Fatal("native diff was not built")
	}
	if got := state.New.String(); !strings.Contains(got, "new-b") || strings.Contains(got, "new-a") {
		t.Fatalf("wrong file selected: %q", got)
	}
}

func TestNativeDiffUsesCompleteCommitFiles(t *testing.T) {
	a, tt := newDiffTestApp(t)
	oldText := "one\ntwo\nthree\nfour\nfive\n"
	newText := "one\nTWO\nthree\nfour\nfive\n"
	a.applyPreview(domain.DiffPreview{
		Scope:    domain.CommitDiff,
		Path:     "a.txt",
		Text:     "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1,5 +1,5 @@\n one\n-two\n+TWO\n three\n four\n five\n",
		OldText:  oldText,
		NewText:  newText,
		FullText: true,
	})
	state := a.nativeDiffState()
	if state == nil {
		t.Fatal("native diff was not built")
	}
	if state.Old.String() != oldText || state.New.String() != newText {
		t.Fatalf("diff did not use complete files: old=%q new=%q", state.Old.String(), state.New.String())
	}
	if !state.ExpandUnchanged {
		t.Fatal("complete files were not fully expanded")
	}
	tt.Frame()
}

func TestNativeDiffFallbacks(t *testing.T) {
	valid := domain.DiffPreview{
		Scope: domain.WorktreeDiff,
		Path:  "a.txt",
		Text:  "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-old\n+new\n",
	}
	tests := []struct {
		name    string
		preview domain.DiffPreview
	}{
		{"limited", domain.DiffPreview{Scope: domain.WorktreeDiff, Limited: true, Text: valid.Text}},
		{"binary", domain.DiffPreview{Scope: domain.WorktreeDiff, Binary: true, Text: valid.Text}},
		{"empty", domain.DiffPreview{Scope: domain.WorktreeDiff}},
		{"malformed", domain.DiffPreview{Scope: domain.WorktreeDiff, Text: "not a patch"}},
		{"multi-file-without-path", domain.DiffPreview{Scope: domain.WorktreeDiff, Text: multiFilePatch}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := newDiffTestApp(t)
			a.applyPreview(tc.preview)
			if state := a.nativeDiffState(); state != nil {
				t.Fatalf("native diff was used for %s", tc.name)
			}
		})
	}
}

func TestNativeDiffWhitespaceFallsBackAndStateIsReused(t *testing.T) {
	a, _ := newDiffTestApp(t)
	a.applyPreview(domain.DiffPreview{
		Scope: domain.WorktreeDiff,
		Path:  "a.txt",
		Text:  "diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-old\n+new\n",
	})
	first := a.nativeDiffState()
	if first == nil {
		t.Fatal("native diff was not built")
	}
	a.findState().sideBySide = true
	second := a.nativeDiffState()
	if second != first || second.Layout != editor.DiffSplit {
		t.Fatal("native diff state was rebuilt or layout was not applied")
	}
	a.findState().showWhitespace = true
	if state := a.nativeDiffState(); state != nil {
		t.Fatal("native diff did not fall back for whitespace mode")
	}
	a.findState().showWhitespace = false
	if state := a.nativeDiffState(); state != first {
		t.Fatal("native diff state was not retained after fallback")
	}
}
