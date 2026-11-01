package ssh

import "testing"

func TestIndexConnectionReuseBoundary(t *testing.T) {
	for _, args := range [][]string{
		{"add", "--", "file"}, {"restore", "--staged", "--", "file"}, {"rm", "--cached", "--", "file"},
	} {
		if !indexOnlyArgs(args) {
			t.Errorf("index-only command not shared: %v", args)
		}
	}
	for _, args := range [][]string{
		{"commit", "-m", "message"}, {"restore", "--worktree", "--", "file"}, {"reset", "--hard"},
		{"apply", "--cached"}, {"push"}, {"fetch"}, {"worktree", "add", "--", "/tmp/tree", "branch"}, {"add", "--renormalize", "--", "file"}, nil,
	} {
		if indexOnlyArgs(args) {
			t.Errorf("command escaped isolation: %v", args)
		}
	}
}
