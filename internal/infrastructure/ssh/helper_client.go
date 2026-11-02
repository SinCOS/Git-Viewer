package ssh

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"gitviewer/internal/domain"
	"gitviewer/internal/gitargs"
	"gitviewer/internal/remotehelper"
)

const (
	helperHelloTimeout = 10 * time.Second
	helperCancelGrace  = 3 * time.Second
	helperCloseTimeout = 5 * time.Second
)

type helperPending struct {
	response chan helperOutcome
	batch    int // zero for exec
}
type helperOutcome struct {
	result  remotehelper.CommandResult
	results []remotehelper.CommandResult
	err     error
}

// HelperClient owns one SSH channel and routes out-of-order terminal frames.
// A failed/partial frame write poisons the channel; it never resends a request.
type HelperClient struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    *connectionOutput
	remotePID int
	writeSlot chan struct{}
	sendSlot  chan struct{}
	pingSlot  chan struct{}
	pong      chan struct{}
	ready     chan remotehelper.Response
	mu        sync.Mutex
	pending   map[uint64]helperPending
	nextID    uint64
	failure   error
	done      chan struct{}
	readDone  chan struct{}
	waitDone  chan struct{}
	waitErr   error // published by waitDone
	stopOnce  sync.Once
	closeOnce sync.Once
}

func startHelperClient(ctx context.Context, cmd *exec.Cmd) (*HelperClient, error) {
	if err := ctx.Err(); err != nil {
		return nil, contextError(err)
	}
	// Own stdout ourselves: exec.Wait must not close a StdoutPipe while its
	// reader is still consuming the final terminal frame.
	output, writer := io.Pipe()
	cmd.Stdout = writer
	stdin, err := cmd.StdinPipe()
	if err != nil {
		output.Close()
		writer.Close()
		return nil, &domain.GitError{Code: domain.RemoteUnavailable, Cause: err}
	}
	c := &HelperClient{cmd: cmd, stdin: stdin, stdout: output, stderr: &connectionOutput{},
		pending: map[uint64]helperPending{}, done: make(chan struct{}), readDone: make(chan struct{}), waitDone: make(chan struct{}),
		ready: make(chan remotehelper.Response, 1), pong: make(chan struct{}, 1), writeSlot: make(chan struct{}, 1), sendSlot: make(chan struct{}, 1), pingSlot: make(chan struct{}, 1)}
	cmd.Stderr = c.stderr
	cmd.WaitDelay = 250 * time.Millisecond
	if err := cmd.Start(); err != nil {
		stdin.Close()
		output.Close()
		writer.Close()
		return nil, &domain.GitError{Code: domain.RemoteUnavailable, Cause: err}
	}
	go c.readLoop()
	go func() { c.waitErr = cmd.Wait(); writer.CloseWithError(c.waitErr); close(c.waitDone) }()
	helloCtx, cancel := context.WithTimeout(ctx, helperHelloTimeout)
	defer cancel()
	if err := c.send(helloCtx, remotehelper.Request{Type: remotehelper.TypeHello, Proto: remotehelper.Proto}); err != nil {
		c.abort(err)
		return nil, err
	}
	select {
	case response := <-c.ready:
		if response.Type != remotehelper.TypeReady || response.Proto != remotehelper.Proto || response.PID <= 0 || response.Platform == "" {
			err := &domain.GitError{Code: domain.ProtocolError}
			c.abort(err)
			return nil, err
		}
		c.remotePID = response.PID
	case <-helloCtx.Done():
		c.abort(contextError(helloCtx.Err()))
		return nil, contextError(helloCtx.Err())
	case <-c.done:
		<-c.waitDone
		return nil, c.startupError()
	}
	return c, nil
}

func (c *HelperClient) startupError() error {
	text := strings.ToLower(string(c.stderr.bytes()))
	exit := -1
	if c.cmd.ProcessState != nil {
		exit = c.cmd.ProcessState.ExitCode()
	}
	if exit == 126 || exit == 127 || strings.Contains(text, "exec format") || strings.Contains(text, "cannot execute") || (exit != 255 && strings.Contains(text, "permission denied")) {
		return ErrHelperPermanent
	}
	if exit == 255 || strings.Contains(text, "host key") || strings.Contains(text, "remote host identification") {
		_, err := classifyResult(domain.GitResult{ExitCode: exit, Stderr: c.stderr.bytes()}, c.waitErr, "")
		if err != nil {
			return err
		}
	}
	return c.failureError()
}

