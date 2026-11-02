package remotehelper

import (
	"path"
	"strconv"
	"strings"

	"gitviewer/internal/gitargs"
)

// Validate is the helper-side whitelist. It accepts exactly the argv shapes the
// product reader and writer emit, and rejects every other Git command, global
// option, shell expression and unsafe path. Callers must treat a rejection as
// final (fail closed) rather than falling back to a shell command.
func Validate(args []string) error {
	if len(args) == 0 {
		return gitargs.Unsafe()
	}
	total := 0
	for _, arg := range args {
		if len(arg) > MaxArg || strings.ContainsRune(arg, 0) {
			return gitargs.Unsafe()
		}
		total += len(arg)
	}
	if total > MaxArgs {
		return gitargs.Unsafe()
	}
	if args[0] == "-c" {
		if len(args) == 10 && args[1] == "log.showSignature=false" {
			fileLog := args[2:]
			return valid(equal(fileLog[:5], "log", "--topo-order", "-z", "--format="+gitargs.HistoryFormat, "--max-count=101") &&
				gitargs.ValidOID(fileLog[5]) && fileLog[6] == "--" && gitargs.ValidPath(fileLog[7]))
		}
		if len(args) >= 3 && args[1] == "core.sshCommand="+gitargs.SafeSSHCommand {
			command := args[2:]
			switch command[0] {
			case "fetch":
				return valid(equal(command, "fetch", "--progress") || equal(command, "fetch", "--progress", "--prune") || equal(command, "fetch", "--progress", "--all"))
			case "pull":
				return validatePull(command)
			case "-c":
				if len(command) == 10 && command[1] == "remote."+command[8]+".mirror=false" {
					return validatePush(command[2:])
				}
			}
		}
		return gitargs.Unsafe()
	}
	switch args[0] {
	case "--version":
		return valid(len(args) == 1)
	case "rev-parse":
		return validateRevParse(args)
	case "status":
		return validateStatus(args)
	case "for-each-ref":
		return validateForEachRef(args)
	case "log":
		return validateLog(args)
	case "show":
		return validateShow(args)
	case "diff":
		return validateDiff(args)
	case "cat-file":
		return valid(len(args) == 3 && args[1] == "blob" && gitargs.ValidBlobSpec(args[2]))
	case "ls-tree":
		return validateLsTree(args)
	case "blame":
		return validateBlame(args)
	case "ls-files":
		return validateLsFiles(args)
	case "config":
		return validateConfig(args)
	case "check-ref-format":
		return valid(len(args) == 3 && args[1] == "--branch" && gitargs.ValidRefName(args[2]))
	case "symbolic-ref":
		return valid(len(args) == 3 && args[1] == "-q" && args[2] == "HEAD")
	case "rev-list":
		return valid(len(args) == 6 && args[1] == "--parents" && args[2] == "-n" && args[3] == "1" &&
			args[5] == "--" && gitargs.ValidRevSpec(args[4]) && strings.HasSuffix(args[4], "^{commit}"))
	case "worktree":
		return validateWorktree(args)
	case "stash":
		return validateStash(args)
	case "add":
		return validPaths(args, "add", "--")
	case "rm":
		return validPaths(args, "rm", "--cached", "--")
	case "restore":
		if len(args) >= 2 && (args[1] == "--staged" || args[1] == "--worktree") {
			return validPaths(args, "restore", args[1], "--")
		}
		return gitargs.Unsafe()
	case "apply":
		return validateApply(args)
	case "reset":
		return valid(len(args) == 3 && args[1] == "--soft" && args[2] == "HEAD^")
	case "commit":
		return valid((len(args) == 2 && args[1] == "--file=-") ||
			(len(args) == 3 && args[1] == "--amend" && args[2] == "--file=-"))
	case "switch":
		return validateSwitch(args)
	case "cherry-pick", "revert":
		return valid(len(args) == 3 && args[1] == "--no-edit" && gitargs.ValidOID(args[2]))
	case "fetch", "pull", "push":
		return gitargs.Unsafe() // Network writes require the exact safety prefix.
	}
	return gitargs.Unsafe()
}

func valid(ok bool) error {
	if ok {
		return nil
	}
	return gitargs.Unsafe()
}

func equal(args []string, want ...string) bool {
	if len(args) != len(want) {
		return false
	}
	for i := range want {
		if args[i] != want[i] {
			return false
		}
	}
	return true
}

func validateRevParse(args []string) error {
	if len(args) == 2 {
		switch args[1] {
		case "--is-inside-work-tree", "--show-toplevel", "--absolute-git-dir", "--git-common-dir":
			return nil
		}
	}
	if len(args) == 3 && args[1] == "--verify" && gitargs.ValidRevSpec(args[2]) {
		return nil
	}
	return gitargs.Unsafe()
}

