package askpass

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	HelperFlag = "--gitviewer-askpass"
	maxMessage = 64 * 1024
)

type Request struct {
	Host   string `json:"host"`
	Prompt string `json:"prompt"`
}

type PromptFunc func(context.Context, Request) (string, error)

type wireRequest struct {
	Token  string `json:"token"`
	Host   string `json:"host"`
	Prompt string `json:"prompt"`
}

type wireResponse struct {
	Password string `json:"password,omitempty"`
	Error    string `json:"error,omitempty"`
}

type Broker struct {
	socket string
	token  string
	script string

	listener net.Listener
	prompt   PromptFunc
	done     chan struct{}
	close    sync.Once
	workers  sync.WaitGroup
	promptMu sync.Mutex
	cacheMu  sync.Mutex
	cache    map[string]string

	oldEnv map[string]*string
}

func randomValue(bytes int) (string, error) {
	data := make([]byte, bytes)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func socketPath() (string, error) {
	id, err := randomValue(8)
	if err != nil {
		return "", err
	}
	name := "gv-askpass-" + id + ".sock"
	for _, dir := range []string{os.Getenv("TMPDIR"), "/tmp"} {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, name)
		if len(candidate) < 100 {
			return candidate, nil
		}
	}
	return "", errors.New("cannot allocate askpass socket path")
}

func Start(prompt PromptFunc) (*Broker, error) {
	if prompt == nil {
		return nil, errors.New("askpass prompt callback is required")
	}
	token, err := randomValue(32)
	if err != nil {
		return nil, err
	}
	socket, err := socketPath()
	if err != nil {
		return nil, err
	}
	_ = os.Remove(socket)
	listener, err := net.Listen("unix", socket)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socket, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(socket)
		return nil, err
	}
	b := &Broker{
		socket: socket, token: token, listener: listener, prompt: prompt,
		done: make(chan struct{}), oldEnv: map[string]*string{}, cache: map[string]string{},
	}
	go b.serve()
	return b, nil
}

func (b *Broker) serve() {
	for {
		conn, err := b.listener.Accept()
		if err != nil {
			select {
			case <-b.done:
				return
			default:
			}
			continue
		}
		b.workers.Add(1)
		go func() {
			defer b.workers.Done()
			b.handle(conn)
		}()
	}
}

func (b *Broker) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Minute))
	reader := bufio.NewReaderSize(conn, maxMessage)
	line, err := reader.ReadBytes('\n')
	if err != nil || len(line) > maxMessage {
		return
	}
	var request wireRequest
	if json.Unmarshal(line, &request) != nil {
		b.writeResponse(conn, wireResponse{Error: "invalid askpass request"})
		return
	}
	if subtle.ConstantTimeCompare([]byte(request.Token), []byte(b.token)) != 1 {
		b.writeResponse(conn, wireResponse{Error: "askpass request rejected"})
		return
	}
	if strings.ContainsRune(request.Prompt, 0) || len(request.Prompt) > 16*1024 {
		b.writeResponse(conn, wireResponse{Error: "invalid askpass prompt"})
		return
	}
	if b.cacheable(request.Prompt) {
		if password := b.cachedPassword(request.Host); password != "" {
			b.writeResponse(conn, wireResponse{Password: password})
			return
		}
	}
	b.promptMu.Lock()
	if b.cacheable(request.Prompt) {
		if password := b.cachedPassword(request.Host); password != "" {
			b.promptMu.Unlock()
			b.writeResponse(conn, wireResponse{Password: password})
			return
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	password, promptErr := b.prompt(ctx, Request{Host: request.Host, Prompt: request.Prompt})
	cancel()
	b.promptMu.Unlock()
	if promptErr != nil {
		b.writeResponse(conn, wireResponse{Error: "password prompt cancelled"})
		return
	}
	if strings.ContainsAny(password, "\x00\r\n") {
		b.writeResponse(conn, wireResponse{Error: "invalid password response"})
		return
	}
	if b.cacheable(request.Prompt) && request.Host != "" {
		b.cacheMu.Lock()
		b.cache[request.Host] = password
		b.cacheMu.Unlock()
	}
	b.writeResponse(conn, wireResponse{Password: password})
}

func (b *Broker) cacheable(prompt string) bool {
	lower := strings.ToLower(prompt)
	return strings.Contains(lower, "password") &&
		!strings.Contains(lower, "verification") &&
		!strings.Contains(lower, "passcode") &&
		!strings.Contains(lower, "one-time") &&
		!strings.Contains(lower, "otp")
}

func (b *Broker) cachedPassword(host string) string {
	b.cacheMu.Lock()
	defer b.cacheMu.Unlock()
	return b.cache[host]
}

func (b *Broker) clearHost(host string) {
	b.cacheMu.Lock()
	delete(b.cache, host)
	b.cacheMu.Unlock()
}

func (b *Broker) writeResponse(conn net.Conn, response wireResponse) {
	data, err := json.Marshal(response)
	if err != nil {
		return
	}
	_, _ = conn.Write(append(data, '\n'))
}

// InstallEnvironment makes system OpenSSH invoke this executable through a
// private wrapper script when it needs a password or keyboard-interactive
// response. The parent app is not marked as a helper because the flag is only
// added by the wrapper.
func (b *Broker) InstallEnvironment() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	script := filepath.Join(filepath.Dir(b.socket), "gv-askpass-"+b.token[:12]+".sh")
	content := "#!/bin/sh\nexec " + shellQuote(executable) + " " + HelperFlag + " \"$@\"\n"
	if err := os.WriteFile(script, []byte(content), 0o700); err != nil {
		return err
	}
	b.script = script
	values := map[string]string{
		"SSH_ASKPASS":             script,
		"SSH_ASKPASS_REQUIRE":     "force",
		"GITVIEWER_ASKPASS_SOCK":  b.socket,
		"GITVIEWER_ASKPASS_TOKEN": b.token,
	}
	for key, value := range values {
		if old, ok := os.LookupEnv(key); ok {
			copy := old
			b.oldEnv[key] = &copy
		} else {
			b.oldEnv[key] = nil
		}
		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}
	return nil
}

