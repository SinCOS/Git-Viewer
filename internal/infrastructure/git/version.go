package git

import (
	"gitviewer/internal/domain"
	"strconv"
	"strings"
)

// Version is a parsed Git major/minor version.
type Version struct {
	Major int
	Minor int
}

// ParseVersion accepts the output of git --version.
func ParseVersion(version string) (Version, error) {
	fields := strings.Fields(version)
	if len(fields) < 3 || fields[0] != "git" || fields[1] != "version" {
		return Version{}, &domain.GitError{Code: domain.GitUnavailable}
	}
	parts := strings.Split(fields[2], ".")
	if len(parts) < 2 {
		return Version{}, &domain.GitError{Code: domain.UnsupportedGitVersion}
	}
	major, e1 := strconv.Atoi(parts[0])
	minor, e2 := strconv.Atoi(parts[1])
	if e1 != nil || e2 != nil {
		return Version{}, &domain.GitError{Code: domain.UnsupportedGitVersion}
	}
	return Version{Major: major, Minor: minor}, nil
}

// AtLeast reports whether v is at least the supplied Git version.
func (v Version) AtLeast(major, minor int) bool {
	return v.Major > major || v.Major == major && v.Minor >= minor
}

// CheckVersion gates the read path: porcelain v1/z status, null-delimited
// log/ref output and -C. Write operations that need newer switch/restore and
// worktree options are checked separately.
func CheckVersion(version string) error {
	parsed, err := ParseVersion(version)
	if err != nil {
		return err
	}
	if !parsed.AtLeast(2, 13) {
		return &domain.GitError{Code: domain.UnsupportedGitVersion, Reason: "Git 2.13 or newer is required"}
	}
	return nil
}