func validateStatus(args []string) error {
	if equal(args, "status", "--porcelain=v1", "-z", "--untracked-files=all") {
		return nil
	}
	return valid(equal(args, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignored"))
}

func validateForEachRef(args []string) error {
	return valid(equal(args, "for-each-ref", "--format=%(objecttype)%00%(objectname)%00%(*objecttype)%00%(*objectname)", "refs/heads/", "refs/remotes/", "refs/tags/") ||
		equal(args, "for-each-ref", "--count=1", "--format=%(objectname)") ||
		equal(args, "for-each-ref", "--format=%(objectname)") ||
		equal(args, "for-each-ref", "--format=%(refname)%00%(objectname)%00%(upstream)%00%(HEAD)%00%(upstream:track)%00%(symref)", "refs/heads/", "refs/remotes/") ||
		equal(args, "for-each-ref", "--format=%(refname)%00%(objectname)", "refs/tags/"))
}

func validateLog(args []string) error {
	if len(args) == 6 && equal(args[:4], "log", "-1", "-z", "--format="+gitargs.HistoryFormat) {
		return valid(gitargs.ValidOID(strings.TrimSuffix(args[4], "^{commit}")) && strings.HasSuffix(args[4], "^{commit}") && args[5] == "--")
	}
	if len(args) < 8 || !equal(args[:4], "log", "--topo-order", "-z", "--format="+gitargs.HistoryFormat) || args[len(args)-1] != "--" {
		return gitargs.Unsafe()
	}
	count, ok := strings.CutPrefix(args[4], "--max-count=")
	skip, skipOK := strings.CutPrefix(args[5], "--skip=")
	if !ok || !skipOK || !validCount(count) || !validCount(skip) {
		return gitargs.Unsafe()
	}
	n, _ := strconv.Atoi(count)
	if n < 2 || n > 1001 || len(args)-7 > 4096 {
		return gitargs.Unsafe()
	}
	for _, arg := range args[6 : len(args)-1] {
		if !gitargs.ValidOID(arg) {
			return gitargs.Unsafe()
		}
	}
	return nil
}

func validCount(value string) bool {
	if value == "" {
		return false
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return false
		}
	}
	_, err := strconv.Atoi(value)
	return err == nil
}

func validateShow(args []string) error {
	if len(args) == 5 && equal(args[:3], "show", "-s", "--format=%B") {
		return valid(commitRev(args[3]) && args[4] == "--")
	}
	if len(args) == 11 && equal(args[:9], "show", "--format=", "--name-status", "-z", "--first-parent", "--root", "--find-renames", "--no-ext-diff", "--no-textconv") {
		return valid(commitRev(args[9]) && args[10] == "--")
	}
	if len(args) < 12 || len(args) > 14 || !equal(args[:4], "show", "--format=", "--root", "--first-parent") || !commitRev(args[4]) || !equal(args[5:12], patchOptions()...) {
		return gitargs.Unsafe()
	}
	return pathTail(args[12:])
}

func commitRev(value string) bool {
	return strings.HasSuffix(value, "^{commit}") && gitargs.ValidOID(strings.TrimSuffix(value, "^{commit}"))
}

func patchOptions() []string {
	return []string{"--no-ext-diff", "--no-textconv", "--no-color", "--find-renames", "--submodule=short", "--patch", "--"}
}

func pathTail(paths []string) error {
	for _, value := range paths {
		if !gitargs.ValidPath(value) {
			return gitargs.Unsafe()
		}
	}
	return nil
}

func validateDiff(args []string) error {
	if len(args) == 8 && equal(args[:7], "diff", "--no-index", "--no-ext-diff", "--no-textconv", "--no-color", "--", "/dev/null") {
		return valid(gitargs.ValidPath(args[7]))
	}
	if len(args) > 1 && args[1] == "--cached" {
		args = append([]string{"diff"}, args[2:]...)
	}
	if len(args) < 8 || len(args) > 10 || !equal(args[1:8], patchOptions()...) {
		return gitargs.Unsafe()
	}
	return pathTail(args[8:])
}

func validateLsTree(args []string) error {
	if len(args) != 6 || args[1] != "-z" || args[2] != "--full-tree" ||
		!gitargs.ValidOID(args[3]) || args[4] != "--" || !gitargs.ValidPath(args[5]) {
		return gitargs.Unsafe()
	}
	return nil
}

func validateBlame(args []string) error {
	if len(args) != 6 || args[1] != "--no-textconv" || args[2] != "--encoding=UTF-8" ||
		!gitargs.ValidOID(args[3]) || args[4] != "--" || !gitargs.ValidPath(args[5]) {
		return gitargs.Unsafe()
	}
	return nil
}

func validateLsFiles(args []string) error {
	if len(args) != 5 || args[2] != "-z" || args[3] != "--" {
		return gitargs.Unsafe()
	}
	if args[1] != "--unmerged" && args[1] != "--stage" && args[1] != "--others" {
		return gitargs.Unsafe()
	}
	for _, arg := range args[4:] {
		if !gitargs.ValidPath(arg) {
			return gitargs.Unsafe()
		}
	}
	return nil
}

func validateConfig(args []string) error {
	if len(args) != 3 || args[1] != "--get" {
		return gitargs.Unsafe()
	}
	key := args[2]
	name, ok := strings.CutPrefix(key, "branch.")
	if !ok {
		return gitargs.Unsafe()
	}
	for _, suffix := range []string{".remote", ".merge"} {
		if branch, found := strings.CutSuffix(name, suffix); found && gitargs.ValidRefName(branch) {
			return nil
		}
	}
	return gitargs.Unsafe()
}

func validateWorktree(args []string) error {
	if len(args) == 4 && args[1] == "list" {
		return valid(args[2] == "--porcelain" && args[3] == "-z")
	}
	if len(args) == 5 && args[1] == "add" && args[2] == "--" {
		return valid(validRemotePath(args[3]) && gitargs.ValidRefName(args[4]))
	}
	if len(args) == 4 && args[1] == "remove" && args[2] == "--" {
		return valid(validRemotePath(args[3]))
	}
	return gitargs.Unsafe()
}

func validRemotePath(value string) bool {
	return len(value) <= MaxDir && path.IsAbs(value) && path.Clean(value) == value && !strings.ContainsRune(value, 0) && value != "/"
}

func validateStash(args []string) error {
	switch {
	case equal(args, "stash", "list", "--format=%gd%x00%gs"):
		return nil
	case equal(args, "stash", "push"), equal(args, "stash", "push", "--include-untracked"), equal(args, "stash", "pop"):
		return nil
	}
	return gitargs.Unsafe()
}

func validPaths(args []string, prefix ...string) error {
	if len(args) <= len(prefix) || !equal(args[:len(prefix)], prefix...) {
		return gitargs.Unsafe()
	}
	for _, arg := range args[len(prefix):] {
		if !gitargs.ValidPath(arg) {
			return gitargs.Unsafe()
		}
	}
	return nil
}

func validateApply(args []string) error {
	if equal(args, "apply", "--unidiff-zero", "--whitespace=nowarn", "-") ||
		equal(args, "apply", "--unidiff-zero", "--whitespace=nowarn", "--cached", "-") ||
		equal(args, "apply", "--unidiff-zero", "--whitespace=nowarn", "--reverse", "-") ||
		equal(args, "apply", "--unidiff-zero", "--whitespace=nowarn", "--cached", "--reverse", "-") {
		return nil
	}
	return gitargs.Unsafe()
}

func validateSwitch(args []string) error {
	if len(args) == 4 && args[1] == "--no-guess" && args[2] == "--" {
		return valid(gitargs.ValidRefName(args[3]))
	}
	if len(args) == 4 && args[1] == "-c" {
		return valid(gitargs.ValidRefName(args[2]) && gitargs.ValidOID(args[3]))
	}
	return gitargs.Unsafe()
}

func validatePull(args []string) error {
	if len(args) == 7 && equal(args[:5], "pull", "--ff-only", "--no-rebase", "--progress", "--") {
		return valid(gitargs.ValidRemote(args[5]) && gitargs.ValidRemoteRef(args[6]))
	}
	if len(args) == 6 && equal(args[:4], "pull", "--rebase", "--progress", "--") {
		return valid(gitargs.ValidRemote(args[4]) && gitargs.ValidRemoteRef(args[5]))
	}
	return gitargs.Unsafe()
}

func validatePush(args []string) error {
	if len(args) != 8 || !equal(args[:6], "push", "--no-force", "--no-follow-tags", "--recurse-submodules=no", "--progress", "--") {
		return gitargs.Unsafe()
	}
	if !gitargs.ValidRemote(args[6]) {
		return gitargs.Unsafe()
	}
	source, target, ok := strings.Cut(args[7], ":")
	if !ok || !gitargs.ValidRemoteRef(target) {
		return gitargs.Unsafe()
	}
	if source == "HEAD" {
		return nil
	}
	return valid(gitargs.ValidRemoteRef(source))
}