func (b *Broker) Close() {
	b.close.Do(func() {
		close(b.done)
		if b.listener != nil {
			_ = b.listener.Close()
		}
		b.workers.Wait()
		if b.socket != "" {
			_ = os.Remove(b.socket)
		}
		if b.script != "" {
			_ = os.Remove(b.script)
		}
		for key, old := range b.oldEnv {
			if old == nil {
				_ = os.Unsetenv(key)
			} else {
				_ = os.Setenv(key, *old)
			}
		}
	})
}

var (
	defaultMu     sync.RWMutex
	defaultBroker *Broker
)

func SetDefault(b *Broker) {
	defaultMu.Lock()
	defaultBroker = b
	defaultMu.Unlock()
}

// ClearHost forgets a cached password after an authentication failure. This
// keeps a wrong password from being reused across the separate SFTP, Git and
// bridge SSH processes.
func ClearHost(host string) {
	defaultMu.RLock()
	b := defaultBroker
	defaultMu.RUnlock()
	if b != nil {
		b.clearHost(host)
	}
}

func HostEnvironment(host string) []string {
	return []string{"GITVIEWER_ASKPASS_HOST=" + host}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func IsHelper(args []string) bool {
	return len(args) > 1 && args[1] == HelperFlag
}

// RunHelper is called by the private wrapper spawned by OpenSSH. It writes
// only the password to stdout so the prompt and credentials never appear in
// process arguments or logs.
func RunHelper(args []string) int {
	prompt := helperPrompt(args)
	socket := os.Getenv("GITVIEWER_ASKPASS_SOCK")
	token := os.Getenv("GITVIEWER_ASKPASS_TOKEN")
	if socket == "" || token == "" {
		return 1
	}
	conn, err := net.DialTimeout("unix", socket, 5*time.Second)
	if err != nil {
		return 1
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Minute))
	request, err := json.Marshal(wireRequest{
		Token: token, Host: os.Getenv("GITVIEWER_ASKPASS_HOST"), Prompt: prompt,
	})
	if err != nil {
		return 1
	}
	if _, err := conn.Write(append(request, '\n')); err != nil {
		return 1
	}
	line, err := bufio.NewReaderSize(conn, maxMessage).ReadBytes('\n')
	if err != nil || len(line) > maxMessage {
		return 1
	}
	var response wireResponse
	if json.Unmarshal(line, &response) != nil || response.Error != "" {
		return 1
	}
	if _, err := fmt.Fprintln(os.Stdout, response.Password); err != nil {
		return 1
	}
	return 0
}

func helperPrompt(args []string) string {
	if len(args) > 0 && args[0] == HelperFlag {
		args = args[1:]
	}
	return strings.Join(args, " ")
}
