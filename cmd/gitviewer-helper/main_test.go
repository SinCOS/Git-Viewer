package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"gitviewer/internal/remotehelper"
)

func TestVersion(t *testing.T) {
	var out bytes.Buffer
	if code := run([]string{"--version"}, strings.NewReader(""), &out); code != 0 {
		t.Fatalf("exit %d", code)
	}
	want := fmt.Sprintf("gitviewer-helper %d %s/%s", remotehelper.Proto, runtime.GOOS, runtime.GOARCH)
	if !strings.Contains(out.String(), want) {
		t.Fatalf("%q does not contain %q", out.String(), want)
	}
}

func TestInvalidArguments(t *testing.T) {
	for _, args := range [][]string{{"--bogus"}, {}, {"extra"}, {"--serve", "extra"}, {"--serve", "--version"}} {
		if code := run(args, strings.NewReader(""), io.Discard); code != 2 {
			t.Errorf("%v: exit %d", args, code)
		}
	}
}

func TestServeHandshakeAndShutdown(t *testing.T) {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan int, 1)
	go func() { done <- run([]string{"--serve"}, inR, outW) }()
	defer func() { _ = inW.Close(); _ = outR.Close() }()
	if err := remotehelper.WriteRequest(inW, remotehelper.Request{Type: remotehelper.TypeHello, Proto: remotehelper.Proto}); err != nil {
		t.Fatal(err)
	}
	payload, err := remotehelper.ReadFrame(outR)
	if err != nil {
		t.Fatal(err)
	}
	response, err := remotehelper.DecodeResponse(payload)
	if err != nil || response.Type != remotehelper.TypeReady || response.Platform != runtime.GOOS+"/"+runtime.GOARCH {
		t.Fatalf("unexpected ready: %+v %v", response, err)
	}
	if err := remotehelper.WriteRequest(inW, remotehelper.Request{Type: remotehelper.TypeShutdown}); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not stop")
	}
}

func TestServeBadFrameExitsOne(t *testing.T) {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan int, 1)
	go func() { done <- run([]string{"--serve"}, inR, outW) }()
	defer func() { _ = inW.Close(); _ = outR.Close() }()
	if _, err := inW.Write([]byte{0, 0, 0, 5, '{', 'b', 'a', 'd', '}'}); err != nil {
		t.Fatal(err)
	}
	payload, err := remotehelper.ReadFrame(outR)
	if err != nil {
		t.Fatal(err)
	}
	if response, err := remotehelper.DecodeResponse(payload); err != nil || response.Type != remotehelper.TypeFatal {
		t.Fatalf("unexpected response: %+v %v", response, err)
	}
	select {
	case code := <-done:
		if code != 1 {
			t.Fatalf("exit %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not stop")
	}
}

func TestServeSubprocessSIGTERMUnblocksInheritedStdio(t *testing.T) {
	if os.Getenv("GITVIEWER_HELPER_SIGNAL_CHILD") == "1" {
		os.Exit(run([]string{"--serve"}, os.Stdin, os.Stdout))
	}
	for _, blockedWrite := range []bool{false, true} {
		t.Run(fmt.Sprint(blockedWrite), func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestServeSubprocessSIGTERMUnblocksInheritedStdio$")
			cmd.Env = append(os.Environ(), "GITVIEWER_HELPER_SIGNAL_CHILD=1")
			input, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			output, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer cmd.Process.Kill()
			if err := remotehelper.WriteRequest(input, remotehelper.Request{Type: remotehelper.TypeHello, Proto: remotehelper.Proto}); err != nil {
				t.Fatal(err)
			}
			if _, err := remotehelper.ReadFrame(output); err != nil {
				t.Fatal(err)
			}
			if blockedWrite {
				// One large result blocks the inherited stdout pipe without
				// overflowing the server's bounded response admission queue.
				dir := t.TempDir()
				file := filepath.Join(dir, "large.txt")
				if err := os.WriteFile(file, bytes.Repeat([]byte("line\n"), 100000), 0600); err != nil {
					t.Fatal(err)
				}
				cmd := exec.Command("git", "-C", dir, "init")
				if output, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git init: %s %v", output, err)
				}
				request := remotehelper.Request{Type: remotehelper.TypeExec, ID: 1, Dir: []byte(dir), Args: [][]byte{[]byte("diff"), []byte("--no-index"), []byte("--no-ext-diff"), []byte("--no-textconv"), []byte("--no-color"), []byte("--"), []byte("/dev/null"), []byte("large.txt")}}
				if err := remotehelper.WriteRequest(input, request); err != nil {
					t.Fatal(err)
				}
				// Observe only the result header, leaving its payload blocked.
				var header [4]byte
				if _, err := io.ReadFull(output, header[:]); err != nil {
					t.Fatal(err)
				}
			}
			if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			select {
			case err := <-done:
				if err != nil {
					t.Fatalf("SIGTERM exit: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("SIGTERM did not unblock inherited descriptors")
			}
		})
	}
}