func (c *HelperClient) failureError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failure != nil {
		return c.failure
	}
	return &domain.GitError{Code: domain.RemoteUnavailable}
}
func (c *HelperClient) failAll(err error) {
	if err == nil {
		err = &domain.GitError{Code: domain.RemoteUnavailable}
	}
	c.stopOnce.Do(func() {
		c.mu.Lock()
		c.failure = err
		pending := c.pending
		c.pending = map[uint64]helperPending{}
		c.mu.Unlock()
		for _, p := range pending {
			p.response <- helperOutcome{err: err}
		}
		close(c.done)
	})
}
func (c *HelperClient) abort(err error) {
	c.failAll(err)
	_ = c.stdin.Close()
	_ = c.stdout.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
}
func (c *HelperClient) readLoop() {
	defer close(c.readDone)
	handshaken := false
	for {
		payload, err := remotehelper.ReadFrame(c.stdout)
		if err != nil {
			c.abort(&domain.GitError{Code: domain.RemoteUnavailable, Cause: err})
			return
		}
		response, err := remotehelper.DecodeResponse(payload)
		if err != nil {
			c.abort(&domain.GitError{Code: domain.ProtocolError})
			return
		}
		if !handshaken {
			if response.Type != remotehelper.TypeReady && response.Type != remotehelper.TypeFatal {
				c.abort(&domain.GitError{Code: domain.ProtocolError})
				return
			}
			c.ready <- response
			handshaken = true
			if response.Type == remotehelper.TypeFatal {
				c.abort(&domain.GitError{Code: domain.ProtocolError})
				return
			}
			continue
		}
		switch response.Type {
		case remotehelper.TypePong:
			select {
			case c.pong <- struct{}{}:
			default:
			}
		case remotehelper.TypeResult, remotehelper.TypeBatchResult:
			c.mu.Lock()
			p, ok := c.pending[response.ID]
			if ok {
				delete(c.pending, response.ID)
			}
			c.mu.Unlock()
			// Unknown/duplicate terminal IDs indicate stream corruption. Cancellation
			// never drops pending requests while their connection remains usable.
			if !ok || response.ID == 0 || (p.batch == 0) != (response.Type == remotehelper.TypeResult) || (p.batch > 0 && response.Error == nil && len(response.Results) != p.batch) {
				if ok {
					p.response <- helperOutcome{err: &domain.GitError{Code: domain.ProtocolError}}
				}
				c.abort(&domain.GitError{Code: domain.ProtocolError})
				return
			}
			if !validRemoteError(response.Error) {
				p.response <- helperOutcome{err: &domain.GitError{Code: domain.ProtocolError}}
				c.abort(&domain.GitError{Code: domain.ProtocolError})
				return
			}
			for _, r := range response.Results {
				if !validRemoteError(r.Error) {
					err = &domain.GitError{Code: domain.ProtocolError}
					break
				}
			}
			if err != nil {
				p.response <- helperOutcome{err: err}
				c.abort(err)
				return
			}
			outcome := helperOutcome{result: remotehelper.CommandResult{Exit: response.Exit, Stdout: response.Stdout, Stderr: response.Stderr, Error: response.Error}, results: response.Results}
			if response.Type == remotehelper.TypeBatchResult {
				outcome.err = helperResponseError(response.Error)
			}
			p.response <- outcome
		case remotehelper.TypeFatal:
			c.abort(&domain.GitError{Code: domain.ProtocolError})
			return
		default:
			c.abort(&domain.GitError{Code: domain.ProtocolError})
			return
		}
	}
}
func validRemoteError(err *remotehelper.RemoteError) bool {
	if err == nil {
		return true
	}
	switch domain.ErrorCode(err.Code) {
	case domain.UnsafeOperation, domain.InvalidRepository, domain.OperationCancelled, domain.OperationTimedOut, domain.OutputLimitExceeded, domain.GitUnavailable, domain.CommandFailed, domain.ProtocolError:
		return true
	}
	return false
}
func helperResponseError(err *remotehelper.RemoteError) error {
	if err == nil {
		return nil
	}
	return &domain.GitError{Code: domain.ErrorCode(err.Code)}
}

