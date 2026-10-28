package bridge

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitviewer/internal/infrastructure/ssh"
)

const maxRequestBytes = 1 << 20

type Config struct {
	SSHPath        string
	ConfigPath     string
	KnownHostsFile string
	// RemoteConfigDir overrides ~/.config/gitviewer on the remote host.
	// Tests use an isolated temporary directory.
	RemoteConfigDir string
}

type OpenRequest struct {
	Host       string   `json:"host"`
	WorkingDir string   `json:"cwd"`
	Argv       []string `json:"argv"`
}

type Handler func(context.Context, OpenRequest) (<-chan struct{}, error)

type Session struct {
	host         string
	token        string
	localSocket  string
	remoteSocket string
	cfg          Config
	cmd          *exec.Cmd
	stderr       bytes.Buffer
	server       *http.Server
	listener     net.Listener
	done         chan struct{}
	exited       chan struct{}
	closeOnce    sync.Once
}

type wireRequest struct {
	Token      string   `json:"token"`
	Host       string   `json:"host"`
	WorkingDir string   `json:"cwd"`
	Argv       []string `json:"argv"`
}

type wireResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

func randomHex(n int) (string, error) {
	data := make([]byte, n)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func localSocketPath() (string, error) {
	id, err := randomHex(8)
	if err != nil {
		return "", err
	}
	name := "gv-" + strconv.Itoa(os.Getuid()) + "-" + id + ".sock"
	for _, dir := range []string{os.Getenv("TMPDIR"), "/tmp"} {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, name)
		if len(candidate) < 100 {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("cannot allocate a short Unix socket path")
}

func (c Config) executable() string {
	if c.SSHPath != "" {
		return c.SSHPath
	}
	return "ssh"
}

func (c Config) executor() ssh.Executor {
	return ssh.Executor{SSHPath: c.executable(), ConfigPath: c.ConfigPath, KnownHostsFile: c.KnownHostsFile}
}

func Start(ctx context.Context, cfg Config, host string, handler Handler) (*Session, error) {
	if !ssh.ValidHost(host) {
		return nil, errors.New("invalid SSH host")
	}
	token, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	localSocket, err := localSocketPath()
	if err != nil {
		return nil, err
	}
	_ = os.Remove(localSocket)
	listener, err := net.Listen("unix", localSocket)
	if err != nil {
		return nil, err
	}
	remoteSocket := "/tmp/gitviewer-" + token[:20] + ".sock"
	s := &Session{
		host: host, token: token, localSocket: localSocket, remoteSocket: remoteSocket,
		cfg: cfg, listener: listener, done: make(chan struct{}), exited: make(chan struct{}),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/open", s.handle(handler))
	s.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		err := s.server.Serve(listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.closeLocal()
		}
	}()

	args := cfg.executor().ForwardArgs(localSocket, remoteSocket)
	args = append(args, host)
	s.cmd = exec.CommandContext(context.Background(), cfg.executable(), args...)
	s.cmd.Stderr = &s.stderr
	if err := s.cmd.Start(); err != nil {
		s.Close()
		return nil, err
	}
	go func() {
		_ = s.cmd.Wait()
		close(s.exited)
		s.closeLocal()
	}()
	select {
	case <-ctx.Done():
		s.Close()
		return nil, ctx.Err()
	case <-s.exited:
		s.Close()
		detail := strings.TrimSpace(s.stderr.String())
		if detail == "" {
			detail = "ssh reverse forwarding failed"
		}
		return nil, errors.New(detail)
	case <-time.After(250 * time.Millisecond):
	}
	if err := s.writeEndpoint(ctx); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func (s *Session) EndpointPath() string {
	if s.cfg.RemoteConfigDir != "" {
		return filepath.Join(s.cfg.RemoteConfigDir, "endpoint")
	}
	return "~/.config/gitviewer/endpoint"
}
func (s *Session) Host() string  { return s.host }
func (s *Session) Token() string { return s.token }

// Done is closed when the reverse-forwarding SSH process exits.
func (s *Session) Done() <-chan struct{} { return s.exited }

func (s *Session) writeEndpoint(ctx context.Context) error {
	dir := `"${XDG_CONFIG_HOME:-$HOME/.config}/gitviewer"`
	if s.cfg.RemoteConfigDir != "" {
		dir = shellQuote(s.cfg.RemoteConfigDir)
	}
	script := strings.Join([]string{
		"umask 077",
		"mkdir -p " + dir,
		"tmp=" + dir + `/endpoint.tmp.$$`,
		"printf '%s\\n' " + shellQuote("socket="+s.remoteSocket) + " " + shellQuote("token="+s.token) + " " + shellQuote("host="+s.host) + ` > "$tmp"`,
		`chmod 600 "$tmp"`,
		`mv "$tmp" ` + dir + `/endpoint`,
	}, "; ")
	encoded, err := ssh.Encode([]string{"sh", "-c", script})
	if err != nil {
		return err
	}
	args := s.cfg.executor().ConnectionArgs()
	args = append(args, s.host, encoded)
	runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, s.cfg.executable(), args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("cannot install bridge endpoint: %s", strings.TrimSpace(stderr.String()))
	}
	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func (s *Session) handle(handler Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxRequestBytes+1))
		if err != nil || len(body) > maxRequestBytes {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		var request wireRequest
		if r.Header.Get("Content-Type") == "application/x-gitviewer-argv" {
			parts := bytes.Split(body, []byte{0})
			if len(parts) < 3 || len(parts[len(parts)-1]) != 0 {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
			parts = parts[:len(parts)-1]
			request.Token = string(parts[0])
			request.Host = string(parts[1])
			request.WorkingDir = string(parts[2])
			for _, part := range parts[3:] {
				request.Argv = append(request.Argv, string(part))
			}
		} else {
			decoder := json.NewDecoder(bytes.NewReader(body))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&request) != nil {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
		}
		if subtle.ConstantTimeCompare([]byte(request.Token), []byte(s.token)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if request.Host != s.host || !ssh.ValidHost(request.Host) || !path.IsAbs(request.WorkingDir) ||
			strings.ContainsRune(request.WorkingDir, 0) || len(request.Argv) > 256 {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		for _, arg := range request.Argv {
			if strings.ContainsRune(arg, 0) || len(arg) > 32*1024 {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
		}
		wait, err := handler(r.Context(), OpenRequest{Host: request.Host, WorkingDir: request.WorkingDir, Argv: request.Argv})
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(wireResponse{Status: "error", Message: err.Error()})
			return
		}
		if wait != nil {
			select {
			case <-wait:
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(wireResponse{Status: "opened"})
	}
}

func (s *Session) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.done)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		dir := `"${XDG_CONFIG_HOME:-$HOME/.config}/gitviewer"`
		if s.cfg.RemoteConfigDir != "" {
			dir = shellQuote(s.cfg.RemoteConfigDir)
		}
		script := "rm -f " + dir + `/endpoint`
		encoded, encodeErr := ssh.Encode([]string{"sh", "-c", script})
		if encodeErr == nil {
			args := s.cfg.executor().ConnectionArgs()
			args = append(args, s.host, encoded)
			_ = exec.CommandContext(ctx, s.cfg.executable(), args...).Run()
		}
		if s.server != nil {
			_ = s.server.Shutdown(ctx)
		}
		if s.cmd != nil && s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
		s.closeLocal()
	})
	return err
}

func (s *Session) closeLocal() {
	if s.listener != nil {
		_ = s.listener.Close()
	}
	if s.localSocket != "" {
		_ = os.Remove(s.localSocket)
	}
}
