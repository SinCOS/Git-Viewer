package ssh

import (
	"context"
	"errors"
	"testing"
	"time"

	"gitviewer/internal/domain"
)

func TestSSHRunningLifecycle(t *testing.T) {
	for _, scenario := range []struct {
		name string
		code domain.ErrorCode
	}{
		{"cancel", domain.OperationCancelled},
		{"timeout", domain.OperationTimedOut},
		{"disconnect", domain.RemoteUnavailable},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			fixture := newRuntimeFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if scenario.name == "timeout" {
				var deadlineCancel context.CancelFunc
				ctx, deadlineCancel = context.WithTimeout(ctx, 1500*time.Millisecond)
				defer deadlineCancel()
			}
			command := remoteCommand(t.TempDir())
			command.Timeout = 6 * time.Second
			type outcome struct {
				result domain.GitResult
				err    error
			}
			finished := make(chan outcome, 1)
			go func() { r, err := fixture.executor.Execute(ctx, command); finished <- outcome{r, err} }()
			fixture.waitStarted(t)
			if ctx.Err() != nil {
				t.Fatal("context expired before authenticated remote task started")
			}
			select {
			case got := <-finished:
				t.Fatalf("task completed before trigger: %v", got.err)
			default:
			}
			trigger := time.Now()
			switch scenario.name {
			case "cancel":
				cancel()
			case "disconnect":
				fixture.proxy.disconnect()
			case "timeout":
				<-ctx.Done()
				trigger = time.Now()
			}
			select {
			case got := <-finished:
				elapsed := time.Since(trigger)
				var ge *domain.GitError
				if !errors.As(got.err, &ge) || ge.Code != scenario.code {
					t.Fatalf("expected %s, got %v; stderr %s", scenario.code, got.err, got.result.Stderr)
				}
				if elapsed > 3*time.Second {
					t.Fatalf("return took %s", elapsed)
				}
				t.Logf("authenticated remote PID observed; %s returned %s in %.3f ms after trigger; remote helper explicitly cleaned up", scenario.name, ge.Code, float64(elapsed)/float64(time.Millisecond))
			case <-time.After(3 * time.Second):
				t.Fatal("running SSH operation did not return within 3 seconds")
			}
		})
	}
}
