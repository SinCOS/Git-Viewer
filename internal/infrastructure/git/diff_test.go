package git_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitviewer/internal/domain"
	"gitviewer/internal/infrastructure/git"
	"gitviewer/internal/testgit"
)

func TestDiffReadOnly(t *testing.T) {
	ctx := context.Background()
	dir := testgit.Repo(t)
	r := git.LocalReader{}
	repo, err := r.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	name := ":(glob)中文 space\n'*.txt"
	testgit.Write(t, filepath.Join(dir, name), "base\n")
	testgit.Run(t, dir, "--literal-pathspecs", "add", "--", name)
	// Staged diff must work before the initial commit.
	p, err := r.Diff(ctx, repo, domain.DiffRequest{Scope: domain.StagedDiff, Path: name})
	if err != nil || !strings.Contains(p.Text, "+base") {
		t.Fatalf("%+v %v", p, err)
	}
	testgit.Run(t, dir, "commit", "-m", "initial")
	oid := strings.TrimSpace(string(testgit.Run(t, dir, "rev-parse", "HEAD")))
	p, err = r.Diff(ctx, repo, domain.DiffRequest{Scope: domain.CommitDiff, Commit: oid})
	if err != nil || !strings.Contains(p.Text, "+base") {
		t.Fatalf("root commit: %+v %v", p, err)
	}
	blob := strings.TrimSpace(string(testgit.Run(t, dir, "rev-parse", "HEAD:"+name)))
	if _, err := r.Diff(ctx, repo, domain.DiffRequest{Scope: domain.CommitDiff, Commit: blob}); err == nil {
		t.Fatal("blob accepted as commit")
	}
	testgit.Write(t, filepath.Join(dir, name), "staged\n")
	testgit.Run(t, dir, "--literal-pathspecs", "add", "--", name)
	testgit.Write(t, filepath.Join(dir, name), "unstaged\n")
	before := testgit.Run(t, dir, "status", "--porcelain=v1", "-z")
	for scope, want := range map[domain.DiffScope]string{domain.StagedDiff: "+staged", domain.WorktreeDiff: "+unstaged"} {
		p, err = r.Diff(ctx, repo, domain.DiffRequest{Scope: scope, Path: name})
		if err != nil || !strings.Contains(p.Text, want) {
			t.Fatalf("%s: %+v %v", scope, p, err)
		}
	}
	if string(testgit.Run(t, dir, "status", "--porcelain=v1", "-z")) != string(before) {
		t.Fatal("read altered index/status")
	}
	// External diff and textconv must never run, even when repository config asks.
	testgit.Run(t, dir, "config", "diff.external", "/definitely-missing-external-diff")
	p, err = r.Diff(ctx, repo, domain.DiffRequest{Scope: domain.WorktreeDiff, Path: name})
	if err != nil || !strings.Contains(p.Text, "+unstaged") {
		t.Fatalf("external diff enabled: %+v %v", p, err)
	}
	for _, request := range []domain.DiffRequest{
		{Scope: domain.WorktreeDiff, Path: "../secret"},
		{Scope: domain.StagedDiff, Path: "/tmp/secret"},
		{Scope: domain.WorktreeDiff, Path: "file\x00"},
		{Scope: domain.CommitDiff, Commit: "--output=/tmp/x"},
		{Scope: domain.WorktreeDiff, Path: name, Commit: "HEAD"},
		{Scope: "shell"},
	} {
		if _, err := r.Diff(ctx, repo, request); err == nil {
			t.Fatalf("accepted %+v", request)
		}
	}
}

