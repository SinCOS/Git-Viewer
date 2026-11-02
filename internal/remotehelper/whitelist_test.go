package remotehelper

import (
	"strings"
	"testing"

	"gitviewer/internal/gitargs"
)

func TestWhitelistAcceptsReaderShapes(t *testing.T) {
	oid := strings.Repeat("a", 40)
	accepted := [][]string{
		{"--version"},
		{"rev-parse", "--is-inside-work-tree"},
		{"rev-parse", "--show-toplevel"},
		{"rev-parse", "--absolute-git-dir"},
		{"rev-parse", "--git-common-dir"},
		{"rev-parse", "--verify", "HEAD"},
		{"rev-parse", "--verify", "HEAD^"},
		{"rev-parse", "--verify", oid + "^{commit}"},
		{"status", "--porcelain=v1", "-z", "--untracked-files=all"},
		{"status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignored"},
		{"for-each-ref", "--format=%(objecttype)%00%(objectname)%00%(*objecttype)%00%(*objectname)", "refs/heads/", "refs/remotes/", "refs/tags/"},
		{"for-each-ref", "--count=1", "--format=%(objectname)"},
		{"for-each-ref", "--format=%(refname)%00%(objectname)%00%(upstream)%00%(HEAD)%00%(upstream:track)%00%(symref)", "refs/heads/", "refs/remotes/"},
		{"log", "--topo-order", "-z", "--format=" + gitargs.HistoryFormat, "--max-count=101", "--skip=0", oid, "--"},
		{"log", "-1", "-z", "--format=" + gitargs.HistoryFormat, oid + "^{commit}", "--"},
		{"show", "-s", "--format=%B", oid + "^{commit}", "--"},
		{"show", "--format=", "--name-status", "-z", "--first-parent", "--root", "--find-renames", "--no-ext-diff", "--no-textconv", oid + "^{commit}", "--"},
		{"show", "--format=", "--root", "--first-parent", oid + "^{commit}", "--no-ext-diff", "--no-textconv", "--no-color", "--find-renames", "--submodule=short", "--patch", "--", "dir/file"},
		{"diff", "--no-ext-diff", "--no-textconv", "--no-color", "--find-renames", "--submodule=short", "--patch", "--", "dir/file"},
		{"diff", "--cached", "--no-ext-diff", "--no-textconv", "--no-color", "--find-renames", "--submodule=short", "--patch", "--"},
		{"diff", "--no-index", "--no-ext-diff", "--no-textconv", "--no-color", "--", "/dev/null", "dir/file"},
		{"cat-file", "blob", oid},
		{"cat-file", "blob", "HEAD:dir/file"},
		{"cat-file", "blob", ":dir/file"},
		{"ls-tree", "-z", "--full-tree", oid, "--", "dir/file"},
		{"blame", "--no-textconv", "--encoding=UTF-8", oid, "--", "dir/file"},
		{"ls-files", "--unmerged", "-z", "--", "dir/file"},
		{"ls-files", "--stage", "-z", "--", "dir/file"},
		{"ls-files", "--others", "-z", "--", "dir/file"},
		{"config", "--get", "branch.main.remote"},
		{"config", "--get", "branch.feature/x.merge"},
		{"check-ref-format", "--branch", "topic"},
		{"symbolic-ref", "-q", "HEAD"},
		{"rev-list", "--parents", "-n", "1", oid + "^{commit}", "--"},
		{"worktree", "list", "--porcelain", "-z"},
		{"worktree", "add", "--", "/tmp/wt", "topic"},
		{"worktree", "remove", "--", "/tmp/wt"},
		{"stash", "list", "--format=%gd%x00%gs"},
		{"stash", "push"},
		{"stash", "push", "--include-untracked"},
		{"stash", "pop"},
		{"add", "--", "dir/file"},
		{"rm", "--cached", "--", "dir/file"},
		{"restore", "--staged", "--", "dir/file"},
		{"restore", "--worktree", "--", "dir/file"},
		{"apply", "--unidiff-zero", "--whitespace=nowarn", "--cached", "-"},
		{"apply", "--unidiff-zero", "--whitespace=nowarn", "--reverse", "-"},
		{"reset", "--soft", "HEAD^"},
		{"commit", "--file=-"},
		{"commit", "--amend", "--file=-"},
		{"switch", "--no-guess", "--", "topic"},
		{"switch", "-c", "topic", oid},
		{"cherry-pick", "--no-edit", oid},
		{"revert", "--no-edit", oid},
		{"-c", "core.sshCommand=" + gitargs.SafeSSHCommand, "fetch", "--progress"},
		{"-c", "core.sshCommand=" + gitargs.SafeSSHCommand, "fetch", "--progress", "--prune"},
		{"-c", "core.sshCommand=" + gitargs.SafeSSHCommand, "fetch", "--progress", "--all"},
		{"-c", "core.sshCommand=" + gitargs.SafeSSHCommand, "pull", "--ff-only", "--no-rebase", "--progress", "--", "origin", "refs/heads/main"},
		{"-c", "core.sshCommand=" + gitargs.SafeSSHCommand, "pull", "--rebase", "--progress", "--", "origin", "refs/heads/main"},
		{"-c", "core.sshCommand=" + gitargs.SafeSSHCommand, "-c", "remote.origin.mirror=false", "push", "--no-force", "--no-follow-tags", "--recurse-submodules=no", "--progress", "--", "origin", "HEAD:refs/heads/main"},
		{"-c", "core.sshCommand=" + gitargs.SafeSSHCommand, "-c", "remote.origin.mirror=false", "push", "--no-force", "--no-follow-tags", "--recurse-submodules=no", "--progress", "--", "origin", "refs/heads/main:refs/heads/main"},
		{"-c", "log.showSignature=false", "log", "--topo-order", "-z", "--format=" + gitargs.HistoryFormat, "--max-count=101", oid, "--", "dir/file"},
	}
	for _, args := range accepted {
		if err := Validate(args); err != nil {
			t.Errorf("rejected: %v", args)
		}
	}
}

