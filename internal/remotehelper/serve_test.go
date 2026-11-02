//go:build unix

package remotehelper

import (
	"context"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"gitviewer/internal/testgit"
)

type helperHarness struct {
	in      *io.PipeWriter
	out     *io.PipeReader
	done    chan error
	closeIn func()
}

func startHarness(t *testing.T, engine *Engine) *helperHarness {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- Serve(context.Background(), inR, outW, engine) }()
	h := &helperHarness{in: inW, out: outR, done: done, closeIn: func() { _ = inW.Close() }}
	t.Cleanup(func() {
		_ = inW.Close()
		_ = outR.Close()
		if h.done == nil {
			return
		}
		select {
		case <-h.done:
		case <-time.After(5 * time.Second):
			t.Error("helper did not stop")
		}
	})
	if err := WriteRequest(inW, Request{Type: TypeHello, Proto: Proto}); err != nil {
		t.Fatal(err)
	}
	ready := h.read(t)
	if ready.Type != TypeReady || ready.Proto != Proto || ready.Platform == "" || ready.PID == 0 {
		t.Fatalf("unexpected ready: %+v", ready)
	}
	return h
}

func (h *helperHarness) read(t *testing.T) Response {
	t.Helper()
	type outcome struct {
		response Response
		err      error
	}
	ch := make(chan outcome, 1)
	go func() {
		payload, err := ReadFrame(h.out)
		if err != nil {
			ch <- outcome{err: err}
			return
		}
		response, err := DecodeResponse(payload)
		ch <- outcome{response, err}
	}()
	select {
	case got := <-ch:
		if got.err != nil {
			t.Fatalf("read response: %v", got.err)
		}
		return got.response
	case <-time.After(10 * time.Second):
		t.Fatal("timed out reading helper response")
		return Response{}
	}
}

func (h *helperHarness) wait(t *testing.T) error {
	t.Helper()
	select {
	case err := <-h.done:
		h.done = nil
		return err
	case <-time.After(8 * time.Second):
		t.Fatal("helper did not stop")
		return nil
	}
}

func (h *helperHarness) send(t *testing.T, request Request) {
	t.Helper()
	if err := WriteRequest(h.in, request); err != nil {
		t.Fatal(err)
	}
}

func TestServeExecBatchPingCancelAndShutdown(t *testing.T) {
	dir := testgit.Repo(t)
	testgit.Write(t, filepath.Join(dir, "a.txt"), "one\n")
	h := startHarness(t, NewEngine(""))

	h.send(t, Request{Type: TypeExec, ID: 1, Dir: []byte(dir), Limit: 1 << 20, Args: [][]byte{[]byte("status"), []byte("--porcelain=v1"), []byte("-z"), []byte("--untracked-files=all")}})
	h.send(t, Request{Type: TypeExec, ID: 2, Dir: []byte(dir), Limit: 1 << 20, Args: [][]byte{[]byte("rev-parse"), []byte("--show-toplevel")}})
	seen := map[uint64]Response{}
	for len(seen) < 2 {
		response := h.read(t)
		if response.Type != TypeResult {
			t.Fatalf("unexpected response: %+v", response)
		}
		seen[response.ID] = response
	}
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(seen[1].Stdout), "a.txt") || strings.TrimSpace(string(seen[2].Stdout)) != canonical {
		t.Fatalf("unexpected exec results: %+v", seen)
	}

	h.send(t, Request{Type: TypeCancel, ID: 999})
	h.send(t, Request{Type: TypePing})
	if pong := h.read(t); pong.Type != TypePong {
		t.Fatalf("unexpected ping response: %+v", pong)
	}

	h.send(t, Request{Type: TypeBatch, ID: 3, Dir: []byte(dir), Limit: 1 << 20, Commands: [][][]byte{
		{[]byte("rev-parse"), []byte("--show-toplevel")},
		{[]byte("for-each-ref"), []byte("--format=%(objectname)")},
	}})
	batch := h.read(t)
	if batch.Type != TypeBatchResult || batch.ID != 3 || len(batch.Results) != 2 || batch.Error != nil {
		t.Fatalf("unexpected batch: %+v", batch)
	}
	if strings.TrimSpace(string(batch.Results[0].Stdout)) != canonical {
		t.Fatalf("unexpected batch output: %q", batch.Results[0].Stdout)
	}

	h.send(t, Request{Type: TypeShutdown})
	if err := h.wait(t); err != nil {
		t.Fatal(err)
	}
}

