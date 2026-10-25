package watch

import (
	"context"
	"github.com/egoist/mygo"
	"gitviewer/internal/domain"
	"sync"
	"time"
)

type Policy struct{ Foreground, Background, Fallback, Debounce, Maximum time.Duration }

func DefaultPolicy() Policy {
	return Policy{15 * time.Second, 60 * time.Second, 60 * time.Second, 500 * time.Millisecond, 5 * time.Minute}
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

// Run owns all timers and watches. notify only requests a refresh; it never
// starts overlapping Git work. Results drive remote exponential backoff.
func Run(ctx context.Context, repo domain.RepositoryContext, p Policy, background <-chan bool, results <-chan error, notify func()) {
	events := make(chan struct{}, 1)
	signal := func() {
		select {
		case events <- struct{}{}:
		default:
		}
	}
	var wg sync.WaitGroup
	if repo.Repository.Location == "local" {
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
					signal()
					return
				}
				defer w.Close()
				signal()
				for {
					_, err := w.Next(ctx)
					if err != nil {
						if ctx.Err() == nil {
							signal()
						}
						return
					}
					signal()
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
		case <-events:
			notify()
		case <-timer.C:
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
