package git

import "strings"

// WriteEnvironment restores the user's normal Git configuration for identity,
// credential helpers and hooks. Inherited GIT_* overrides remain stripped.
func WriteEnvironment() []string {
	env := CleanEnvironment()
	result := make([]string, 0, len(env))
	for _, v := range env {
		if !strings.HasPrefix(v, "GIT_CONFIG_NOSYSTEM=") && !strings.HasPrefix(v, "GIT_CONFIG_GLOBAL=") {
			result = append(result, v)
		}
	}
	return result
}

// IsWriteArgs recognizes only the fixed commands emitted by Write.
func IsWriteArgs(args []string) bool {
	for len(args) >= 2 && args[0] == "-c" {
		args = args[2:]
	}
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "add", "rm", "restore", "commit", "switch", "fetch", "pull", "push":
		return true
	case "worktree":
		return len(args) > 1 && (args[1] == "add" || args[1] == "remove")
	}
	return false
}
