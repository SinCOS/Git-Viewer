package local

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPasswordPromptViewAndSubmit(t *testing.T) {
	result := make(chan passwordPromptResult, 1)
	a := &App{
		AskPasswordOpen:   true,
		AskPasswordHost:   "dev-box",
		AskPasswordPrompt: "dev-box password:",
		AskPasswordInput:  "secret",
		askPasswordResult: result,
	}
	tt := ui.NewTester(func(c *ui.Context) { a.passwordPromptView(c) }, 700, 500)
	tt.Frame()
	for _, label := range []string{"SSH authentication", "dev-box", "dev-box password:", "Password", "Continue"} {
		if !tt.HasText(label) {
			t.Fatalf("missing %q in %v", label, tt.Texts())
		}
	}
	a.submitPasswordPrompt()
	answer := <-result
	if answer.err != nil || answer.password != "secret" {
		t.Fatalf("answer = %+v", answer)
	}
	if a.AskPasswordOpen || a.AskPasswordInput != "" {
		t.Fatal("password prompt state was not cleared")
	}
}