func TestWhitelistRejectsArbitraryCommands(t *testing.T) {
	oid := strings.Repeat("a", 40)
	rejected := [][]string{
		{},
		{"sh", "-c", "rm -rf /"},
		{"status"},
		{"status", "--porcelain=v1", "-z", "--untracked-files=all", "--short"},
		{"--exec-path"},
		{"--git-dir", "/tmp"},
		{"-c", "core.pager=sh", "status", "--porcelain=v1", "-z", "--untracked-files=all"},
		{"-c", "core.sshCommand=sh", "fetch", "--progress"},
		{"config", "--get", "core.sshCommand"},
		{"config", "--get", "branch.main.remote.extra"},
		{"rev-parse", "--verify", "HEAD~1"},
		{"rev-parse", "--verify", "../../etc"},
		{"log", "--format=%H%n%an", oid, "--"},
		{"log", "--output=/tmp/x", oid, "--"},
		{"show", "--format=%B", "--", oid},
		{"diff", "--output=/tmp/x", "--", "a"},
		{"diff", "--no-index", "--", "/etc/passwd", "a"},
		{"cat-file", "tree", oid},
		{"cat-file", "blob", "HEAD:../x"},
		{"ls-tree", "-z", "--full-tree", oid, "--", "../x"},
		{"ls-files", "--stage", "-z", "--"},
		{"worktree", "add", "--", "relative", "topic"},
		{"worktree", "add", "--", "/tmp/wt", "-b"},
		{"worktree", "remove", "--", "/"},
		{"stash", "apply"},
		{"add", "--", "../x"},
		{"add", "--", "/etc/passwd"},
		{"reset", "--hard"},
		{"commit", "-m", "message"},
		{"switch", "-C", "topic", oid},
		{"cherry-pick", "--no-edit", "HEAD~1"},
		{"push", "--force", "--", "origin", "HEAD:refs/heads/main"},
		{"push", "--no-force", "--no-follow-tags", "--recurse-submodules=no", "--progress", "--", "-oProxyCommand=x", "HEAD:refs/heads/main"},
		{"-c", "core.sshCommand=" + gitargs.SafeSSHCommand, "pull", "--rebase", "--progress", "--", "origin", "main"},
		{"-c", "core.sshCommand=" + gitargs.SafeSSHCommand, "fetch", "--progress", "--depth=1"},
		{"apply", "--unidiff-zero", "--whitespace=nowarn", "/tmp/patch"},
		{"apply", "--unidiff-zero", "--whitespace=nowarn", "-", "--unsafe-paths"},
		{"symbolic-ref", "HEAD", "refs/heads/main"},
		{"-c"},
		{"status\x00"},
		{"log", "--topo-order", "-z", "--format=" + gitargs.HistoryFormat, "--max-count=101", strings.Repeat("a", 17), "--"},
	}
	for _, args := range rejected {
		if err := Validate(args); err == nil {
			t.Errorf("accepted: %q", args)
		}
	}
}

func TestWhitelistLimits(t *testing.T) {
	if err := Validate([]string{"status", "--porcelain=v1", "-z", "--untracked-files=all", strings.Repeat("x", MaxArg)}); err == nil {
		t.Fatal("oversized argument accepted")
	}
	many := []string{"add", "--"}
	for i := 0; i < 700; i++ {
		many = append(many, strings.Repeat("a", 2000))
	}
	if err := Validate(many); err == nil {
		t.Fatal("oversized argv accepted")
	}
}

func FuzzValidate(f *testing.F) {
	for _, seed := range [][]string{
		{"status", "--porcelain=v1", "-z", "--untracked-files=all"},
		{"commit", "--file=-"},
		{"-c", "core.sshCommand=" + gitargs.SafeSSHCommand, "fetch", "--progress"},
	} {
		f.Add(strings.Join(seed, "\x00"))
	}
	f.Fuzz(func(t *testing.T, joined string) {
		if len(joined) > 1<<16 {
			return
		}
		args := strings.Split(joined, "\x00")
		// Validate must never panic, regardless of input.
		_ = Validate(args)
	})
}

func TestWhitelistConfigCannotEscapeItsCommand(t *testing.T) {
	status := []string{"status", "--porcelain=v1", "-z", "--untracked-files=all"}
	push := []string{"push", "--no-force", "--no-follow-tags", "--recurse-submodules=no", "--progress", "--", "origin", "HEAD:refs/heads/main"}
	safe := []string{"-c", "core.sshCommand=" + gitargs.SafeSSHCommand}
	for _, args := range [][]string{
		append([]string{"-c", "log.showSignature=false"}, status...),
		append(append([]string{}, safe...), status...),
		append([]string{"-c", "remote.origin.mirror=false"}, status...),
		{"fetch", "--progress"}, {"pull", "--rebase", "--progress", "--", "origin", "refs/heads/main"},
		push, append(append([]string{}, safe...), push...),
		append(append(append([]string{}, safe...), "-c", "remote.other.mirror=false"), push...),
		append(append(append([]string{}, safe...), "-c", "remote.origin.mirror=false"), "fetch", "--progress"),
		{"apply", "--unidiff-zero", "--whitespace=nowarn", "-", "-"},
		{"show", "--format=%B", strings.Repeat("a", 40) + "^{commit}", "--"},
		{"log", strings.Repeat("a", 40), "--"},
		{"for-each-ref", "--format=%B"},
	} {
		if Validate(args) == nil {
			t.Errorf("accepted %q", args)
		}
	}
}
