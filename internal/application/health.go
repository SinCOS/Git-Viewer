package application

import (
	"context"
	"strings"
	"time"

	"gitviewer/internal/infrastructure/git"
	"gitviewer/internal/infrastructure/process"
)

const Version = "Go 1.27.3 · MyGo 9c69cf829ece"

// Health is a Phase 0 read-only background subprocess probe, not repository UI.
func Health(ctx context.Context) (string, error) {
	result, err := process.Run(ctx, 5*time.Second, 4096, "git", []string{"--version"}, git.CleanEnvironment())
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(result.Stdout)), nil
}
