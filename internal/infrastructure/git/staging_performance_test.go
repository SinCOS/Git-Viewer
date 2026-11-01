package git

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"gitviewer/internal/domain"
)

// Exclude disk and SSH costs so this measures bulk-stage command preparation.
func BenchmarkBulkStagePreparation(b *testing.B) {
	for _, count := range []int{1000, 10000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			var status strings.Builder
			paths := make([]string, count)
			for i := range paths {
				paths[i] = fmt.Sprintf("src/file-%05d.go", i)
				fmt.Fprintf(&status, " M %s\x00", paths[i])
			}
			repo := domain.RepositoryContext{Repository: domain.Repository{RepositoryID: "bulk", Location: "ssh", Path: "/repo"}, SSHHost: "fixture", CommonDir: "/repo/.git"}
			reader := LocalReader{Transport: func(_ context.Context, _ string, _ int, _ []byte, args ...string) (domain.GitResult, error) {
				if args[0] == "status" {
					return domain.GitResult{Stdout: []byte(status.String())}, nil
				}
				return domain.GitResult{}, nil
			}}
			op := domain.GitOperation{RepositoryID: "bulk", Operation: domain.Stage, Paths: paths}
			b.ResetTimer()
			for b.Loop() {
				result, err := reader.Write(context.Background(), repo, op)
				if err != nil || !result.Verified {
					b.Fatal(result, err)
				}
			}
		})
	}
}
