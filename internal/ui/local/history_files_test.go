package local_test

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/testgit"
)

func numberedFile(lines int, changed int) string {
	var b strings.Builder
	for i := 1; i <= lines; i++ {
		if i == changed {
			b.WriteString("changed line ")
			b.WriteString(strconv.Itoa(i))
		} else {
			b.WriteString("line ")
			b.WriteString(strconv.Itoa(i))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func TestCommitDetailsFileNavigationKeepsSingleHunkState(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Write(t, filepath.Join(dir, "a.txt"), "old-a\n")
	testgit.Write(t, filepath.Join(dir, "b.txt"), "old-b\n")
	testgit.Run(t, dir, "add", ".")
	testgit.Run(t, dir, "commit", "-m", "two files")
	testgit.Write(t, filepath.Join(dir, "a.txt"), "new-a\n")
	testgit.Write(t, filepath.Join(dir, "b.txt"), "new-b\n")
	testgit.Run(t, dir, "add", ".")
	testgit.Run(t, dir, "commit", "-m", "change both files")

	a, queue := setup(t)
	a.Open(dir)
	settle(t, a, queue)
	a.Pane = "History"
	tt := ui.NewTester(a.View, 1400, 850)
	tt.Frame()
	settle(t, a, queue)
	tt.Frame()
	if err := tt.Click("change both files"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, queue)
	tt.Frame()
	if a.Preview.Path != "" {
		t.Fatalf("commit should initially show the combined diff: %q", a.Preview.Path)
	}
	if !tt.HasText("Previous file") || !tt.HasText("Next file") {
		t.Fatalf("missing changed-file navigation: %v", tt.Texts())
	}

	selectAndAssert := func(button, path string) {
		t.Helper()
		if err := tt.Click(button); err != nil {
			t.Fatal(err)
		}
		settle(t, a, queue)
		tt.Frame()
		if a.Preview.Path != path {
			t.Fatalf("%s selected %q, want %q", button, a.Preview.Path, path)
		}
		if !tt.HasText("1 change") {
			t.Fatalf("%s is missing the single-change state: %v", path, tt.Texts())
		}
		if tt.HasText("Previous change") || tt.HasText("Next change") {
			t.Fatalf("%s still exposes useless hunk navigation: %v", path, tt.Texts())
		}
	}
	selectAndAssert("Next file", "a.txt")
	selectAndAssert("Next file", "b.txt")
	selectAndAssert("Previous file", "a.txt")

	if err := tt.Click("b.txt"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, queue)
	tt.Frame()
	tt.Key(0, ui.KeyUp)
	settle(t, a, queue)
	tt.Frame()
	if a.Preview.Path != "a.txt" {
		t.Fatalf("keyboard up selected %q, want a.txt", a.Preview.Path)
	}
	tt.Key(0, ui.KeyDown)
	settle(t, a, queue)
	tt.Frame()
	if a.Preview.Path != "b.txt" || !strings.Contains(a.Preview.Text, "+new-b") {
		t.Fatalf("keyboard down did not return to b.txt: %+v", a.Preview)
	}
	if tt.HasText("Previous change") || tt.HasText("Next change") {
		t.Fatalf("switching files and back restored single-hunk navigation: %v", tt.Texts())
	}
}

func TestCommitDetailsHidesFileNavigationForOneFile(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Write(t, filepath.Join(dir, "only.txt"), "old\n")
	testgit.Run(t, dir, "add", ".")
	testgit.Run(t, dir, "commit", "-m", "one file")
	testgit.Write(t, filepath.Join(dir, "only.txt"), "new\n")
	testgit.Run(t, dir, "add", ".")
	testgit.Run(t, dir, "commit", "-m", "change one file")

	a, queue := setup(t)
	a.Open(dir)
	settle(t, a, queue)
	a.Pane = "History"
	tt := ui.NewTester(a.View, 1400, 850)
	tt.Frame()
	settle(t, a, queue)
	tt.Frame()
	if err := tt.Click("change one file"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, queue)
	tt.Frame()
	if tt.HasText("Previous file") || tt.HasText("Next file") {
		t.Fatalf("single-file commit exposes file navigation: %v", tt.Texts())
	}
}

func TestCommitDetailsFileClickJumpsToFirstChange(t *testing.T) {
	dir := testgit.Repo(t)
	name := filepath.Join(dir, "long.txt")
	testgit.Write(t, name, numberedFile(120, 0))
	testgit.Run(t, dir, "add", ".")
	testgit.Run(t, dir, "commit", "-m", "long file")
	testgit.Write(t, name, numberedFile(120, 90))
	testgit.Run(t, dir, "add", ".")
	testgit.Run(t, dir, "commit", "-m", "change near the end")

	a, queue := setup(t)
	a.Open(dir)
	settle(t, a, queue)
	a.Pane = "History"
	tt := ui.NewTester(a.View, 1400, 850)
	tt.Frame()
	settle(t, a, queue)
	tt.Frame()
	if err := tt.Click("change near the end"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, queue)
	tt.Frame()
	if err := tt.Click("long.txt"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, queue)
	tt.Frame()
	line, hunk, inside := a.NativeDiffCursorForTest()
	if line != 89 || hunk != 0 || !inside {
		t.Fatalf("file click landed at line %d, hunk %d inside=%t; want line 89 in hunk 0", line, hunk, inside)
	}
}