func TestServeRejectsBadProtocol(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, h *helperHarness)
	}{
		{"wrong proto", func(t *testing.T, h *helperHarness) {
			_ = WriteRequest(h.in, Request{Type: TypeHello, Proto: Proto + 1})
		}},
		{"unknown type", func(t *testing.T, h *helperHarness) {
			h.send(t, Request{Type: "delete-everything"})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inR, inW := io.Pipe()
			outR, outW := io.Pipe()
			done := make(chan error, 1)
			go func() { done <- Serve(context.Background(), inR, outW, NewEngine("")) }()
			defer func() { _ = inW.Close(); _ = outR.Close() }()
			if tc.name != "wrong proto" {
				_ = WriteRequest(inW, Request{Type: TypeHello, Proto: Proto})
				if payload, err := ReadFrame(outR); err != nil {
					t.Fatal(err)
				} else if response, _ := DecodeResponse(payload); response.Type != TypeReady {
					t.Fatalf("unexpected ready: %+v", response)
				}
			}
			tc.setup(t, &helperHarness{in: inW, out: outR})
			payload, err := ReadFrame(outR)
			if err != nil {
				t.Fatal(err)
			}
			response, err := DecodeResponse(payload)
			if err != nil || response.Type != TypeFatal {
				t.Fatalf("expected fatal, got %+v %v", response, err)
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("protocol error did not fail Serve")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("protocol error did not stop Serve")
			}
		})
	}
}

func TestServeRejectsMalformedFrame(t *testing.T) {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- Serve(context.Background(), inR, outW, NewEngine("")) }()
	defer func() { _ = inW.Close(); _ = outR.Close() }()
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], 5)
	if _, err := inW.Write(append(header[:], []byte("{bad}")...)); err != nil {
		t.Fatal(err)
	}
	payload, err := ReadFrame(outR)
	if err != nil {
		t.Fatal(err)
	}
	if response, _ := DecodeResponse(payload); response.Type != TypeFatal {
		t.Fatalf("expected fatal, got %+v", response)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("malformed frame did not stop Serve")
	}
}

func TestServeEOFStopsRunningCommand(t *testing.T) {
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "git.pid")
	script := filepath.Join(dir, "git")
	testgit.Write(t, script, "#!/bin/sh\necho $$ > "+pidFile+"\nsleep 30\n")
	if err := os.Chmod(script, 0700); err != nil {
		t.Fatal(err)
	}
	h := startHarness(t, NewEngine(script))
	h.send(t, Request{Type: TypeExec, ID: 1, Dir: []byte(dir), Args: [][]byte{[]byte("--version")}})
	var pid int
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		data, err := os.ReadFile(pidFile)
		if err == nil && len(data) > 0 {
			pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
			if pid != 0 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("command never started")
	}
	h.closeIn()
	if err := h.wait(t); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		if syscall.Kill(pid, 0) == syscall.ESRCH {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("running command %d survived EOF", pid)
}

func TestServeBatchRejectsWritesAndOversize(t *testing.T) {
	dir := testgit.Repo(t)
	h := startHarness(t, NewEngine(""))
	h.send(t, Request{Type: TypeBatch, ID: 1, Dir: []byte(dir), Commands: [][][]byte{
		{[]byte("stash"), []byte("push")},
	}})
	response := h.read(t)
	if response.Type != TypeBatchResult || response.Error == nil || response.Error.Code != "UnsafeOperation" {
		t.Fatalf("expected batch rejection, got %+v", response)
	}
	h.send(t, Request{Type: TypeBatch, ID: 2, Dir: []byte(dir), Commands: nil})
	response = h.read(t)
	if response.Type != TypeBatchResult || response.Error == nil || response.Error.Code != "UnsafeOperation" {
		t.Fatalf("expected empty batch rejection, got %+v", response)
	}
}

