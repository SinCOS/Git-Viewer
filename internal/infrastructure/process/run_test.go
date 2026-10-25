package process

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"gitviewer/internal/domain"
)

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GITVIEWER_HELPER") != "1" {
		return
	}
	switch os.Getenv("GITVIEWER_MODE") {
	case "exit":
		fmt.Fprint(os.Stdout, "out\x00")
		fmt.Fprint(os.Stderr, "err")
		os.Exit(7)
	case "wait":
		time.Sleep(20 * time.Second)
	case "flood":
		for {
			fmt.Fprint(os.Stdout, strings.Repeat("x", 4096))
		}
	}
	os.Exit(0)
}
func TestProcessBoundaries(t *testing.T) {
	for _, tc := range []struct {
		mode    string
		timeout time.Duration
		cancel  bool
		code    domain.ErrorCode
	}{
		{"exit", time.Second * 5, false, domain.CommandFailed},
		{"wait", time.Millisecond * 50, false, domain.OperationTimedOut},
		{"wait", time.Second * 5, true, domain.OperationCancelled},
		{"flood", time.Second * 5, false, domain.OutputLimitExceeded},
	} {
		t.Run(tc.mode+string(tc.code), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				time.AfterFunc(50*time.Millisecond, cancel)
			}
			before := time.Now()
			result, err := Run(ctx, tc.timeout, 1024, os.Args[0], []string{"-test.run=^TestHelperProcess$"}, append(os.Environ(), "GITVIEWER_HELPER=1", "GITVIEWER_MODE="+tc.mode))
			var ge *domain.GitError
			if !errors.As(err, &ge) || ge.Code != tc.code {
				t.Fatalf("%v", err)
			}
			if time.Since(before) > 3*time.Second {
				t.Fatal("process did not stop promptly")
			}
			if len(result.Stdout)+len(result.Stderr) > 1024 {
				t.Fatal("output exceeded budget")
			}
			if tc.mode == "exit" && (result.ExitCode != 7 || string(result.Stdout) != "out\x00" || string(result.Stderr) != "err") {
				t.Fatalf("%+v", result)
			}
		})
	}
}
