package local_test

import (
	"bytes"
	"image"
	"image/draw"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"gitviewer/internal/testgit"
)

func captureRegion(t *testing.T, tt *ui.Tester, label string) []byte {
	t.Helper()
	r, ok := tt.Find(label)
	if !ok {
		t.Fatal("missing region", label)
	}
	bounds := image.Rect(int(r.X), int(r.Y), int(r.X+r.W), int(r.Y+r.H))
	cropped := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(cropped, cropped.Bounds(), tt.Image(), bounds.Min, draw.Src)
	return cropped.Pix
}

func TestCommitReadDoesNotFadeUnrelatedPanels(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Write(t, filepath.Join(dir, "file"), "content\n")
	testgit.Run(t, dir, "add", "file")
	testgit.Run(t, dir, "commit", "-m", "quiet commit")
	a, q := setup(t)
	a.Open(dir)
	settle(t, a, q)
	a.Pane = "History"
	tt := ui.NewTester(a.View, 1400, 850)
	settle(t, a, q)
	tt.Frame()
	r, ok := tt.Find("quiet commit")
	if !ok {
		t.Fatal("missing commit")
	}
	tt.Move(r.X+r.W/2, r.Y+r.H/2)
	regions := []string{"Branch navigation", "Search commits", "Branch tree", "All Branches"}
	before := make(map[string][]byte)
	for _, label := range regions {
		before[label] = captureRegion(t, tt, label)
	}
	tt.Frame()
	for _, label := range regions {
		if !bytes.Equal(before[label], captureRegion(t, tt, label)) {
			t.Fatalf("idle frames are not visually stable for %s", label)
		}
	}
	if err := tt.Click("quiet commit"); err != nil {
		t.Fatal(err)
	}
	if !a.Loading {
		t.Fatal("read did not start")
	}
	tt.Frame()
	for _, label := range regions {
		after := captureRegion(t, tt, label)
		if !bytes.Equal(before[label], after) {
			r, _ := tt.Find(label)
			minX, minY, maxX, maxY := int(r.W), int(r.H), 0, 0
			for i := 0; i+3 < len(after); i += 4 {
				if !bytes.Equal(before[label][i:i+4], after[i:i+4]) {
					x, y := (i/4)%int(r.W), (i/4)/int(r.W)
					minX, minY, maxX, maxY = min(minX, x), min(minY, y), max(maxX, x), max(maxY, y)
				}
			}
			t.Errorf("unrelated region changed appearance during read: %s (%d,%d)-(%d,%d)", label, minX, minY, maxX, maxY)
		}
	}
	settle(t, a, q)
}