func TestServeDuplicateIDIsFatal(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "git")
	testgit.Write(t, script, "#!/bin/sh\nsleep 5\n")
	if err := os.Chmod(script, 0700); err != nil {
		t.Fatal(err)
	}
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- Serve(context.Background(), inR, outW, NewEngine(script)) }()
	defer func() { _ = inW.Close(); _ = outR.Close() }()
	_ = WriteRequest(inW, Request{Type: TypeHello, Proto: Proto})
	if payload, err := ReadFrame(outR); err != nil {
		t.Fatal(err)
	} else if response, _ := DecodeResponse(payload); response.Type != TypeReady {
		t.Fatalf("unexpected ready: %+v", response)
	}
	request := Request{Type: TypeExec, ID: 7, Dir: []byte(dir), Args: [][]byte{[]byte("--version")}}
	_ = WriteRequest(inW, request)
	_ = WriteRequest(inW, request)
	payload, err := ReadFrame(outR)
	if err != nil {
		t.Fatal(err)
	}
	if response, _ := DecodeResponse(payload); response.Type != TypeFatal {
		t.Fatalf("expected fatal for duplicate id, got %+v", response)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("duplicate id did not fail Serve")
		}
	case <-time.After(8 * time.Second):
		t.Fatal("duplicate id did not stop Serve")
	}
}

func TestServeRequiresHelloFirstAndOnce(t *testing.T) {
	for _, first := range []Request{{Type: TypePing}, {Type: TypeExec, ID: 1}, {Type: TypeShutdown}} {
		inR, inW := io.Pipe()
		outR, outW := io.Pipe()
		done := make(chan error, 1)
		go func() { done <- Serve(context.Background(), inR, outW, nil) }()
		if err := WriteRequest(inW, first); err != nil {
			t.Fatal(err)
		}
		payload, err := ReadFrame(outR)
		if err != nil {
			t.Fatal(err)
		}
		response, err := DecodeResponse(payload)
		if err != nil || response.Type != TypeFatal {
			t.Fatalf("first request: %+v %v", response, err)
		}
		if err := <-done; err == nil {
			t.Fatal("missing hello accepted")
		}
		inW.Close()
		outR.Close()
	}
	h := startHarness(t, nil)
	h.send(t, Request{Type: TypeHello, Proto: Proto})
	if response := h.read(t); response.Type != TypeFatal {
		t.Fatalf("duplicate hello: %+v", response)
	}
	if err := h.wait(t); err == nil {
		t.Fatal("duplicate hello accepted")
	}
}

func TestServeStopsWithBlockedIO(t *testing.T) {
	for _, stop := range []string{"context", "EOF", "shutdown", "bad-frame"} {
		t.Run(stop, func(t *testing.T) {
			inR, inW := io.Pipe()
			outR, outW := io.Pipe()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			defer inW.Close()
			defer outR.Close()
			done := make(chan error, 1)
			go func() { done <- Serve(ctx, inR, outW, nil) }()
			// Never read ready: the writer is blocked in the pipe.
			if err := WriteRequest(inW, Request{Type: TypeHello, Proto: Proto}); err != nil {
				t.Fatal(err)
			}
			switch stop {
			case "context":
				cancel()
			case "EOF":
				inW.Close()
			case "shutdown":
				WriteRequest(inW, Request{Type: TypeShutdown})
			case "bad-frame":
				WriteFrame(inW, []byte(`{"type":"ping","extra":0}`))
			}
			select {
			case err := <-done:
				if stop == "bad-frame" && err == nil {
					t.Fatal("bad frame not fatal")
				}
			case <-time.After(2 * time.Second):
				t.Fatal("blocked writer prevented stop")
			}
		})
	}
	// Cancellation must also unblock the idle reader before any frame arrives.
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	defer inW.Close()
	defer outR.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, inR, outW, nil) }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("idle reader prevented stop")
	}
}

func TestServeTruncatedFrameIsFatal(t *testing.T) {
	for _, raw := range [][]byte{{0}, {0, 0, 0, 5}, {0, 0, 0, 5, '{'}} {
		inR, inW := io.Pipe()
		outR, outW := io.Pipe()
		done := make(chan error, 1)
		go func() { done <- Serve(context.Background(), inR, outW, nil) }()
		inW.Write(raw)
		inW.Close()
		payload, err := ReadFrame(outR)
		if err != nil {
			t.Fatal(err)
		}
		response, err := DecodeResponse(payload)
		if err != nil || response.Type != TypeFatal {
			t.Fatalf("truncated: %+v %v", response, err)
		}
		if err := <-done; err == nil {
			t.Fatal("truncated frame accepted as clean EOF")
		}
		outR.Close()
	}
}

