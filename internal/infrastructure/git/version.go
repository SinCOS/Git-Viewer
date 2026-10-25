package git

import (
	"gitviewer/internal/domain"
	"strconv"
	"strings"
)

// CheckVersion gates commands that require switch/restore and worktree -z.
// The enforced floor is 2.38; the execution record lists versions actually tested.
func CheckVersion(version string) error {
	fields := strings.Fields(version)
	if len(fields) < 3 || fields[0] != "git" || fields[1] != "version" {
		return &domain.GitError{Code: domain.GitUnavailable}
	}
	parts := strings.Split(fields[2], ".")
	if len(parts) < 2 {
		return &domain.GitError{Code: domain.UnsupportedGitVersion}
	}
	major, e1 := strconv.Atoi(parts[0])
	minor, e2 := strconv.Atoi(parts[1])
	if e1 != nil || e2 != nil || major < 2 || major == 2 && minor < 38 {
		return &domain.GitError{Code: domain.UnsupportedGitVersion}
	}
	return nil
}
