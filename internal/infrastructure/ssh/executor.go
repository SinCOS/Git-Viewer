package ssh

import (
	"context"
	"errors"
	"path"
	"regexp"
	"strings"
	"time"

	"gitviewer/internal/askpass"
	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
	"gitviewer/internal/infrastructure/process"
)

var hostPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

// ValidHost reports whether host is a safe OpenSSH Host alias.
func ValidHost(host string) bool { return hostPattern.MatchString(host) }

// Encode quotes each POSIX-shell argument independently. OpenSSH remote exec
// crosses a shell boundary, even when its own arguments were passed as argv.
func Encode(args []string) (string, error) {
	quoted := make([]string, len(args))
	for i, arg := range args {
		if strings.ContainsRune(arg, 0) {
			return "", &domain.GitError{Code: domain.UnsafeOperation}
		}
		quoted[i] = "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
	}
	return strings.Join(quoted, " "), nil
}

type Executor struct {
	SSHPath        string
	ConfigPath     string
	KnownHostsFile string
	OutputLimit    int
	AllowPassword  bool
	// Connections optionally scopes transport lifetime (tests, embedded users).
	// A nil pool uses the application-wide pool shared with SFTP.
	Connections *Connections
}

var _ domain.GitExecutor = Executor{}

func (e Executor) Execute(ctx context.Context, command domain.GitCommand) (domain.GitResult, error) {
	invalid := domain.GitResult{ExitCode: -1}
	repo := command.Repository
	if repo.Repository.Location != "ssh" || !hostPattern.MatchString(repo.SSHHost) || !path.IsAbs(repo.Repository.Path) {
		return invalid, &domain.GitError{Code: domain.InvalidRepository}
	}
	gitArgs, err := gitexec.ReadArgs(command)
	if err != nil {
		return invalid, err
	}
	// Keep the legacy POC executor isolated for lifecycle compatibility.
	// Product reads use Reader; its controlled writes explicitly opt out of
	// connection multiplexing before post-operation verification.
	return e.runRemoteIsolated(ctx, repo.SSHHost, command.Timeout, e.OutputLimit, append([]string{"env", "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "git"}, gitArgs...), nil, true)
}

func (e Executor) runRemote(ctx context.Context, host string, timeout time.Duration, limit int, argv []string) (domain.GitResult, error) {
	return e.runRemoteInput(ctx, host, timeout, limit, argv, nil)
}
func (e Executor) runRemoteInput(ctx context.Context, host string, timeout time.Duration, limit int, argv []string, input []byte) (domain.GitResult, error) {
	return e.runRemoteIsolated(ctx, host, timeout, limit, argv, input, false)
}

// runReadRemote uses an OpenSSH ControlMaster for read-only work. Reusing the
// encrypted, authenticated transport removes the ~0.5s TCP, key exchange and
// authentication cost from every branch, history and commit-detail query.
func (e Executor) runReadRemote(ctx context.Context, host string, timeout time.Duration, limit int, argv []string) (domain.GitResult, error) {
	return e.runRemoteIsolated(ctx, host, timeout, limit, argv, nil, false)
}

func (e Executor) runRemoteIsolated(ctx context.Context, host string, timeout time.Duration, limit int, argv []string, input []byte, isolated bool) (domain.GitResult, error) {
	invalid := domain.GitResult{ExitCode: -1}
	if !hostPattern.MatchString(host) {
		return invalid, &domain.GitError{Code: domain.InvalidRepository}
	}
	remote, err := Encode(argv)
	if err != nil {
		return invalid, err
	}
	mode := SharedSession
	if isolated {
		mode = IsolatedSession
	}
	if !isolated {
		// The request deadline covers connection setup as well as execution.
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
		c, err := e.connections().get(ctx, e, host)
		if err != nil {
			return invalid, err
		}
		select {
		case c.slots <- struct{}{}:
			defer func() { <-c.slots }()
		case <-ctx.Done():
			return invalid, contextError(ctx.Err())
		}
	}
	args, err := e.SessionArgs(ctx, host, mode)
	if err != nil {
		return invalid, err
	}
	args = append(args, host, remote)
	executable := e.SSHPath
	if executable == "" {
		executable = "ssh"
	}
	result, err := process.RunInput(ctx, timeout, limit, executable, args, e.Environment(host), input)
	return classifyResult(result, err, host)
}

func classifyResult(result domain.GitResult, err error, host string) (domain.GitResult, error) {
	if err == nil {
		return result, nil
	}
	var ge *domain.GitError
	if errors.As(err, &ge) && ge.Code != domain.CommandFailed {
		return result, err
	}
	message := strings.ToLower(string(result.Stderr))
	code := domain.CommandFailed
	switch {
	case strings.Contains(message, "host key verification failed"), strings.Contains(message, "remote host identification has changed"), strings.Contains(message, "no ed25519 host key is known"):
		code = domain.HostKeyRejected
	case result.ExitCode == 255 && strings.Contains(message, "permission denied"):
		code = domain.SshAuthFailed
	case result.ExitCode == 255:
		code = domain.RemoteUnavailable
	case result.ExitCode == 127:
		code = domain.GitUnavailable
	case strings.Contains(message, "permission denied"):
		code = domain.PermissionDenied
	}
	if code == domain.SshAuthFailed {
		askpass.ClearHost(host)
	}
	return result, &domain.GitError{Code: code, Cause: err}
}

// ConnectionArgs returns the fixed OpenSSH options shared by Git and SFTP
// transports. Password prompting is opt-in and still leaves host-key checking
// strict; key and agent authentication remain the default.
func (e Executor) ConnectionArgs() []string {
	batchMode := "BatchMode=yes"
	if e.AllowPassword {
		batchMode = "BatchMode=no"
	}
	args := []string{"-T", "-o", batchMode, "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=5", "-o", "ConnectionAttempts=1", "-o", "ClearAllForwardings=yes", "-o", "PermitLocalCommand=no", "-o", "RemoteCommand=none"}
	if e.ConfigPath != "" {
		args = append(args, "-F", e.ConfigPath)
	}
	if e.KnownHostsFile != "" {
		args = append(args, "-o", "UserKnownHostsFile="+e.KnownHostsFile)
	}
	return args
}

// ForwardArgs returns strict OpenSSH options for an active reverse stream
// local forwarding session. Unlike ConnectionArgs it must not clear
// forwardings, because the forwarding is the purpose of this process.
func (e Executor) ForwardArgs(localSocket, remoteSocket string) []string {
	args := e.ConnectionArgs()
	for i, arg := range args {
		if arg == "ClearAllForwardings=yes" {
			args[i] = "ClearAllForwardings=no"
		}
	}
	return append(args, "-S", "none", "-o", "ControlMaster=no", "-N", "-o", "ExitOnForwardFailure=yes", "-o", "StreamLocalBindUnlink=yes", "-R", remoteSocket+":"+localSocket)
}

// Environment returns a clean Git/SSH environment plus the host hint used by
// the in-process askpass broker. The askpass socket and token are inherited
// from the application environment installed by askpass.Broker.
func (e Executor) Environment(host string) []string {
	env := gitexec.CleanEnvironment()
	return append(env, askpass.HostEnvironment(host)...)
}
