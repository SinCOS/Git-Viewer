package remotehelper

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"

	"gitviewer/internal/domain"
	"gitviewer/internal/gitargs"
)

// MaxInflight bounds queued and running requests, including blocked responses.
const MaxInflight = 64

// Serve owns the streams until it returns. Live streams must implement io.Closer
// so cancellation can interrupt blocked I/O; finite in-memory streams need not.
// Every exec/batch has one terminal response unless the connection fails.
func Serve(ctx context.Context, in io.Reader, out io.Writer, engine *Engine) error {
	input, err := interruptibleStream(in)
	if err != nil {
		return err
	}
	in = input.(io.Reader)
	if closer, ok := in.(io.Closer); ok {
		defer closer.Close()
	}
	output, err := interruptibleStream(out)
	if err != nil {
		return err
	}
	out = output.(io.Writer)
	if engine == nil {
		engine = NewEngine("")
	}
	ctx, cancel := context.WithCancel(ctx)
	var ioWorkers sync.WaitGroup
	s := &server{engine: engine, ctx: ctx, inflight: map[uint64]context.CancelFunc{}, writes: make(chan outgoing, MaxInflight), writeErr: make(chan error, 1)}
	defer func() {
		cancel()
		if closer, ok := in.(io.Closer); ok {
			_ = closer.Close()
		}
		if closer, ok := out.(io.Closer); ok {
			_ = closer.Close()
		}
		s.stopAll()
		ioWorkers.Wait()
	}()
	_, waitWriter := out.(io.Closer)
	if waitWriter {
		ioWorkers.Add(1)
	}
	go func() {
		if waitWriter {
			defer ioWorkers.Done()
		}
		for {
			select {
			case <-ctx.Done():
				return
			case frame := <-s.writes:
				if ctx.Err() != nil {
					return
				}
				err := WriteResponse(out, frame.response)
				frame.done <- err
				if err != nil {
					s.writeErr <- err
					return
				}
			}
		}
	}()
	type incoming struct {
		payload []byte
		err     error
	}
	frames := make(chan incoming)
	_, waitReader := in.(io.Closer)
	if waitReader {
		ioWorkers.Add(1)
	}
	go func() {
		if waitReader {
			defer ioWorkers.Done()
		}
		for {
			payload, err := ReadFrame(in)
			select {
			case frames <- incoming{payload, err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-s.writeErr:
			return err
		case frame := <-frames:
			if frame.err != nil {
				if errors.Is(frame.err, io.EOF) {
					return nil
				}
				return s.fatal(frame.err)
			}
			stop, err := s.handle(ctx, frame.payload)
			if err != nil {
				return err
			}
			if stop {
				return nil
			}
		}
	}
}

type outgoing struct {
	response Response
	done     chan error
}
type server struct {
	engine   *Engine
	ctx      context.Context
	writes   chan outgoing
	writeErr chan error
	hello    bool
	mu       sync.Mutex
	inflight map[uint64]context.CancelFunc
	workers  sync.WaitGroup
}

func (s *server) sendContext(ctx context.Context, response Response) error {
	if s.ctx.Err() != nil {
		return s.ctx.Err()
	}
	frame := outgoing{response, make(chan error, 1)}
	select {
	case s.writes <- frame:
	case <-ctx.Done():
		return ctx.Err()
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
	select {
	case err := <-frame.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-s.ctx.Done():
		return s.ctx.Err()
	}
}
func (s *server) send(response Response) error {
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	default:
	}
	select {
	case s.writes <- outgoing{response, make(chan error, 1)}:
		return nil
	default:
		select {
		case s.writeErr <- io.ErrShortWrite:
		default:
		}
		return io.ErrShortWrite
	}
}
func (s *server) fatal(err error) error {
	// A peer that stopped reading cannot prevent cancellation and process cleanup.
	ctx, cancel := context.WithTimeout(s.ctx, 250*time.Millisecond)
	defer cancel()
	_ = s.sendContext(ctx, Response{Type: TypeFatal, Error: &RemoteError{Code: string(domain.ProtocolError), Message: "protocol error"}})
	if err == nil {
		return fmt.Errorf("remote helper protocol error")
	}
	return err
}

func (s *server) handle(ctx context.Context, payload []byte) (bool, error) {
	request, err := DecodeRequest(payload)
	if err != nil {
		return false, s.fatal(err)
	}
	if !s.hello {
		if request.Type != TypeHello || request.Proto != Proto {
			return false, s.fatal(errors.New("expected protocol hello"))
		}
		s.hello = true
		return false, s.send(Response{Type: TypeReady, Proto: Proto, Platform: runtime.GOOS + "/" + runtime.GOARCH, PID: os.Getpid()})
	}
	switch request.Type {
	case TypeHello:
		return false, s.fatal(errors.New("duplicate hello"))
	case TypeExec, TypeBatch:
		if request.ID == 0 {
			return false, s.fatal(errors.New("missing request id"))
		}
		// Establish the deadline at admission, covering both scheduler queues and
		// every batch command. Reject overflow before converting milliseconds.
		if request.TimeoutMs < 0 || int64(request.TimeoutMs) > int64((1<<63-1)/time.Millisecond) {
			return false, s.fatal(errors.New("invalid timeout"))
		}
		timeout := time.Duration(request.TimeoutMs) * time.Millisecond
		if timeout == 0 {
			timeout = defaultReadTimeout
			if request.Type == TypeExec && gitargs.IsWriteArgs(byteArgs(request.Args)) {
				timeout = defaultWriteTime
			}
		}
		runCtx, cancel := context.WithTimeout(ctx, timeout)
		if !s.register(request.ID, cancel) {
			cancel()
			return false, s.fatal(errors.New("duplicate id or too many requests"))
		}
		s.workers.Add(1)
		go func() {
			defer s.workers.Done()
			defer cancel()
			defer s.unregister(request.ID)
			if request.Type == TypeExec {
				result := s.engine.Exec(runCtx, string(request.Dir), request.Limit, timeout, byteArgs(request.Args), request.Input)
				_ = s.sendContext(s.ctx, Response{Type: TypeResult, ID: request.ID, Exit: result.Exit, Stdout: result.Stdout, Stderr: result.Stderr, Error: result.Error})
			} else {
				s.runBatch(runCtx, request)
			}
		}()
		return false, nil
	case TypeCancel:
		if request.ID == 0 {
			return false, s.fatal(errors.New("missing cancel id"))
		}
		s.mu.Lock()
		cancel := s.inflight[request.ID]
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		return false, nil
	case TypePing:
		return false, s.send(Response{Type: TypePong})
	case TypeShutdown:
		return true, nil
	}
	return false, s.fatal(errors.New("unknown request type"))
}

func byteArgs(raw [][]byte) []string {
	args := make([]string, len(raw))
	for i := range raw {
		args[i] = string(raw[i])
	}
	return args
}
func (s *server) register(id uint64, cancel context.CancelFunc) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.inflight[id]; exists || len(s.inflight) >= MaxInflight {
		return false
	}
	s.inflight[id] = cancel
	return true
}
func (s *server) unregister(id uint64) { s.mu.Lock(); delete(s.inflight, id); s.mu.Unlock() }
func (s *server) stopAll() {
	s.mu.Lock()
	for _, cancel := range s.inflight {
		cancel()
	}
	s.mu.Unlock()
	s.workers.Wait()
}
func (s *server) runBatch(ctx context.Context, request Request) {
	send := func(results []CommandResult, failure *RemoteError) {
		_ = s.sendContext(s.ctx, Response{Type: TypeBatchResult, ID: request.ID, Results: results, Error: failure})
	}
	limit, failure := normalizeLimit(request.Limit)
	if failure != nil {
		send(nil, failure)
		return
	}
	if !validDir(string(request.Dir)) || len(request.Commands) == 0 || len(request.Commands) > MaxBatchCommands {
		send(nil, unsafeError("Invalid batch"))
		return
	}
	commands := make([][]string, len(request.Commands))
	for i, raw := range request.Commands {
		commands[i] = byteArgs(raw)
		if Validate(commands[i]) != nil || gitargs.IsWriteArgs(commands[i]) {
			send(nil, unsafeError("Batch commands must be fixed read-only commands"))
			return
		}
	}
	// Partition the aggregate ceiling before starting concurrent commands. This
	// preserves the standard 2 MiB per-command budget for batches of up to four;
	// larger requested budgets are conservatively capped to 8 MiB / count.
	limit = min(limit, MaxBatchOutput/len(commands))
	results := make([]CommandResult, len(commands))
	var workers sync.WaitGroup
	for i, args := range commands {
		workers.Add(1)
		go func(i int, args []string) {
			defer workers.Done()
			results[i] = s.engine.Exec(ctx, string(request.Dir), limit, defaultReadTimeout, args, nil)
		}(i, args)
	}
	workers.Wait()
	for _, result := range results {
		if result.Error != nil && result.Error.Code == string(domain.OutputLimitExceeded) {
			send(nil, result.Error)
			return
		}
	}
	send(results, nil)
}