func TestCommitDiffIncludesCompleteFiles(t *testing.T) {
	ctx := context.Background()
	dir := testgit.Repo(t)
	r := git.LocalReader{}
	repo, err := r.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	base := "one\ntwo\nthree\nfour\nfive\n"
	testgit.Write(t, filepath.Join(dir, "file"), base)
	testgit.Run(t, dir, "add", "file")
	testgit.Run(t, dir, "commit", "-m", "base")
	root := strings.TrimSpace(string(testgit.Run(t, dir, "rev-parse", "HEAD")))
	changed := "one\nTWO\nthree\nfour\nfive\n"
	testgit.Write(t, filepath.Join(dir, "file"), changed)
	testgit.Run(t, dir, "add", "file")
	testgit.Run(t, dir, "commit", "-m", "change")
	oid := strings.TrimSpace(string(testgit.Run(t, dir, "rev-parse", "HEAD")))

	preview, err := r.Diff(ctx, repo, domain.DiffRequest{
		Scope: domain.CommitDiff, Commit: oid, Path: "file", OldPath: "file", NewPath: "file",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.FullText || preview.OldText != base || preview.NewText != changed {
		t.Fatalf("complete files missing: %+v", preview)
	}
	if !strings.Contains(preview.Text, "-two") || !strings.Contains(preview.Text, "+TWO") {
		t.Fatalf("patch text changed: %q", preview.Text)
	}

	preview, err = r.Diff(ctx, repo, domain.DiffRequest{
		Scope: domain.CommitDiff, Commit: root, Path: "file", NewPath: "file",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.FullText || preview.OldText != "" || preview.NewText != base {
		t.Fatalf("root commit files missing: %+v", preview)
	}
}

func TestWorkingDiffIncludesCompleteFiles(t *testing.T) {
	ctx := context.Background()
	dir := testgit.Repo(t)
	r := git.LocalReader{}
	repo, err := r.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	base := "one\ntwo\nthree\nfour\nfive\n"
	testgit.Write(t, filepath.Join(dir, "file"), base)
	testgit.Run(t, dir, "add", "file")
	testgit.Run(t, dir, "commit", "-m", "base")
	changed := "one\nTWO\nthree\nfour\nfive\n"
	testgit.Write(t, filepath.Join(dir, "file"), changed)

	preview, err := r.Diff(ctx, repo, domain.DiffRequest{Scope: domain.WorktreeDiff, Path: "file"})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.FullText || preview.OldText != base || preview.NewText != changed {
		t.Fatalf("worktree complete files missing: %+v", preview)
	}

	testgit.Run(t, dir, "add", "file")
	preview, err = r.Diff(ctx, repo, domain.DiffRequest{Scope: domain.StagedDiff, Path: "file"})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.FullText || preview.OldText != base || preview.NewText != changed {
		t.Fatalf("staged complete files missing: %+v", preview)
	}

	testgit.Write(t, filepath.Join(dir, "new.txt"), "untracked\nfile\n")
	preview, err = r.Diff(ctx, repo, domain.DiffRequest{Scope: domain.WorktreeDiff, Path: "new.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.FullText || preview.OldText != "" || preview.NewText != "untracked\nfile\n" {
		t.Fatalf("untracked complete file missing: %+v", preview)
	}
}

func TestDiffFallbackAndRename(t *testing.T) {
	ctx := context.Background()
	dir := testgit.Repo(t)
	r := git.LocalReader{}
	repo, err := r.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"binary": "a\x00b", "large": strings.Repeat("x", git.PreviewLimit+1), "text": "中文\nBinary files are text too differ\n", "many": strings.Repeat("x\n", git.PreviewLineLimit+1)} {
		testgit.Write(t, filepath.Join(dir, name), content)
		p, err := r.Diff(ctx, repo, domain.DiffRequest{Scope: domain.WorktreeDiff, Path: name})
		if err != nil {
			t.Fatal(err)
		}
		if name == "text" && p.Binary {
			t.Fatal("text mistaken for binary")
		}
		if name == "binary" && !p.Binary || (name == "large" || name == "many") && !p.Limited || name == "text" && !strings.Contains(p.Text, "+中文") {
			t.Fatalf("%s: %+v", name, p)
		}
	}
	outside := filepath.Join(t.TempDir(), "secret")
	testgit.Write(t, outside, "PRIVATE")
	if err := os.Symlink(outside, filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	p, err := r.Diff(ctx, repo, domain.DiffRequest{Scope: domain.WorktreeDiff, Path: "link"})
	if err != nil || strings.Contains(p.Text, "PRIVATE") || !strings.Contains(p.Text, outside) {
		t.Fatalf("symlink: %+v %v", p, err)
	}
	testgit.Run(t, dir, "add", "binary", "text")
	testgit.Run(t, dir, "commit", "-m", "base")
	testgit.Write(t, filepath.Join(dir, "binary"), "new\x00binary")
	p, err = r.Diff(ctx, repo, domain.DiffRequest{Scope: domain.WorktreeDiff, Path: "binary"})
	if err != nil || !p.Binary {
		t.Fatalf("tracked binary: %+v %v", p, err)
	}
	testgit.Write(t, filepath.Join(dir, "text"), "Binary files example differ\n")
	textPreview, err := r.Diff(ctx, repo, domain.DiffRequest{Scope: domain.WorktreeDiff, Path: "text"})
	if err != nil || textPreview.Binary {
		t.Fatalf("text mistaken for binary: %+v %v", textPreview, err)
	}
	testgit.Write(t, filepath.Join(dir, "text"), "中文\nBinary files are text too differ\n")
	testgit.Run(t, dir, "mv", "text", "renamed 中文")
	p, err = r.Diff(ctx, repo, domain.DiffRequest{Scope: domain.StagedDiff, Path: "renamed 中文"})
	if err != nil || !strings.Contains(p.Text, "rename from") {
		t.Fatalf("rename: %+v %v", p, err)
	}
	testgit.Write(t, filepath.Join(dir, "renamed 中文"), strings.Repeat("long line\n", 10000))
	p, err = (git.LocalReader{OutputLimit: 100}).Diff(ctx, repo, domain.DiffRequest{Scope: domain.WorktreeDiff, Path: "renamed 中文"})
	if err != nil || !p.Limited || p.Text != "" {
		t.Fatalf("limited: %+v %v", p, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := r.Diff(cancelled, repo, domain.DiffRequest{Scope: domain.WorktreeDiff, Path: "renamed 中文"}); err == nil {
		t.Fatal("cancel ignored")
	}
}
