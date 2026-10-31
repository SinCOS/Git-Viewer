package local

import (
	"context"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

type conflictState struct {
	Open    bool
	Loading bool
	Path    string
	Data    domain.ConflictFile
	Result  string
	Error   string
	Block   int
}

type conflictBlock struct {
	Start, End   int
	Ours, Theirs string
	StartLine    int
}

func (a *App) OpenConflict(path string) {
	if path == "" || a.Repository.Repository.RepositoryID == "" {
		return
	}
	service, repo := a.Service, a.Repository
	a.Conflict = conflictState{Open: true, Loading: true, Path: path}
	a.start(func(ctx context.Context) (func(), error) {
		conflict, err := service.ConflictFile(ctx, repo, path)
		return func() {
			a.Conflict.Loading = false
			if err != nil {
				a.Conflict.Error = message(err)
				a.Message = message(err)
				return
			}
			a.Conflict.Data = conflict
			a.Conflict.Result = conflict.Result
			if !conflict.ResultAvailable {
				switch {
				case conflict.OursAvailable:
					a.Conflict.Result = conflict.Ours
				case conflict.TheirsAvailable:
					a.Conflict.Result = conflict.Theirs
				case conflict.BaseAvailable:
					a.Conflict.Result = conflict.Base
				}
			}
			if conflict.Binary || conflict.Limited {
				a.Conflict.Error = "Binary or oversized conflicts must be resolved with an external merge tool"
			}
			a.Message = "Resolve conflict · " + path
		}, nil
	})
}

func (a *App) conflictBlocks() []conflictBlock {
	return parseConflictBlocks(a.Conflict.Result)
}

func parseConflictBlocks(text string) []conflictBlock {
	lines := strings.SplitAfter(text, "\n")
	var blocks []conflictBlock
	offset := 0
	start, startLine, separator := -1, 0, -1
	oursStart, theirsStart := -1, -1
	for i, line := range lines {
		plain := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		switch {
		case strings.HasPrefix(plain, "<<<<<<<") && start < 0:
			start, startLine = offset, i+1
			separator, oursStart, theirsStart = -1, offset+len(line), -1
		case start >= 0 && strings.HasPrefix(plain, "=======") && separator < 0:
			separator, theirsStart = offset, offset+len(line)
		case start >= 0 && separator >= 0 && strings.HasPrefix(plain, ">>>>>>>"):
			end := offset + len(line)
			blocks = append(blocks, conflictBlock{
				Start: start, End: end, StartLine: startLine,
				Ours:   strings.TrimSuffix(strings.TrimSuffix(text[oursStart:separator], "\n"), "\r"),
				Theirs: strings.TrimSuffix(strings.TrimSuffix(text[theirsStart:offset], "\n"), "\r"),
			})
			start, separator, oursStart, theirsStart = -1, -1, -1, -1
		}
		offset += len(line)
	}
	return blocks
}

func (a *App) applyConflictChoice(choice string) {
	blocks := a.conflictBlocks()
	if len(blocks) == 0 {
		return
	}
	a.Conflict.Block = min(max(a.Conflict.Block, 0), len(blocks)-1)
	block := blocks[a.Conflict.Block]
	replacement := ""
	switch choice {
	case "ours":
		replacement = block.Ours
	case "theirs":
		replacement = block.Theirs
	case "both":
		replacement = block.Ours
		if replacement != "" && block.Theirs != "" {
			replacement += "\n"
		}
		replacement += block.Theirs
	}
	if block.End > block.Start && block.End <= len(a.Conflict.Result) {
		a.Conflict.Result = a.Conflict.Result[:block.Start] + replacement + a.Conflict.Result[block.End:]
	}
	if next := a.conflictBlocks(); len(next) > 0 {
		a.Conflict.Block = min(a.Conflict.Block, len(next)-1)
	} else {
		a.Conflict.Block = 0
	}
}

func (a *App) cycleConflict(delta int) {
	blocks := a.conflictBlocks()
	if len(blocks) == 0 {
		a.Conflict.Block = 0
		return
	}
	a.Conflict.Block = (a.Conflict.Block + delta + len(blocks)) % len(blocks)
}

func (a *App) SaveConflictResolution() {
	if a.Conflict.Loading || a.Conflict.Path == "" || a.Conflict.Error != "" && strings.HasPrefix(a.Conflict.Error, "Binary") {
		return
	}
	if blocks := a.conflictBlocks(); len(blocks) > 0 {
		a.Conflict.Error = fmt.Sprintf("%d conflict block(s) still contain markers", len(blocks))
		return
	}
	service, repo := a.Service, a.Repository
	path, result := a.Conflict.Path, a.Conflict.Result
	a.Conflict.Error = ""
	a.start(func(ctx context.Context) (func(), error) {
		written, err := service.ResolveConflict(ctx, repo, path, result)
		return func() {
			a.Writing = false
			if err != nil {
				a.Conflict.Error = message(err)
				a.Message = message(err)
				if written.Verified {
					a.applyStatus(written.Status)
				}
				return
			}
			if written.Verified {
				a.applyStatus(written.Status)
				a.Stale = false
			} else {
				a.Stale = true
			}
			a.Conflict = conflictState{}
			a.Message = "Conflict resolved · " + path
		}, nil
	})
	if a.Loading {
		a.Writing = true
		a.Message = "Saving conflict resolution · staging file"
	}
}

func (a *App) conflictView(c *ui.Context) {
	if !a.Conflict.Open {
		return
	}
	ui.Modal(c, &a.Conflict.Open, func() {
		width, height := c.Size()
		w := min(float32(1120), max(float32(760), width-80))
		h := min(float32(820), max(float32(560), height-60))
		ui.Column(c).Label("Three-way conflict resolver").Width(w).MaxHeight(h).Gap(0).
			Background(c.Theme().Background).Border(1, c.Theme().Border).Radius(10).Clip().Children(func() {
			ui.Row(c).Height(46).FillWidth().Padding(0, 16).Gap(8).AlignItems(ui.Center).
				Background(c.Theme().Surface).BorderWidth(0, 0, 1, 0).BorderColor(c.Theme().Border).Children(func() {
				ui.Text(c, "Merge Conflict").Bold().FontSize(14)
				ui.Text(c, a.Conflict.Path).Font("monospace").FontSize(11).TextColor(c.Theme().TextMuted).NoWrap().Grow(1)
				ui.Button(c, "Close").Height(26).OnClick(func() { a.Conflict = conflictState{} })
			})
			switch {
			case a.Conflict.Loading:
				ui.Column(c).Grow(1).FillWidth().Center().Children(func() {
					ui.Text(c, "Loading conflict stages…").FontSize(12).TextColor(c.Theme().TextMuted)
				})
				return
			case a.Conflict.Error != "" && a.Conflict.Data.Path == "":
				ui.Column(c).Grow(1).FillWidth().Center().Padding(20).Children(func() {
					ui.Text(c, a.Conflict.Error).FontSize(12).TextColor(c.Theme().Danger)
				})
				return
			}
			ui.Column(c).Grow(1).FillWidth().MinHeight(0).Padding(12).Gap(10).Children(func() {
				ui.Row(c).FillWidth().Grow(1).MinHeight(0).Gap(8).Children(func() {
					a.conflictStagePane(c, "BASE", a.Conflict.Data.Base, a.Conflict.Data.BaseAvailable)
					a.conflictStagePane(c, "OURS (CURRENT)", a.Conflict.Data.Ours, a.Conflict.Data.OursAvailable)
					a.conflictStagePane(c, "THEIRS (INCOMING)", a.Conflict.Data.Theirs, a.Conflict.Data.TheirsAvailable)
				})
				blocks := a.conflictBlocks()
				ui.Row(c).FillWidth().Height(30).Gap(6).AlignItems(ui.Center).Children(func() {
					if len(blocks) == 0 {
						ui.Text(c, "No conflict markers remain").FontSize(11).TextColor(c.Theme().Success).Grow(1)
					} else {
						a.Conflict.Block = min(max(a.Conflict.Block, 0), len(blocks)-1)
						ui.Textf(c, "Conflict %d of %d · line %d", a.Conflict.Block+1, len(blocks), blocks[a.Conflict.Block].StartLine).
							FontSize(11).TextColor(c.Theme().Warning).Grow(1)
						ui.Button(c, "Previous").Height(24).OnClick(func() { a.cycleConflict(-1) })
						ui.Button(c, "Next").Height(24).OnClick(func() { a.cycleConflict(1) })
						ui.Button(c, "Take Ours").Height(24).OnClick(func() { a.applyConflictChoice("ours") })
						ui.Button(c, "Take Theirs").Height(24).OnClick(func() { a.applyConflictChoice("theirs") })
						ui.Button(c, "Take Both").Height(24).OnClick(func() { a.applyConflictChoice("both") })
					}
				})
				ui.Column(c).FillWidth().Grow(1).MinHeight(150).Gap(4).Children(func() {
					ui.Text(c, "RESULT").Bold().FontSize(10).TextColor(c.Theme().TextMuted)
					ui.TextArea(c, &a.Conflict.Result).Label("Resolved file contents").FillWidth().Grow(1).
						Font("monospace").Background(c.Theme().Surface).Border(1, c.Theme().Border).Radius(4)
				})
				if a.Conflict.Error != "" {
					ui.Text(c, a.Conflict.Error).FontSize(11).TextColor(c.Theme().Danger)
				}
			})
			ui.Row(c).Height(44).FillWidth().Padding(0, 16).Gap(8).AlignItems(ui.Center).
				Background(c.Theme().Surface).BorderWidth(1, 0, 0, 0).BorderColor(c.Theme().Border).Children(func() {
				ui.Text(c, "Saving writes atomically, then stages only this path.").FontSize(10).TextColor(c.Theme().TextMuted).Grow(1)
				ui.Button(c, "Reload").Height(28).OnClick(func() { a.OpenConflict(a.Conflict.Path) })
				ui.Button(c, "Save and Mark Resolved").Height(28).Background(c.Theme().Accent).
					TextColor(c.Theme().AccentText).Disabled(a.Writing || a.Conflict.Data.Binary || a.Conflict.Data.Limited).
					OnClick(a.SaveConflictResolution)
			})
		})
	})
}

func (a *App) conflictStagePane(c *ui.Context, title, text string, available bool) {
	ui.Column(c).Grow(1).FillHeight().MinWidth(0).Gap(4).Children(func() {
		ui.Text(c, title).Bold().FontSize(10).TextColor(c.Theme().TextMuted)
		ui.Scroll(c).Label(title).Grow(1).FillWidth().MinHeight(0).
			Background(c.Theme().Surface).Border(1, c.Theme().Border).Radius(4).Children(func() {
			if !available {
				ui.Text(c, "(not present in this conflict)").Padding(8).FontSize(11).TextColor(c.Theme().TextMuted)
				return
			}
			ui.Text(c, text).Font("monospace").FontSize(11).Padding(8).Selectable()
		})
	})
}
