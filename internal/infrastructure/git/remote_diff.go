package git

import (
	"bytes"
	"context"
	"errors"
	"gitviewer/internal/domain"
	"unicode/utf8"
)

func (r LocalReader) remoteUntracked(ctx context.Context, repo domain.RepositoryContext, p domain.DiffPreview) (domain.DiffPreview, error) {
	r.OutputLimit = PreviewLimit
	result, err := r.run(ctx, repo.Repository.Path, "diff", "--no-index", "--no-ext-diff", "--no-textconv", "--no-color", "--", "/dev/null", p.Path)
	var ge *domain.GitError
	if errors.As(err, &ge) && ge.Code == domain.OutputLimitExceeded {
		p.Limited = true
		return p, nil
	}
	if err != nil && !(errors.As(err, &ge) && ge.Code == domain.CommandFailed && result.ExitCode == 1) {
		return p, err
	}
	p.Binary = !utf8.Valid(result.Stdout) || bytes.Contains(result.Stdout, []byte("\nBinary files "))
	if utf8.Valid(result.Stdout) {
		p.Text = displayText(result.Stdout, &p.Limited)
	}
	if !p.Binary && !p.Limited {
		if text, ok := addedTextFromPatch(result.Stdout); ok {
			p.OldText, p.NewText, p.FullText = "", text, true
		}
	}
	return p, nil
}
