package ssh

import (
	"strings"
	"testing"
)

func TestConnectionArgsAllowPasswordOptIn(t *testing.T) {
	strict := strings.Join(Executor{}.ConnectionArgs(), " ")
	if !strings.Contains(strict, "BatchMode=yes") {
		t.Fatalf("strict args = %s", strict)
	}
	interactive := strings.Join(Executor{AllowPassword: true}.ConnectionArgs(), " ")
	if !strings.Contains(interactive, "BatchMode=no") || !strings.Contains(interactive, "StrictHostKeyChecking=yes") {
		t.Fatalf("interactive args = %s", interactive)
	}
	forward := strings.Join(Executor{AllowPassword: true}.ForwardArgs("/tmp/local.sock", "/tmp/remote.sock"), " ")
	if !strings.Contains(forward, "BatchMode=no") || !strings.Contains(forward, "StrictHostKeyChecking=yes") {
		t.Fatalf("forward args = %s", forward)
	}
}

func TestEnvironmentIncludesAskpassHost(t *testing.T) {
	env := strings.Join(Executor{}.Environment("dev-box"), "\n")
	if !strings.Contains(env, "GITVIEWER_ASKPASS_HOST=dev-box") {
		t.Fatalf("environment = %s", env)
	}
}
