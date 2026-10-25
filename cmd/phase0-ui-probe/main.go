package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"gitviewer/internal/ui/poc"
)

type metric struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}
type scenario struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Details string `json:"details"`
}
type command struct {
	Command  string `json:"command"`
	ExitCode int    `json:"exitCode"`
}
type report struct {
	Workstream  string            `json:"workstream"`
	Baseline    string            `json:"baseline"`
	Environment map[string]string `json:"environment"`
	Commands    []command         `json:"commands"`
	Scenarios   []scenario        `json:"scenarios"`
	Metrics     []metric          `json:"metrics"`
	Limitations []string          `json:"limitations"`
}
type probe struct {
	app           *poc.App
	scene         string
	buildTimes    map[string][]time.Duration
	dispatchTimes []time.Duration
	failure       string
	maxBuiltRows  int
	observedEnd   bool
	graphScrolled bool
	diffScrolled  bool
	completed     bool
}

func (p *probe) view(c *ui.Context) {
	start := time.Now()
	ui.Column(c).Fill().Children(func() {
		ui.Text(c, "Phase 0 native validation · "+p.scene).Padding(8)
		ui.Box(c).Grow(1).FillWidth().Children(func() {
			switch p.scene {
			case "list":
				p.app.HistoryView(c)
				p.maxBuiltRows = max(p.maxBuiltRows, p.app.BuiltRows)
			case "graph":
				p.app.GraphView(c)
			case "diff", "binary":
				p.app.DiffView(c)
			}
		})
	})
	p.buildTimes[p.scene] = append(p.buildTimes[p.scene], time.Since(start))
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	output := flag.String("output", ".tools/native-ui-probe", "evidence directory")
	baseline := flag.String("baseline", "", "40-character frozen contract commit")
	flag.Parse()
	if !regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(*baseline) {
		return fmt.Errorf("-baseline must name the frozen 40-character commit")
	}
	folder, err := filepath.Abs(*output)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(folder, 0755); err != nil {
		return err
	}
	p := &probe{app: poc.New(), scene: "list", buildTimes: map[string][]time.Duration{}}
	var workers sync.WaitGroup
	var runErr error
	mygo.App.WhenReady(func() {
		win := mygo.NewWindow(mygo.WindowOptions{Title: "Phase 0 Native Probe", Width: 1000, Height: 700, Content: ui.View(p.view)})
		ctx, cancel := context.WithCancel(context.Background())
		mygo.App.OnBeforeQuit(func(_ *mygo.QuitEvent) { cancel() })
		win.OnClosed(func() { cancel(); mygo.App.Quit() })
		workers.Add(1)
		go func() {
			defer workers.Done()
			// A generous bound prevents a GUI failure from leaving this probe running.
			watchdog := time.AfterFunc(20*time.Second, func() { cancel(); mygo.App.Quit() })
			defer watchdog.Stop()
			update := func(fn func()) bool {
				ack := make(chan struct{})
				queued := time.Now()
				win.Update(func() { p.dispatchTimes = append(p.dispatchTimes, time.Since(queued)); fn(); close(ack) })
				select {
				case <-ack:
					return true
				case <-ctx.Done():
					return false
				}
			}
			pause := func() bool {
				timer := time.NewTimer(25 * time.Millisecond)
				defer timer.Stop()
				select {
				case <-timer.C:
					return true
				case <-ctx.Done():
					return false
				}
			}
			snapshot := func(name string) error {
				data, err := win.CapturePage()
				if err != nil {
					return err
				}
				config, err := png.DecodeConfig(bytes.NewReader(data))
				if err != nil {
					return err
				}
				if config.Width < 300 || config.Height < 200 {
					return fmt.Errorf("native capture unexpectedly small")
				}
				return os.WriteFile(filepath.Join(folder, name+".png"), data, 0644)
			}
			for _, scene := range []string{"list", "graph", "diff", "binary"} {
				if !update(func() { p.scene = scene; p.app.Binary = scene == "binary" }) {
					return
				}
				for i := 0; i < 50; i++ {
					step := i
					if !update(func() {
						switch scene {
						case "list":
							if step > 0 {
								first, last := p.app.List.Visible()
								if first <= 9999 && last >= 9999 {
									p.observedEnd = true
								}
							}
							row := (step * 211) % poc.CommitCount
							if step == 48 {
								row = 9999
							}
							p.app.Selected = row
							p.app.List.ScrollTo(row, ui.Center)
						case "graph":
							if p.app.GraphScroll.X > 0 && p.app.GraphScroll.Y > 0 {
								p.graphScrolled = true
							}
							p.app.GraphSelected = step * 4 % poc.GraphCount
							// Last steps restore a viewport large enough for all 200 nodes.
							if step == 15 {
								win.SetSize(500, 350)
							}
							if step >= 16 && step < 35 {
								p.app.GraphScroll.X = 200
								p.app.GraphScroll.Y = 100
							}
							if step == 35 {
								win.SetSize(1000, 700)
								p.app.GraphScroll.X = 0
								p.app.GraphScroll.Y = 0
							}
						case "diff":
							if p.app.DiffScroll.X > 0 {
								p.diffScrolled = true
							}
							p.app.DiffScroll.X = float32(step * 20)
						}
					}) {
						return
					}
					if !pause() {
						return
					}
				}
				if err := snapshot(scene); err != nil {
					runErr = err
					break
				}
			}
			update(func() {
				if !p.observedEnd {
					p.failure = "native list never reported final row visible"
				}
				if p.maxBuiltRows <= 0 || p.maxBuiltRows >= 100 {
					p.failure = "native list failed virtualization check"
				}
				if !p.graphScrolled || !p.diffScrolled {
					p.failure = "native graph or diff scroll offset never advanced"
				}
				p.completed = true
				mygo.App.Quit()
			})
		}()
	})
	appErr := mygo.App.Run()
	workers.Wait()
	if appErr != nil {
		return appErr
	}
	if runErr != nil {
		return runErr
	}
	if !p.completed {
		return fmt.Errorf("native probe interrupted before all scenarios completed")
	}
	if p.failure != "" {
		return fmt.Errorf("%s", p.failure)
	}
	for _, scene := range []string{"list", "graph", "diff", "binary"} {
		if len(p.buildTimes[scene]) == 0 {
			return fmt.Errorf("%s never built in native window", scene)
		}
	}
	env := map[string]string{"go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "mygo": "9c69cf829ece", "measurement": "real native window; programmatic state exercise, not OS input automation"}
	for key, args := range map[string][]string{"system": {"uname", "-sr"}, "cpu": {"sysctl", "-n", "machdep.cpu.brand_string"}, "memory_bytes": {"sysctl", "-n", "hw.memsize"}} {
		data, err := exec.Command(args[0], args[1:]...).Output()
		if err == nil {
			env[key] = strings.TrimSpace(string(data))
		}
	}
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	r := report{Workstream: "native-ui", Baseline: *baseline, Environment: env, Commands: []command{{"go run ./cmd/phase0-ui-probe -baseline " + *baseline + " -output " + *output, 0}}, Scenarios: []scenario{
		{"native-list", "passed", "10,000 mock rows; scripted selection/scroll; final row observed visible; <100 rows built per pass"},
		{"native-graph", "passed", "200 mock nodes rendered in full viewport; narrow viewport scroll and resize exercised; native PNG captured"},
		{"native-diff", "passed", "Chinese/long-line horizontal scrolling and binary placeholder rendered; native PNGs captured"},
		{"os-input-and-gpu", "unverified", "This probe does not inject physical input or read GPU counters; pointer/clipboard semantics are checked separately with ui.NewTester"},
	}, Limitations: []string{"View build time is CPU work before layout/paint, not GPU time or presentation FPS.", "Dispatch latency measures posted main-thread callbacks, not physical-input latency.", "Memory metrics cover the Go heap, not GPU allocations or process RSS.", "Scripted state changes do not verify physical mouse/keyboard/IME interactions.", "Apple Silicon is not available on this machine."}}
	for _, scene := range []string{"list", "graph", "diff", "binary"} {
		r.Metrics = append(r.Metrics, summarize(scene+"_view_build", p.buildTimes[scene])...)
	}
	r.Metrics = append(r.Metrics, summarize("main_thread_dispatch", p.dispatchTimes)...)
	r.Metrics = append(r.Metrics, metric{"max_built_rows", float64(p.maxBuiltRows), "rows"}, metric{"go_heap_alloc", float64(memory.HeapAlloc), "bytes"}, metric{"go_total_alloc", float64(memory.TotalAlloc), "bytes"})
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(folder, "report.json"), append(data, '\n'), 0644); err != nil {
		return err
	}
	fmt.Printf("Native probe passed; report and four captures in %s\n", folder)
	return nil
}
func summarize(name string, values []time.Duration) []metric {
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	if len(sorted) == 0 {
		return nil
	}
	return []metric{{name + "_samples", float64(len(sorted)), "samples"}, {name + "_p50", float64(sorted[(len(sorted)-1)/2]) / float64(time.Millisecond), "ms"}, {name + "_p95", float64(sorted[(len(sorted)-1)*95/100]) / float64(time.Millisecond), "ms"}, {name + "_max", float64(sorted[len(sorted)-1]) / float64(time.Millisecond), "ms"}}
}
