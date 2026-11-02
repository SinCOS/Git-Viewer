// Package gitargs holds the fixed Git argv vocabulary, environment policy and
// argument validators shared by the local reader, the SSH CLI fallback and the
// remote helper. Nothing here accepts a shell command or a free-form argv.
package gitargs

import (
	"os"
	"path"
	"regexp"
	"strings"

	"gitviewer/internal/domain"
)

// HistoryFormat is the NUL-delimited commit record used by log and show.
const HistoryFormat = "%H%x00%P%x00%an%x00%aI%x00%s%x00%D"

var oidPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// SafeSSHCommand is the fixed core.sshCommand used for network Git writes so
// a repository-level override cannot weaken host-key checking or prompt.
const SafeSSHCommand = "ssh -T -o BatchMode=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=5 -o ConnectionAttempts=1 -o ClearAllForwardings=yes -o PermitLocalCommand=no -o RemoteCommand=none"

// BasePrefix is the fixed Git global-option prefix. The remote helper inserts
// it itself; clients never send these options.
func BasePrefix() []string {
	return []string{
		"--no-pager", "--no-optional-locks", "--literal-pathspecs",
		"-c", "core.fsmonitor=false",
		"-c", "log.showSignature=false",
		"-c", "color.ui=false",
	}
}

// CleanEnvironment removes inherited repository and configuration overrides
// while keeping PATH, HOME, SSH agent and ordinary locale variables.
func CleanEnvironment() []string {
	var env []string
	for _, value := range os.Environ() {
		key, _, _ := strings.Cut(value, "=")
		if !strings.HasPrefix(key, "GIT_") && key != "PAGER" {
			env = append(env, value)
		}
	}
	return append(env, "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
}

// WriteEnvironment restores the user's normal Git configuration so identity,
// credential helpers and hooks behave exactly as they do in a terminal.
// Inherited GIT_* overrides remain stripped.
func WriteEnvironment() []string {
	env := CleanEnvironment()
	result := make([]string, 0, len(env))
	for _, value := range env {
		if !strings.HasPrefix(value, "GIT_CONFIG_NOSYSTEM=") && !strings.HasPrefix(value, "GIT_CONFIG_GLOBAL=") {
			result = append(result, value)
		}
	}
	return result
}

// IsWriteArgs recognizes only the fixed commands emitted by the product write
// path. It intentionally errs toward "write": an unrecognized mutation must
// never be granted the read-only environment or concurrent scheduling.
func IsWriteArgs(args []string) bool {
	for len(args) >= 2 && args[0] == "-c" {
		args = args[2:]
	}
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "add", "rm", "restore", "apply", "reset", "commit", "switch", "fetch", "pull", "push", "cherry-pick", "revert":
		return true
	case "stash":
		return len(args) < 2 || args[1] != "list"
	case "worktree":
		return len(args) > 1 && (args[1] == "add" || args[1] == "remove")
	}
	return false
}

// ValidOID accepts a full SHA-1 or SHA-256 object ID.
func ValidOID(value string) bool { return oidPattern.MatchString(value) }

// ValidPath accepts a canonical repository-relative POSIX path. Remote paths
// are POSIX regardless of the local platform.
func ValidPath(value string) bool {
	if value == "" || strings.ContainsRune(value, 0) {
		return false
	}
	if path.IsAbs(value) || path.Clean(value) != value || value == "." {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

// ValidRefName accepts a conservative Git branch name. The remote side still
// runs check-ref-format before creating anything; this only blocks option
// injection and control characters from reaching argv.
func ValidRefName(value string) bool {
	if value == "" || len(value) > 1024 || strings.HasPrefix(value, "-") ||
		strings.ContainsAny(value, "\x00\r\n\t ~^:?*[\\") ||
		strings.Contains(value, "..") || strings.Contains(value, "@{") ||
		strings.HasPrefix(value, "/") || strings.HasSuffix(value, "/") ||
		strings.HasSuffix(value, ".") || strings.Contains(value, "//") {
		return false
	}
	for _, r := range value {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}

// ValidRemote accepts a configured remote name. Colons and whitespace are
// rejected so a name cannot become a refspec or an option.
func ValidRemote(value string) bool {
	if value == "" || len(value) > 1024 || strings.HasPrefix(value, "-") {
		return false
	}
	if strings.ContainsAny(value, "\x00\r\n\t :~^?*[\\") {
		return false
	}
	for _, r := range value {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}

// ValidRemoteRef accepts "refs/heads/<branch>" used by pull and push.
func ValidRemoteRef(value string) bool {
	const prefix = "refs/heads/"
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	return ValidRefName(strings.TrimPrefix(value, prefix))
}

// ValidRevSpec accepts the exact revision forms the reader uses.
func ValidRevSpec(value string) bool {
	switch value {
	case "HEAD", "HEAD^", "HEAD^{commit}":
		return true
	}
	if strings.HasSuffix(value, "^{commit}") {
		return ValidOID(strings.TrimSuffix(value, "^{commit}"))
	}
	return ValidOID(value)
}

// ValidBlobSpec accepts an object ID or a "revision:path" expression.
func ValidBlobSpec(value string) bool {
	if ValidOID(value) {
		return true
	}
	rev, file, ok := strings.Cut(value, ":")
	if !ok || file == "" {
		return false
	}
	if rev == "" {
		return ValidPath(file)
	}
	if !ValidRevSpec(rev) || strings.HasSuffix(rev, "^{commit}") {
		return false
	}
	return ValidPath(file)
}

// ValidFormat accepts the fixed pretty formats used by log and show. Only the
// product formats are allowed; format strings are not arbitrary Git code.
func ValidFormat(value string) bool {
	switch value {
	case "", HistoryFormat, "%B", "%gd%x00%gs", "%(objectname)",
		"%(objecttype)%00%(objectname)%00%(*objecttype)%00%(*objectname)",
		"%(refname)%00%(objectname)%00%(upstream)%00%(HEAD)%00%(upstream:track)%00%(symref)",
		"%(refname)%00%(objectname)":
		return true
	}
	return false
}

// Unsafe returns the standard fail-closed validation error.
func Unsafe() error { return &domain.GitError{Code: domain.UnsafeOperation} }
