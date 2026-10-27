package git_test

import (
	"context"
	"strings"
	"testing"

	"gitviewer/internal/testgit"
)

func TestTagListAndStashList(t *testing.T) {
	r, repo := configured(t)
	ctx := context.Background()
	dir := repo.Repository.Path
	testgit.Run(t, dir, "commit", "--allow-empty", "-m", "base")
	base := strings.TrimSpace(string(testgit.Run(t, dir, "rev-parse", "HEAD")))
	testgit.Run(t, dir, "tag", "v1.0")
	testgit.Run(t, dir, "tag", "annotated", "-m", "release")
	testgit.Run(t, dir, "commit", "--allow-empty", "-m", "wip")
	testgit.Write(t, dir+"/stashed.txt", "work\n")
	testgit.Run(t, dir, "stash", "push", "--include-untracked", "--message", "staged work")

	tags, err := r.TagList(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 || tags[0].Name != "refs/tags/annotated" || tags[1].Name != "refs/tags/v1.0" || !strings.HasPrefix(tags[1].OID, base[:8]) {
		t.Fatalf("tags: %+v", tags)
	}
	stashes, err := r.StashList(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(stashes) != 1 || stashes[0].Ref != "stash@{0}" || stashes[0].Subject != "On main: staged work" {
		t.Fatalf("stashes: %+v", stashes)
	}
	// An empty stash and tag list parse to empty, not nil-parsing failures.
	testgit.Run(t, dir, "stash", "clear")
	testgit.Run(t, dir, "tag", "-d", "v1.0")
	testgit.Run(t, dir, "tag", "-d", "annotated")
	if tags, err = r.TagList(ctx, repo); err != nil || len(tags) != 0 {
		t.Fatalf("cleared tags: %+v %v", tags, err)
	}
	if stashes, err = r.StashList(ctx, repo); err != nil || len(stashes) != 0 {
		t.Fatalf("cleared stashes: %+v %v", stashes, err)
	}
}
