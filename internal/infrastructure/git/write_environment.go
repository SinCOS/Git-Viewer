package git

import "gitviewer/internal/gitargs"

// WriteEnvironment restores the user's normal Git configuration for identity,
// credential helpers and hooks. Inherited GIT_* overrides remain stripped.
func WriteEnvironment() []string { return gitargs.WriteEnvironment() }

// IsWriteArgs recognizes only the fixed commands emitted by Write.
func IsWriteArgs(args []string) bool { return gitargs.IsWriteArgs(args) }
