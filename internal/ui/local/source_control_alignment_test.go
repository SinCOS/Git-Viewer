package local

import (
	"gitviewer/internal/testgit"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/domain"
)

func sourceControlFixture(t *testing.T) *App {
	t.Helper()
	a := checkboxApp(t)
	a.Repository.Repository.DisplayName = "hyperf-skeleton"
	a.Entries = []domain.StatusEntry{
		{Change: domain.Change{Path: "docs/ticket-mcp-server.md", IndexStatus: " ", WorktreeStatus: "?", Untracked: true}, Scope: domain.WorktreeDiff, Group: "Untracked"},
		{Change: domain.Change{Path: "src/main.go", IndexStatus: "M", WorktreeStatus: " "}, Scope: domain.StagedDiff, Group: "Staged"},
	}
	return a
}

func TestSourceControlCompactFormAndMenus(t *testing.T) {
	for _, height := range []int{400, 850} {
		a := sourceControlFixture(t)
		tt := ui.NewTester(a.View, 1000, height)
		tt.Frame()
		if tt.HasText("Amend last commit") {
			t.Fatal("amend should be an action, not a permanent checkbox")
		}
		box := func(label string) ui.Rect {
			r, ok := tt.Find(label)
			if !ok {
				t.Fatalf("missing %s", label)
			}
			return r
		}
		input, form, changes := box("Commit message"), box("Commit form"), box("Local Changes")
		if input.H > 60 || form.H > 150 || changes.Y > input.Y+150 {
			t.Fatalf("commit form crowds out changed files: input=%+v form=%+v changes=%+v", input, form, changes)
		}
		if err := tt.Click("Commit actions"); err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(tt.Menu(), "Commit (Amend)") {
			t.Fatal("missing amend action", tt.Menu())
		}
		if err := tt.ChooseMenuItem("Commit (Amend)"); err != nil {
			t.Fatal(err)
		}
		if !a.Amend || a.Writing {
			t.Fatal("amend should select the mode before a message is committed")
		}
	}
}

func TestSourceControlTreeAppliesToEveryGroup(t *testing.T) {
	a := sourceControlFixture(t)
	tt := ui.NewTester(a.View, 1100, 850)
	tt.Frame()
	if _, ok := tt.Find(`Toggle directory Staged "src"`); !ok {
		t.Fatal("staged changes ignore tree mode")
	}
	if err := tt.Click(`Toggle directory Staged "src"`); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if tt.HasText("main.go") || !tt.HasText("ticket-mcp-server.md") {
		t.Fatal("staged folder collapse affected another group", tt.Texts())
	}
	if err := tt.Click("List"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	for _, folder := range []string{"src", "docs"} {
		if !tt.HasText(folder) {
			t.Fatalf("list mode hides parent path %s: %v", folder, tt.Texts())
		}
	}
}

func TestSourceControlInlineActionsAndGroupBoundaries(t *testing.T) {
	a := sourceControlFixture(t)
	tt := ui.NewTester(a.View, 1000, 850)
	tt.Frame()
	for _, label := range []string{`Stage file "docs/ticket-mcp-server.md"`, `Unstage file "src/main.go"`, "Unstage all changes", "Stage all Changes"} {
		if _, ok := tt.Find(label); !ok {
			t.Fatalf("missing direct action %s", label)
		}
	}
	for _, label := range []string{`Check Stage file "docs/ticket-mcp-server.md"`, "Discard staged changes", "Stage all changes"} {
		if _, ok := tt.Find(label); ok {
			t.Fatalf("unexpected action in source control: %s", label)
		}
	}
}

func TestSourceControlMessageShortcutCommits(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Write(t, filepath.Join(dir, "chosen.txt"), "chosen\n")
	testgit.Run(t, dir, "add", ".")
	a := checkboxApp(t)
	queue := make(chan func(), 16)
	a.Dispatch = func(apply func()) { queue <- apply }
	a.Open(dir)
	drainDialog(t, a, queue)
	a.MessageInput = "keyboard commit"
	tt := ui.NewTester(a.View, 1000, 650)
	tt.Frame()
	if err := tt.Click("Commit message"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyEnter)
	drainDialog(t, a, queue)
	if got := strings.TrimSpace(string(testgit.Run(t, dir, "log", "-1", "--format=%s"))); got != "keyboard commit" {
		t.Fatalf("keyboard commit failed: %s", got)
	}
}

func TestSourceControlDiscardCapturesFileAndConfirms(t *testing.T) {
	a := checkboxApp(t)
	a.Entries = []domain.StatusEntry{
		{Change: domain.Change{Path: "src/changed.go", IndexStatus: " ", WorktreeStatus: "M"}, Scope: domain.WorktreeDiff, Group: "Unstaged"},
		{Change: domain.Change{Path: "new.txt", Untracked: true}, Scope: domain.WorktreeDiff, Group: "Untracked"},
	}
	a.Selected = 1
	queue := make(chan func(), 8)
	a.Dispatch = func(apply func()) { queue <- apply }
	var detail string
	a.Confirm = func(_, message string) bool { detail = message; return false }
	tt := ui.NewTester(a.View, 1000, 650)
	tt.Frame()
	if _, ok := tt.Find(`Discard changes for "new.txt"`); ok {
		t.Fatal("untracked deletion must not be offered by the tracked-file writer")
	}
	if err := tt.Click(`Discard changes for "src/changed.go"`); err != nil {
		t.Fatal(err)
	}
	drainDialog(t, a, queue)
	if !strings.Contains(detail, `"src/changed.go"`) || strings.Contains(detail, `"new.txt"`) || a.Selected != 1 {
		t.Fatalf("discard targeted the selection instead of its row: %s selection=%d", detail, a.Selected)
	}
}

func TestSourceControlOrdinaryEnterAndUnavailableShortcut(t *testing.T) {
	a := sourceControlFixture(t)
	a.MessageInput = "subject"
	tt := ui.NewTester(a.View, 1000, 650)
	tt.Frame()
	if err := tt.Click("Commit message"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEnter)
	if a.MessageInput != "subject\n" || a.Writing {
		t.Fatalf("Enter should insert a newline: %q", a.MessageInput)
	}
	a.Stale = true
	tt.Frame()
	tt.Key(ui.Cmd, ui.KeyEnter)
	if a.Writing || a.Loading {
		t.Fatal("shortcut bypassed disabled commit state")
	}
}

// Export reproducible light/dark renderer evidence without touching a real repo.
func TestCaptureSourceControlAlignment(t *testing.T) {
	output := os.Getenv("GITVIEWER_CAPTURE_SCM")
	if output == "" {
		t.Skip("set GITVIEWER_CAPTURE_SCM for renderer evidence")
	}
	if err := os.MkdirAll(output, 0700); err != nil {
		t.Fatal(err)
	}
	for _, dark := range []bool{false, true} {
		for _, width := range []int{300, 420} {
			a := sourceControlFixture(t)
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(vscodeTheme(c.Theme()))
				a.commitSidebarPane(c)
			}, width, 650)
			tt.SetDark(dark)
			tt.Frame()
			suffix := "light"
			if dark {
				suffix = "dark"
			}
			if width == 300 {
				suffix += "-narrow"
			}
			f, err := os.Create(filepath.Join(output, "source-control-"+suffix+".png"))
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, tt.Image())
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}