func (c *HelperClient) send(ctx context.Context, request remotehelper.Request) error {
	payload, err := remotehelper.Encode(request)
	if err != nil || len(payload) > remotehelper.MaxFrame {
		return &domain.GitError{Code: domain.UnsafeOperation}
	}
	select {
	case c.sendSlot <- struct{}{}:
		defer func() { <-c.sendSlot }()
	case <-ctx.Done():
		return contextError(ctx.Err())
	case <-c.done:
		return c.failureError()
	}
	if err := ctx.Err(); err != nil {
		return contextError(err)
	}
	select {
	case <-c.done:
		return c.failureError()
	default:
	}
	// Closing stdin interrupts a blocked OS pipe write. Once transmission has
	// started cancellation makes the connection unusable, including other reads.
	stop := context.AfterFunc(ctx, func() { c.abort(&domain.GitError{Code: domain.RemoteUnavailable}) })
	err = remotehelper.WriteFrame(c.stdin, payload)
	if !stop() || err != nil {
		failure := &domain.GitError{Code: domain.RemoteUnavailable, Cause: err}
		c.abort(failure)
		return failure
	}
	return nil
}
func (c *HelperClient) register(batch int) (uint64, chan helperOutcome, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failure != nil {
		return 0, nil, c.failure
	}
	c.nextID++
	id := c.nextID
	response := make(chan helperOutcome, 1)
	c.pending[id] = helperPending{response: response, batch: batch}
	return id, response, nil
}
func (c *HelperClient) unregister(id uint64)  { c.mu.Lock(); delete(c.pending, id); c.mu.Unlock() }
func (c *HelperClient) Done() <-chan struct{} { return c.done }
func (c *HelperClient) Alive() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

