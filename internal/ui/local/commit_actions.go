package local

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

func (a *App) runCommitAction(op domain.Operation, commit, ref string) {
	if a.syncDisabled() || commit == "" {
		return
	}
	a.confirmWrite(domain.GitOperation{
		RepositoryID: a.Repository.Repository.RepositoryID,
		Operation:    op,
		Ref:          ref,
		Commit:       commit,
	}, false)
}

func (a *App) openCommitBranchPrompt(commit, source string) {
	if commit == "" {
		return
	}
	suggestion := "branch-" + commit[:min(8, len(commit))]
	if source != "" {
		name := strings.TrimPrefix(strings.TrimPrefix(source, "refs/heads/"), "refs/remotes/")
		name = strings.ReplaceAll(name, "/", "-")
		if name != "" {
			suggestion = name + "-copy"
		}
	}
	a.openFilePrompt("commit-branch", commit, suggestion, "Create Branch",
		"Create a local branch at "+commit[:min(12, len(commit))]+" and switch to it.")
}

func (a *App) revealHistoryCommit(oid string) {
	if oid == "" {
		return
	}
	index := -1
	for i, row := range a.GraphRows {
		if row.Commit.OID == oid {
			index = i
			break
		}
	}
	if index < 0 {
		a.pendingRevealOID = oid
		return
	}
	a.pendingRevealOID = ""
	a.HistorySelected = index
	a.historyListSelected = -1
	for i, source := range a.filteredHistory() {
		if source == index {
			a.historyListSelected = i
			a.HistoryList.ScrollTo(i, ui.Center)
			break
		}
	}
	a.HistoryDetailsOpen = true
	a.CommitInput = oid
	a.CommitDiff()
}

func (a *App) selectBranchAndReveal(branchName, oid string) {
	if a.HistoryRef == branchName && len(a.GraphRows) > 0 {
		a.revealHistoryCommit(oid)
		return
	}
	a.pendingRevealOID = oid
	a.selectHistoryRef(branchName)
}
