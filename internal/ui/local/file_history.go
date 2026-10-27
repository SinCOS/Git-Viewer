package local

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
)

// FileHistoryState belongs to the UI thread and should be held by the owning App.
// Call Release when the view is dismissed or its App is closed.
type FileHistoryState struct {
	generation    uint64
	jobGeneration uint64
	open          bool
	loading       bool
	kind          string
	path          string
	repo          domain.RepositoryContext
	err           string
	history       gitexec.FileHistoryResult
	lines         []string
	list          ui.ListState
}

// Release invalidates pending results and drops retained data.
func (s *FileHistoryState) Release() {
	generation := s.generation + 1
	*s = FileHistoryState{generation: generation}
}

// OpenFileHistory loads the selected diff path without changing the diff.
func (a *App) OpenFileHistory(s *FileHistoryState, path string) {
	a.openFileRead(s, path, "history")
}

// OpenFileBlame loads the HEAD version of the selected diff path.
func (a *App) OpenFileBlame(s *FileHistoryState, path string) {
	a.openFileRead(s, path, "blame")
}

func (a *App) openFileRead(s *FileHistoryState, path, kind string) {
	if s == nil || a.closed || a.Dispatch == nil || a.Writing || a.DialogOpen || path == "" || a.Preview.Path != path || a.Repository.Repository.RepositoryID == "" {
		return
	}
	s.Release()
	s.open, s.loading, s.kind, s.path, s.repo = true, true, kind, path, a.Repository
	generation, repo, service := s.generation, a.Repository, a.Service
	a.start(func(ctx context.Context) (func(), error) {
		var history gitexec.FileHistoryResult
		var text string
		var err error
		if kind == "history" {
			history, err = service.FileHistory(ctx, repo, path)
		} else {
			text, err = service.FileBlame(ctx, repo, path)
		}
		return func() {
			if !s.open || s.generation != generation || a.Repository != repo || a.Preview.Path != path {
				return
			}
			s.loading = false
			if err != nil {
				s.err = message(err)
				return
			}
			s.history = history
			if text != "" {
				s.lines = strings.Split(strings.TrimSuffix(text, "\n"), "\n")
			}
		}, nil
	})
	s.jobGeneration = a.generation
}

// FileHistoryView renders the independently owned history/blame panel. The
// caller can retain the existing diff and mount this panel alongside it.
func (a *App) FileHistoryView(c *ui.Context, s *FileHistoryState) {
	if s == nil || !s.open {
		return
	}
	if a.Repository != s.repo || a.Preview.Path != s.path {
		s.Release()
		return
	}
	if s.loading && s.jobGeneration != a.generation {
		s.loading = false
		s.err = "Operation cancelled"
	}
	ui.Column(c).Fill().Gap(6).Children(func() {
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			label := "File History"
			if s.kind == "blame" {
				label = "Blame · HEAD"
			}
			ui.Text(c, label).Bold()
			ui.Text(c, displayPath(s.path)).Grow(1).SingleLine().Tooltip(pathLabel(s.path))
			ui.Button(c, "Close").OnClick(func() { s.Release() })
		})
		if s.loading {
			ui.Text(c, "Loading…")
			return
		}
		if s.err != "" {
			ui.Text(c, s.err)
			return
		}
		if s.kind == "history" {
			if s.history.More {
				ui.Text(c, "Showing the first 100 commits").TextColor(c.Theme().TextMuted)
			}
			ui.List(c, &s.list, len(s.history.Commits), func(i int) {
				commit := s.history.Commits[i]
				ui.Text(c, commit.OID[:12]+"  "+displayPath(commit.Author)+"  "+commit.AuthoredAt.Format("2006-01-02")+"  "+displayPath(commit.Subject)).SingleLine().Selectable()
			}).Grow(1).FillWidth()
			return
		}
		if len(s.lines) == 0 {
			ui.Text(c, "Empty file")
			return
		}
		ui.List(c, &s.list, len(s.lines), func(i int) {
			ui.Text(c, s.lines[i]).Font("monospace").SingleLine().Selectable()
		}).Grow(1).FillWidth()
	})
}