func (c *HelperClient) Exec(ctx context.Context, dir string, limit int, timeout time.Duration, args []string, input []byte) (domain.GitResult, error) {
	invalid := domain.GitResult{ExitCode: -1}
	if err := ctx.Err(); err != nil {
		return invalid, contextError(err)
	}
	if gitargs.IsWriteArgs(args) {
		select {
		case c.writeSlot <- struct{}{}:
			defer func() { <-c.writeSlot }()
		case <-ctx.Done():
			return invalid, contextError(ctx.Err())
		case <-c.done:
			return invalid, c.failureError()
		}
	}
	if err := ctx.Err(); err != nil {
		return invalid, contextError(err)
	}
	id, pending, err := c.register(0)
	if err != nil {
		return invalid, err
	}
	request := remotehelper.Request{Type: remotehelper.TypeExec, ID: id, Dir: []byte(dir), Limit: limit, TimeoutMs: durationMillis(timeout), Args: byteArgs(args), Input: input}
	if err := c.send(ctx, request); err != nil {
		c.unregister(id)
		return invalid, err
	}
	outcome, err := c.await(ctx, id, pending)
	if err != nil {
		return invalid, err
	}
	if outcome.err != nil {
		return invalid, outcome.err
	}
	result := outcome.result
	output := domain.GitResult{ExitCode: result.Exit, Stdout: result.Stdout, Stderr: result.Stderr}
	if result.Error != nil {
		// The engine reports every ordinary nonzero Git exit as CommandFailed.
		// Classify its captured stderr like the CLI transport so callers retain
		// PermissionDenied and the fixed SSH diagnostic mappings.
		if result.Error.Code == string(domain.CommandFailed) {
			return classifyResult(output, helperResponseError(result.Error), "")
		}
		return output, helperResponseError(result.Error)
	}
	if result.Exit != 0 {
		return classifyResult(output, &domain.GitError{Code: domain.CommandFailed}, "")
	}
	return output, nil
}
func (c *HelperClient) Batch(ctx context.Context, dir string, limit int, timeout time.Duration, commands [][]string) ([]domain.GitResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, contextError(err)
	}
	if len(commands) < 1 || len(commands) > remotehelper.MaxBatchCommands {
		return nil, &domain.GitError{Code: domain.UnsafeOperation}
	}
	id, pending, err := c.register(len(commands))
	if err != nil {
		return nil, err
	}
	request := remotehelper.Request{Type: remotehelper.TypeBatch, ID: id, Dir: []byte(dir), Limit: limit, TimeoutMs: durationMillis(timeout)}
	for _, command := range commands {
		request.Commands = append(request.Commands, byteArgs(command))
	}
	if err := c.send(ctx, request); err != nil {
		c.unregister(id)
		return nil, err
	}
	outcome, err := c.await(ctx, id, pending)
	if err != nil {
		return nil, err
	}
	if outcome.err != nil {
		return nil, outcome.err
	}
	results := make([]domain.GitResult, len(outcome.results))
	for i, result := range outcome.results {
		results[i] = domain.GitResult{ExitCode: result.Exit, Stdout: result.Stdout, Stderr: result.Stderr}
		if result.Error != nil {
			if result.Error.Code == string(domain.CommandFailed) {
				_, err := classifyResult(domain.GitResult{ExitCode: result.Exit, Stdout: result.Stdout, Stderr: result.Stderr}, helperResponseError(result.Error), "")
				return nil, err
			}
			return nil, helperResponseError(result.Error)
		}
		if result.Exit != 0 {
			output := domain.GitResult{ExitCode: result.Exit, Stdout: result.Stdout, Stderr: result.Stderr}
			_, err := classifyResult(output, &domain.GitError{Code: domain.CommandFailed}, "")
			return nil, err
		}
	}
	return results, nil
}
func (c *HelperClient) await(ctx context.Context, id uint64, pending chan helperOutcome) (helperOutcome, error) {
	select {
	case outcome := <-pending:
		return outcome, nil
	case <-ctx.Done():
		cancelCtx, cancel := context.WithTimeout(context.Background(), helperCancelGrace)
		defer cancel()
		if err := c.send(cancelCtx, remotehelper.Request{Type: remotehelper.TypeCancel, ID: id}); err != nil {
			return helperOutcome{}, contextError(ctx.Err())
		}
		select {
		case outcome := <-pending:
			return outcome, nil
		case <-cancelCtx.Done():
			c.abort(&domain.GitError{Code: domain.RemoteUnavailable})
			return helperOutcome{}, contextError(ctx.Err())
		case <-c.done:
			return helperOutcome{}, c.failureError()
		}
	case <-c.done:
		// failAll delivers the specific failure before closing done.
		select {
		case outcome := <-pending:
			return outcome, nil
		default:
			return helperOutcome{}, c.failureError()
		}
	}
}
func (c *HelperClient) Ping(ctx context.Context) error {
	select {
	case c.pingSlot <- struct{}{}:
		defer func() { <-c.pingSlot }()
	case <-ctx.Done():
		return contextError(ctx.Err())
	case <-c.done:
		return c.failureError()
	}
	select {
	case <-c.pong:
	default:
	}
	if err := c.send(ctx, remotehelper.Request{Type: remotehelper.TypePing}); err != nil {
		return err
	}
	select {
	case <-c.pong:
		return nil
	case <-ctx.Done():
		return contextError(ctx.Err())
	case <-c.done:
		return c.failureError()
	}
}
func (c *HelperClient) Close() error {
	c.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), helperCloseTimeout)
		defer cancel()
		_ = c.send(ctx, remotehelper.Request{Type: remotehelper.TypeShutdown})
		_ = c.stdin.Close()
		select {
		case <-c.waitDone:
		case <-ctx.Done():
			c.abort(&domain.GitError{Code: domain.RemoteUnavailable})
		}
		c.abort(&domain.GitError{Code: domain.RemoteUnavailable})
		<-c.waitDone
		<-c.readDone
	})
	return nil
}
func durationMillis(timeout time.Duration) int {
	if timeout <= 0 {
		return 0
	}
	return int(max(1, min(timeout, 2*time.Minute)/time.Millisecond))
}
func byteArgs(args []string) [][]byte {
	result := make([][]byte, len(args))
	for i, arg := range args {
		result[i] = []byte(arg)
	}
	return result
}
