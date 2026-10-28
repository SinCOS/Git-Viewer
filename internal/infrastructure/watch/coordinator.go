package watch

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"gitviewer/internal/domain"
)

type Policy struct{ Foreground, Background, Fallback, Debounce, Maximum time.Duration }

func DefaultPolicy() Policy {
	return Policy{15 * time.Second, 60 * time.Second, 5 * time.Second, 500 * time.Millisecond, 5 * time.Minute}
}
func (p Policy) Delay(background bool, failures int) time.Duration {
	base := p.Foreground
	if background {
		base = p.Background
	}
	for i := 0; i < failures && base < p.Maximum; i++ {
		base *= 2
	}
	return min(base, p.Maximum)
}

type eventKind int

const (
	eventGit eventKind = iota
	eventWorktree
	eventForced
)

// repoSignature is a cheap fingerprint of local repository state: HEAD file
// content (branch or detached commit), index stat, packed-refs stat and the
// loose refs/heads listing. Best-effort: any read error is returned and the
// caller fails open. Remote repositories never use it.
func repoSignature(repo domain.RepositoryContext) (string, error) {
	gitDir := repo.GitDir
	if gitDir == "" {
		gitDir = filepath.Join(repo.Repository.Path, ".git")
	}
	common := repo.CommonDir
	if common == "" {
		common = gitDir
	}
	head, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.Write(head)
	for _, path := range []string{filepath.Join(gitDir, "index"), filepath.Join(common, "packed-refs")} {
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		fmt.Fprintf(&b, "%s %d %d\n", filepath.Base(path), info.Size(), info.ModTime().UnixNano())
	}
	refs, err := looseRefs(filepath.Join(common, "refs", "heads"))
	if err != nil {
		return "", err
	}
	b.WriteString(refs)
	return b.String(), nil
}

func looseRefs(root string) (string, error) {
	var b strings.Builder
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return filepath.SkipAll
			}
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s %d %d\n", rel, info.Size(), info.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		return "", err
	}
	return b.String(), nil
}

// Run owns all timers and watches. notify only requests a refresh; it never
// starts overlapping Git work. Results drive remote exponential backoff and
// capture the local precheck signature after each successful refresh.
func Run(ctx context.Context, repo domain.RepositoryContext, p Policy, background <-chan bool, results <-chan error, notify func()) {
	local := repo.Repository.Location == "local"
	events := make(chan eventKind, 1)
	signal := func(kind eventKind) {
		if kind != eventGit {
			// Worktree and forced events must notify, so they win the slot.
			select {
			case <-events:
			default:
			}
		}
		select {
		case events <- kind:
		default:
		}
	}
	var lastSig string
	haveSig := false
	// skip reports whether the cheap signature proves nothing changed. It is
	// best-effort and fails open on read errors and before the first
	// successful refresh. Worktree events are never skipped: editing a
	// tracked file changes neither HEAD, the index, nor refs.
	skip := func(kind eventKind) bool {
		if !local || kind != eventGit || !haveSig {
			return false
		}
		sig, err := repoSignature(repo)
		return err == nil && sig == lastSig
	}
	capture := func() {
		if !local {
			return
		}
		if sig, err := repoSignature(repo); err == nil {
			lastSig, haveSig = sig, true
		}
	}
	var wg sync.WaitGroup
	if local {
		seen := map[string]bool{}
		for _, dir := range []string{repo.Repository.Path, repo.GitDir, repo.CommonDir} {
			if dir == "" || seen[dir] {
				continue
			}
			seen[dir] = true
			root := dir == repo.Repository.Path
			wg.Add(1)
			go func() {
				defer wg.Done()
				w, err := mygo.WatchFiles(ctx, dir, mygo.FileWatchOptions{Recursive: true, HintsOnly: true, Debounce: p.Debounce, Ignore: func(path string, isDir bool) bool {
					return isDir && (root && path == ".git" || !root && path == "objects")
				}})
				if err != nil {
					signal(eventForced)
					return
				}
				defer w.Close()
				if root {
					signal(eventWorktree)
				} else {
					signal(eventGit)
				}
				for {
					_, err := w.Next(ctx)
					if err != nil {
						if ctx.Err() == nil {
							signal(eventForced)
						}
						return
					}
					if root {
						signal(eventWorktree)
					} else {
						signal(eventGit)
					}
				}
			}()
		}
	}
	defer wg.Wait()
	bg := false
	failures := 0
	interval := func() time.Duration {
		if repo.Repository.Location == "local" {
			return p.Fallback
		}
		return p.Delay(bg, failures)
	}
	timer := time.NewTimer(interval())
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case kind := <-events:
			if !skip(kind) {
				notify()
			}
		case <-timer.C:
			// Local fallback polls must also cover worktree edits: the cheap
			// Git signature only tracks HEAD/index/refs, so applying it here
			// would hide exactly the changes a missed file event represents.
			notify()
			timer.Reset(interval())
		case value, ok := <-background:
			if !ok {
				background = nil
				continue
			}
			bg = value
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(interval())
		case err, ok := <-results:
			if !ok {
				results = nil
				continue
			}
			if err == nil {
				failures = 0
				capture()
			} else {
				failures = min(10, failures+1)
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(interval())
		}
	}
}
