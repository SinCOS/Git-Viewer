package local

import (
	"context"
	"errors"
	"strings"

	"github.com/egoist/mygo/ui"
)

type passwordPromptResult struct {
	password string
	err      error
}

var errPasswordPromptUnavailable = errors.New("password prompt is unavailable")

// RequestPassword asks the window that owns the SSH operation for a password.
// The password is returned directly to the askpass broker and is never stored
// in App state after the modal closes.
func (a *App) RequestPassword(ctx context.Context, host, prompt string) (string, error) {
	if a.Dispatch == nil {
		return "", errPasswordPromptUnavailable
	}
	result := make(chan passwordPromptResult, 1)
	a.Dispatch(func() {
		if ctx.Err() != nil {
			result <- passwordPromptResult{err: ctx.Err()}
			return
		}
		if a.closed || a.AskPasswordOpen || a.RemoteOpen || a.QuickOpen || a.DialogOpen {
			result <- passwordPromptResult{err: errPasswordPromptUnavailable}
			return
		}
		a.AskPasswordOpen = true
		a.AskPasswordHost = host
		a.AskPasswordPrompt = prompt
		a.AskPasswordInput = ""
		a.AskPasswordMessage = ""
		a.askPasswordResult = result
	})
	select {
	case answer := <-result:
		return answer.password, answer.err
	case <-ctx.Done():
		a.Dispatch(func() {
			if a.askPasswordResult == result {
				a.closePasswordPrompt(passwordPromptResult{err: ctx.Err()})
			}
		})
		return "", ctx.Err()
	}
}

func (a *App) submitPasswordPrompt() {
	if !a.AskPasswordOpen {
		return
	}
	if a.AskPasswordInput == "" {
		a.AskPasswordMessage = "Enter a password or verification code"
		return
	}
	password := a.AskPasswordInput
	a.closePasswordPrompt(passwordPromptResult{password: password})
}

func (a *App) cancelPasswordPrompt() {
	if !a.AskPasswordOpen {
		return
	}
	a.closePasswordPrompt(passwordPromptResult{err: errors.New("password prompt cancelled")})
}

func (a *App) closePasswordPrompt(answer passwordPromptResult) {
	result := a.askPasswordResult
	a.askPasswordResult = nil
	a.AskPasswordOpen = false
	a.AskPasswordHost = ""
	a.AskPasswordPrompt = ""
	a.AskPasswordInput = ""
	a.AskPasswordMessage = ""
	if result != nil {
		select {
		case result <- answer:
		default:
		}
	}
}

func (a *App) passwordPromptView(c *ui.Context) {
	ui.Modal(c, &a.AskPasswordOpen, func() {
		ui.Column(c).Width(480).MaxHeight(360).Padding(14).Gap(10).
			Background(c.Theme().Surface).Border(1, c.Theme().Border).Radius(6).Children(func() {
			ui.Text(c, "SSH authentication").FontSize(15).Bold()
			if a.AskPasswordHost != "" {
				ui.Text(c, a.AskPasswordHost).FontSize(11).TextColor(c.Theme().TextMuted)
			}
			if prompt := strings.TrimSpace(a.AskPasswordPrompt); prompt != "" {
				ui.Text(c, prompt).FontSize(12)
			}
			input := ui.TextInput(c, &a.AskPasswordInput).
				Label("Password").Placeholder("Password or verification code").
				Password().FillWidth().AutoFocus()
			if input.Submitted() {
				a.submitPasswordPrompt()
				return
			}
			ui.Text(c, "The password is kept only for this SSH prompt. OpenSSH still verifies the host key.").
				FontSize(11).TextColor(c.Theme().TextMuted)
			if message := strings.TrimSpace(a.AskPasswordMessage); message != "" {
				ui.Text(c, message).FontSize(11).TextColor(c.Theme().Danger)
			}
			ui.Row(c).FillWidth().Gap(8).Children(func() {
				ui.Box(c).Grow(1)
				ui.Button(c, "Cancel").Height(26).OnClick(a.cancelPasswordPrompt)
				ui.PrimaryButton(c, "Continue").Height(26).
					Disabled(a.AskPasswordInput == "").OnClick(a.submitPasswordPrompt)
			})
		})
	})
}
