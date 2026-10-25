package local_test

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestOperationsLayoutLabels(t *testing.T) {
	a, _ := setup(t)
	tt := ui.NewTester(a.View, 1600, 1000)
	tt.Frame()
	for _, label := range []string{"Commit and Push", "Commit staged", "Stage file", "Connect SSH", "Switch branch"} {
		if !tt.HasText(label) {
			t.Fatalf("missing button label %q in %v", label, tt.Texts())
		}
	}
	hasConnectHeading := false
	for _, s := range tt.Texts() {
		if s == "Connect" {
			hasConnectHeading = true
		}
	}
	if !hasConnectHeading {
		t.Fatalf("missing Connect heading in %v", tt.Texts())
	}
}

func TestOperationsDisabledWhileLoading(t *testing.T) {
	a, queue := setup(t)
	a.Loading = true
	tt := ui.NewTester(a.View, 1600, 1000)
	tt.Frame()
	// "Connect SSH" is Disabled(a.Loading) and OpenSSH has no Loading guard of
	// its own, so a click that fired would start a worker and enqueue a dispatch.
	if err := tt.Click("Connect SSH"); err != nil {
		t.Fatal(err)
	}
	a.Wait()
	if len(queue) != 0 {
		t.Fatal("button was not disabled while Loading")
	}
}
