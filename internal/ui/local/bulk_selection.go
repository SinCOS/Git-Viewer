package local

import "gitviewer/internal/domain"

// runSelectedChanges turns checked rows into one confirmed operation; each
// path is still rechecked by the existing Git writer before any write.
func (a *App) runSelectedChanges(op domain.Operation) {
	if a.syncDisabled() || (op != domain.Stage && op != domain.Unstage && op != domain.Discard) {
		return
	}
	selected := a.selectedDiffEntries()
	request := domain.GitOperation{RepositoryID: a.Repository.Repository.RepositoryID, Operation: op}
	seen := make(map[string]bool)
	for _, entry := range selected {
		if op == domain.Stage && entry.Scope == domain.StagedDiff || op == domain.Unstage && entry.Scope != domain.StagedDiff {
			continue
		}
		if op == domain.Discard {
			change := entry.Change
			if entry.Scope != domain.WorktreeDiff || change.Untracked || change.Conflicted || change.OldPath != nil ||
				(change.IndexStatus != " " && change.IndexStatus != "M" && change.IndexStatus != "A") ||
				(change.WorktreeStatus != "M" && change.WorktreeStatus != "D") {
				a.Message = "Discard selected requires only safely discardable unstaged files"
				return
			}
		}
		if !seen[entry.Change.Path] {
			seen[entry.Change.Path] = true
			request.Paths = append(request.Paths, entry.Change.Path)
		}
	}
	if len(request.Paths) == 0 {
		a.Message = "No selected files are eligible for this operation"
		return
	}
	a.confirmWrite(request, false)
}
