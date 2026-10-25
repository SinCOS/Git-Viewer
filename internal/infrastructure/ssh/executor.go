package ssh

import (
	"context"
	"errors"
	"path"
	"regexp"
	"strings"

	"gitviewer/internal/domain"
	gitexec "gitviewer/internal/infrastructure/git"
	"gitviewer/internal/infrastructure/process"
)

var hostPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

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
	remote, err := Encode(append([]string{"env", "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "git"}, gitArgs...))
	if err != nil {
		return invalid, err
	}
	args := []string{"-T", "-o", "BatchMode=yes", "-o", "StrictHostKeyChecking=yes", "-o", "ConnectTimeout=5", "-o", "ConnectionAttempts=1", "-o", "ClearAllForwardings=yes", "-o", "PermitLocalCommand=no", "-o", "RemoteCommand=none"}
	if e.ConfigPath != "" {
		args = append(args, "-F", e.ConfigPath)
	}
	if e.KnownHostsFile != "" {
		args = append(args, "-o", "UserKnownHostsFile="+e.KnownHostsFile)
	}
	args = append(args, repo.SSHHost, remote)
	executable := e.SSHPath
	if executable == "" {
		executable = "ssh"
	}
	result, err := process.Run(ctx, command.Timeout, e.OutputLimit, executable, args, gitexec.CleanEnvironment())
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
	return result, &domain.GitError{Code: code, Cause: err}
}
