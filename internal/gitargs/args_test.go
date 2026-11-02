package gitargs

import (
	"strings"
	"testing"
)

func TestValidators(t *testing.T) {
	acceptPath := []string{"a", "dir/file", "dir/file with space", "中文/文件", "a\\b"}
	rejectPath := []string{"", ".", "..", "/abs", "a/../b", "a//b", "a/", "a\x00b"}
	for _, value := range acceptPath {
		if !ValidPath(value) {
			t.Errorf("path rejected: %q", value)
		}
	}
	for _, value := range rejectPath {
		if ValidPath(value) {
			t.Errorf("path accepted: %q", value)
		}
	}
	for _, value := range []string{"HEAD", "HEAD^", "HEAD^{commit}", strings.Repeat("a", 40), strings.Repeat("f", 64)} {
		if !ValidRevSpec(value) {
			t.Errorf("revision rejected: %q", value)
		}
	}
	for _, value := range []string{"head", "HEAD~1", strings.Repeat("a", 39), "HEAD:path"} {
		if ValidRevSpec(value) {
			t.Errorf("revision accepted: %q", value)
		}
	}
	for _, value := range []string{"main", "feature/x", "release-1.2"} {
		if !ValidRefName(value) {
			t.Errorf("ref rejected: %q", value)
		}
	}
	for _, value := range []string{"", "-x", "a b", "a..b", "a~1", "a^", "a:", "a?", "a*", "a[b", "a\\b", "a@{1}", "a/", "/a", "a.", "a\x00b"} {
		if ValidRefName(value) {
			t.Errorf("ref accepted: %q", value)
		}
	}
	for _, value := range []string{"origin", "upstream-2"} {
		if !ValidRemote(value) {
			t.Errorf("remote rejected: %q", value)
		}
	}
	for _, value := range []string{"", "-o", "a b", "a:b", "a\tb"} {
		if ValidRemote(value) {
			t.Errorf("remote accepted: %q", value)
		}
	}
	for _, value := range []string{"HEAD:dir/file", ":dir/file", strings.Repeat("a", 40)} {
		if !ValidBlobSpec(value) {
			t.Errorf("blob spec rejected: %q", value)
		}
	}
	for _, value := range []string{"", "HEAD:../x", ":a/../b", "HEAD~1:x", "HEAD:"} {
		if ValidBlobSpec(value) {
			t.Errorf("blob spec accepted: %q", value)
		}
	}
}

func TestIsWriteClassification(t *testing.T) {
	reads := [][]string{
		{"status", "--porcelain=v1", "-z", "--untracked-files=all"},
		{"for-each-ref", "--format=%(objectname)"},
		{"stash", "list", "--format=%gd%x00%gs"},
		{"worktree", "list", "--porcelain", "-z"},
		{"log", "--topo-order", "-z", "--format=" + HistoryFormat, "-n", "1", "--"},
		{"cat-file", "blob", "HEAD:path"},
	}
	writes := [][]string{
		{"add", "--", "path"},
		{"rm", "--cached", "--", "path"},
		{"restore", "--staged", "--", "path"},
		{"apply", "--unidiff-zero", "--whitespace=nowarn", "-"},
		{"reset", "--soft", "HEAD^"},
		{"commit", "--file=-"},
		{"switch", "-c", "topic", "HEAD"},
		{"stash", "push"},
		{"stash", "pop"},
		{"fetch", "--progress"},
		{"pull", "--rebase", "--progress", "--", "origin", "refs/heads/main"},
		{"push", "--no-force", "--no-follow-tags", "--recurse-submodules=no", "--progress", "--", "origin", "HEAD:refs/heads/main"},
		{"worktree", "add", "--", "/tmp/wt", "topic"},
		{"cherry-pick", "--no-edit", "HEAD"},
		{"revert", "--no-edit", "HEAD"},
		{"-c", "remote.origin.mirror=false", "push", "--no-force", "--no-follow-tags", "--recurse-submodules=no", "--progress", "--", "origin", "HEAD:refs/heads/main"},
	}
	for _, args := range reads {
		if IsWriteArgs(args) {
			t.Errorf("read classified as write: %v", args)
		}
	}
	for _, args := range writes {
		if !IsWriteArgs(args) {
			t.Errorf("write classified as read: %v", args)
		}
	}
}