func TestServeBatchValidatesAllBeforeExecutingAndSharesBudget(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	script := filepath.Join(dir, "git")
	testgit.Write(t, script, "#!/bin/sh\necho ran >> "+marker+"\nprintf 123456\n")
	if err := os.Chmod(script, 0700); err != nil {
		t.Fatal(err)
	}
	h := startHarness(t, NewEngine(script))
	h.send(t, Request{Type: TypeBatch, ID: 1, Dir: []byte(dir), Commands: [][][]byte{{[]byte("--version")}, {[]byte("status")}}})
	response := h.read(t)
	if response.Error == nil || response.Error.Code != "UnsafeOperation" {
		t.Fatalf("invalid batch: %+v", response)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("valid command ran before invalid command was checked")
	}
	// Each command exceeds its preassigned quarter of the aggregate ceiling.
	testgit.Write(t, script, "#!/bin/sh\necho ran >> "+marker+"\ndd if=/dev/zero bs=65536 count=40 2>/dev/null\n")
	h.send(t, Request{Type: TypeBatch, ID: 2, Dir: []byte(dir), Limit: MaxLimit, Commands: [][][]byte{{[]byte("--version")}, {[]byte("--version")}, {[]byte("--version")}, {[]byte("--version")}}})
	response = h.read(t)
	if response.Error == nil || response.Error.Code != "OutputLimitExceeded" {
		t.Fatalf("aggregate budget: %+v", response)
	}
	data, err := os.ReadFile(marker)
	if err != nil || strings.Count(string(data), "ran") != MaxBatchCommands {
		t.Fatalf("concurrent batch commands: %q %v", data, err)
	}
}

func TestServeInflightBoundAndCancellation(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "git")
	testgit.Write(t, script, "#!/bin/sh\nsleep 30\n")
	os.Chmod(script, 0700)
	h := startHarness(t, NewEngine(script))
	for i := 1; i <= MaxInflight+1; i++ {
		h.send(t, Request{Type: TypeExec, ID: uint64(i), Dir: []byte(dir), Args: [][]byte{[]byte("--version")}})
	}
	response := h.read(t)
	if response.Type != TypeFatal {
		t.Fatalf("inflight overflow: %+v", response)
	}
	if err := h.wait(t); err == nil {
		t.Fatal("inflight overflow accepted")
	}
	h = startHarness(t, NewEngine(script))
	h.send(t, Request{Type: TypeExec, ID: 1, Dir: []byte(dir), Args: [][]byte{[]byte("--version")}})
	h.send(t, Request{Type: TypeCancel, ID: 1})
	response = h.read(t)
	if response.Type != TypeResult || response.ID != 1 || response.Error == nil || response.Error.Code != "OperationCancelled" {
		t.Fatalf("cancel result: %+v", response)
	}
	h.send(t, Request{Type: TypePing})
	if response := h.read(t); response.Type != TypePong {
		t.Fatalf("duplicate terminal result: %+v", response)
	}
}

func TestServeBatchDeadlineIncludesReadQueue(t *testing.T) {
	engine := NewEngine("must-not-run")
	for i := 0; i < cap(engine.readSlots); i++ {
		engine.readSlots <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(engine.readSlots); i++ {
			<-engine.readSlots
		}
	}()
	h := startHarness(t, engine)
	h.send(t, Request{Type: TypeBatch, ID: 1, Dir: []byte("/tmp"), TimeoutMs: 30, Commands: [][][]byte{{[]byte("--version")}, {[]byte("--version")}}})
	response := h.read(t)
	if response.Error != nil || len(response.Results) != 2 {
		t.Fatalf("batch timeout: %+v", response)
	}
	for _, result := range response.Results {
		if result.Error == nil || result.Error.Code != "OperationTimedOut" {
			t.Fatalf("queue timeout: %+v", result)
		}
	}
}
