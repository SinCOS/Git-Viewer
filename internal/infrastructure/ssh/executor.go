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
	return e.runRemote(ctx, repo.SSHHost, command.Timeout, e.OutputLimit, append([]string{"env", "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "git"}, gitArgs...))
}

func (e Executor) runRemote(ctx context.Context, host string, timeout time.Duration, limit int, argv []string) (domain.GitResult, error) {
	return e.runRemoteInput(ctx, host, timeout, limit, argv, nil)
}
func (e Executor) runRemoteInput(ctx context.Context, host string, timeout time.Duration, limit int, argv []string, input []byte) (domain.GitResult, error) {
	invalid := domain.GitResult{ExitCode: -1}
	if !hostPattern.MatchString(host) {
		return invalid, &domain.GitError{Code: domain.InvalidRepository}
	}
	remote, err := Encode(argv)
	if err != nil {
		return invalid, err
	}
	args := e.ConnectionArgs()
	args = append(args, host, remote)
	executable := e.SSHPath
	if executable == "" {
		executable = "ssh"
	}
	result, err := process.RunInput(ctx, timeout, limit, executable, args, e.Environment(host), input)
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
	batchMode := "BatchMode=yes"
	if e.AllowPassword {
		batchMode = "BatchMode=no"
	}
	args := []string{
		"-T", "-N",
		"-o", batchMode,
		"-o", "StrictHostKeyChecking=yes",
		"-o", "ConnectTimeout=5",
		"-o", "ConnectionAttempts=1",
		"-o", "ClearAllForwardings=no",
		"-o", "PermitLocalCommand=no",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "StreamLocalBindUnlink=yes",
		"-R", remoteSocket + ":" + localSocket,
	}
	if e.ConfigPath != "" {
		args = append(args, "-F", e.ConfigPath)
	}
	if e.KnownHostsFile != "" {
		args = append(args, "-o", "UserKnownHostsFile="+e.KnownHostsFile)
	}
	return args
}

// Environment returns a clean Git/SSH environment plus the host hint used by
// the in-process askpass broker. The askpass socket and token are inherited
// from the application environment installed by askpass.Broker.
func (e Executor) Environment(host string) []string {
	env := gitexec.CleanEnvironment()
	return append(env, askpass.HostEnvironment(host)...)
}
