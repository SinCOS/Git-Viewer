//go:build unix

package ssh

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"gitviewer/internal/domain"
	"gitviewer/internal/testgit"
)

// Optional structural comparison: loopback does not predict real LAN latency.
// Run GITVIEWER_HELPER_LATENCY=1 go test -run TestHelperLatency -v ./internal/infrastructure/ssh.
func TestHelperLatency(t *testing.T) {
	if os.Getenv("GITVIEWER_HELPER_LATENCY") != "1" {
		t.Skip("set GITVIEWER_HELPER_LATENCY=1 for latency measurements")
	}
	f := newHelperSSHFixture(t, true)
	dir := filepath.Join(f.dir, "latency-repo")
	os.Mkdir(dir, 0700)
	testgit.Run(t, dir, "init", "-b", "main")
	testgit.Run(t, dir, "config", "user.name", "Test")
	testgit.Run(t, dir, "config", "user.email", "test@example.invalid")
	testgit.Write(t, filepath.Join(dir, "file"), "base\n")
	testgit.Run(t, dir, "add", "--", "file")
	testgit.Run(t, dir, "commit", "-m", "base")
	ctx := context.Background()
	repo, err := f.executor.Open(ctx, "fixture", dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"helper", "cli"} {
		executor := f.executor
		if mode == "cli" {
			executor.Helpers = nil
		}
		reader := executor.Reader("fixture")
		for _, kind := range []string{"status", "batch", "commit"} {
			samples := make([]time.Duration, 15)
			for n := range samples {
				if kind == "commit" {
					testgit.Write(t, filepath.Join(dir, "file"), time.Now().String()+"\n")
					testgit.Run(t, dir, "add", "--", "file")
				}
				started := time.Now()
				switch kind {
				case "status":
					_, err = reader.Status(ctx, repo)
				case "batch":
					_, err = reader.ListRefs(ctx, repo)
				case "commit":
					_, err = reader.Write(ctx, repo, domain.GitOperation{RepositoryID: repo.Repository.RepositoryID, Operation: domain.Commit, Message: "latency fixture"})
				}
				if err != nil {
					t.Fatalf("%s/%s: %v", mode, kind, err)
				}
				samples[n] = time.Since(started)
			}
			sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
			t.Logf("%s %s n=%d p50=%.3fms p95=%.3fms", mode, kind, len(samples), float64(samples[len(samples)/2])/float64(time.Millisecond), float64(samples[(len(samples)*95+99)/100-1])/float64(time.Millisecond))
		}
	}
}
