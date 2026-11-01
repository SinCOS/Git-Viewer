package git

import (
	"context"

	"gitviewer/internal/domain"
)

// RefLists is the branch-tree snapshot read alongside history. Its fixed
// queries may execute concurrently over separate channels of one SSH master.
type RefLists struct {
	Branches []BranchRecord
	Tags     []TagRecord
	Stashes  []StashRecord
}

func (r LocalReader) ListRefs(ctx context.Context, repo domain.RepositoryContext) (RefLists, error) {
	results, err := r.runReadBatch(ctx, repo.Repository.Path, [][]string{
		{"for-each-ref", "--format=%(refname)%00%(objectname)%00%(upstream)%00%(HEAD)%00%(upstream:track)%00%(symref)", "refs/heads/", "refs/remotes/"},
		{"for-each-ref", "--format=%(refname)%00%(objectname)", "refs/tags/"},
		{"stash", "list", "--format=%gd%x00%gs"},
	})
	if err != nil {
		return RefLists{}, err
	}
	branches, err := parseBranches(results[0].Stdout)
	if err != nil {
		return RefLists{}, err
	}
	tags, err := parseTags(results[1].Stdout)
	if err != nil {
		return RefLists{}, err
	}
	stashes, err := parseStashes(results[2].Stdout)
	if err != nil {
		return RefLists{}, err
	}
	return RefLists{Branches: branches, Tags: tags, Stashes: stashes}, nil
}
