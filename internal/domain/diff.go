package domain

import "sort"

// DiffRequest is an internal typed read request. Commit accepts a full object ID.
type DiffRequest struct {
	Scope  DiffScope
	Path   string
	Commit string
}
type DiffScope string

const (
	WorktreeDiff DiffScope = "worktree"
	StagedDiff   DiffScope = "staged"
	CommitDiff   DiffScope = "commit"
)

// DiffPreview is bounded display text, never an apply-able patch contract.
type DiffPreview struct {
	RepositoryID string    `json:"repositoryId"`
	Scope        DiffScope `json:"scope"`
	Path         string    `json:"path"`
	Text         string    `json:"text"`
	Binary       bool      `json:"binary"`
	Limited      bool      `json:"limited"`
}

// ConflictFile is the read-only three-way view of one unresolved path. Result
// is the current worktree text when it is safe to display and edit.
type ConflictFile struct {
	RepositoryID    string `json:"repositoryId"`
	Path            string `json:"path"`
	Base            string `json:"base,omitempty"`
	Ours            string `json:"ours,omitempty"`
	Theirs          string `json:"theirs,omitempty"`
	Result          string `json:"result,omitempty"`
	BaseAvailable   bool   `json:"baseAvailable"`
	OursAvailable   bool   `json:"oursAvailable"`
	TheirsAvailable bool   `json:"theirsAvailable"`
	ResultAvailable bool   `json:"resultAvailable"`
	Binary          bool   `json:"binary"`
	Limited         bool   `json:"limited"`
}

type StatusEntry struct {
	Change Change
	Scope  DiffScope
	Group  string
}

func StatusEntries(status GitStatus) []StatusEntry {
	entries := make([]StatusEntry, 0, len(status.Changes)*2)
	for _, c := range status.Changes {
		switch {
		case c.Conflicted:
			entries = append(entries, StatusEntry{c, WorktreeDiff, "Conflicted"})
		case c.Untracked:
			entries = append(entries, StatusEntry{c, WorktreeDiff, "Untracked"})
		default:
			if c.IndexStatus != " " && c.IndexStatus != "!" {
				entries = append(entries, StatusEntry{c, StagedDiff, "Staged"})
			}
			if c.WorktreeStatus != " " && c.WorktreeStatus != "!" {
				entries = append(entries, StatusEntry{c, WorktreeDiff, "Unstaged"})
			}
		}
	}
	order := map[string]int{"Staged": 0, "Unstaged": 1, "Untracked": 2, "Conflicted": 3}
	sort.SliceStable(entries, func(i, j int) bool { return order[entries[i].Group] < order[entries[j].Group] })
	return entries
}
