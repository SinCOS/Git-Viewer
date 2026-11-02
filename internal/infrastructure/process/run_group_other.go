//go:build !unix

package process

import (
	"context"
	"time"

	"gitviewer/internal/domain"
)

// RunInputGroup falls back to ordinary process lifetime management on systems
// without POSIX process groups. The product targets macOS and Linux, where the
// group implementation is used.
func RunInputGroup(ctx context.Context, timeout time.Duration, limit int, executable string, args []string, env []string, input []byte) (domain.GitResult, error) {
	return RunInput(ctx, timeout, limit, executable, args, env, input)
}
