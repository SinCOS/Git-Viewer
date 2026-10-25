package git

import (
	"context"
	"gitviewer/internal/domain"
	"strings"
)

type CommitFile struct {
	Status  string
	Path    string
	OldPath string
}
type CommitDetails struct {
	Commit  domain.CommitRecord
	Message string
	Files   []CommitFile
}

func (r LocalReader) CommitDetails(ctx context.Context, repo domain.RepositoryContext, oid string) (CommitDetails, error) {
	details := CommitDetails{Files: []CommitFile{}}
	if !oidPattern.MatchString(oid) {
		return details, &domain.GitError{Code: domain.UnsafeOperation}
	}
	record, err := r.run(ctx, repo.Repository.Path, "log", "-1", "-z", "--format="+HistoryFormat, oid+"^{commit}", "--")
	if err != nil {
		return details, err
	}
	records, err := ParseHistory(record.Stdout)
	if err != nil || len(records) != 1 {
		return details, parseError()
	}
	details.Commit = records[0]
	r.OutputLimit = PreviewLimit
	message, err := r.run(ctx, repo.Repository.Path, "show", "-s", "--format=%B", oid+"^{commit}", "--")
	if err != nil {
		return details, err
	}
	limited := false
	details.Message = displayText(message.Stdout, &limited)
	files, err := r.run(ctx, repo.Repository.Path, "show", "--format=", "--name-status", "-z", "--first-parent", "--root", "--find-renames", "--no-ext-diff", "--no-textconv", oid+"^{commit}", "--")
	if err != nil {
		return details, err
	}
	fields, err := tokens(files.Stdout)
	if err != nil {
		return details, err
	}
	for i := 0; i < len(fields); {
		status := string(fields[i])
		i++
		if i >= len(fields) || status == "" {
			return details, parseError()
		}
		file := CommitFile{Status: status, Path: string(fields[i])}
		i++
		if strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C") {
			file.OldPath = file.Path
			if i >= len(fields) {
				return details, parseError()
			}
			file.Path = string(fields[i])
			i++
		}
		details.Files = append(details.Files, file)
	}
	return details, nil
}
