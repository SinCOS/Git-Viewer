package git

import (
	"bytes"
	"regexp"
	"strings"
	"time"

	"gitviewer/internal/domain"
)

func parseError() error { return &domain.GitError{Code: domain.ParseError} }

// tokens requires the protocol's final NUL. Paths are bytes, not lines.
func tokens(data []byte) ([][]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}
	if data[len(data)-1] != 0 {
		return nil, parseError()
	}
	return bytes.Split(data[:len(data)-1], []byte{0}), nil
}
func ParseStatus(data []byte) ([]domain.Change, error) {
	fields, err := tokens(data)
	if err != nil {
		return nil, err
	}
	result := make([]domain.Change, 0, len(fields))
	conflicts := map[string]bool{"DD": true, "AU": true, "UD": true, "UA": true, "DU": true, "AA": true, "UU": true}
	for i := 0; i < len(fields); i++ {
		field := fields[i]
		if len(field) < 4 || field[2] != ' ' {
			return nil, parseError()
		}
		xy := string(field[:2])
		if xy == "  " {
			return nil, parseError()
		}
		for _, c := range field[:2] {
			if !strings.ContainsRune(" MADRCUT?!", rune(c)) {
				return nil, parseError()
			}
		}
		if (field[0] == '?' || field[1] == '?') && xy != "??" || (field[0] == '!' || field[1] == '!') && xy != "!!" {
			return nil, parseError()
		}
		row := domain.Change{Path: string(field[3:]), IndexStatus: string(field[0]), WorktreeStatus: string(field[1]), Untracked: xy == "??", Conflicted: conflicts[xy]}
		if bytes.ContainsAny(field[:2], "RC") {
			i++
			if i >= len(fields) || len(fields[i]) == 0 {
				return nil, parseError()
			}
			old := string(fields[i])
			row.OldPath = &old
		}
		result = append(result, row)
	}
	return result, nil
}

var oidPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

func ParseHistory(data []byte) ([]domain.CommitRecord, error) {
	fields, err := tokens(data)
	if err != nil {
		return nil, err
	}
	if len(fields)%6 != 0 {
		return nil, parseError()
	}
	result := make([]domain.CommitRecord, 0, len(fields)/6)
	for i := 0; i < len(fields); i += 6 {
		oid := string(fields[i])
		if !oidPattern.MatchString(oid) {
			return nil, parseError()
		}
		parents := strings.Fields(string(fields[i+1]))
		for _, parent := range parents {
			if !oidPattern.MatchString(parent) {
				return nil, parseError()
			}
		}
		date, err := time.Parse(time.RFC3339, string(fields[i+3]))
		if err != nil {
			return nil, parseError()
		}
		result = append(result, domain.CommitRecord{OID: oid, Parents: parents, Author: string(fields[i+2]), AuthoredAt: date, Subject: string(fields[i+4]), Refs: string(fields[i+5])})
	}
	return result, nil
}
func ParseWorktrees(data []byte) ([]domain.WorktreeRecord, error) {
	fields, err := tokens(data)
	if err != nil {
		return nil, err
	}
	result := make([]domain.WorktreeRecord, 0)
	var current *domain.WorktreeRecord
	seen := map[string]bool{}
	for _, field := range fields {
		if len(field) == 0 {
			if current == nil || (!current.Bare && !oidPattern.MatchString(current.Head)) {
				return nil, parseError()
			}
			if current.Detached && current.Branch != "" {
				return nil, parseError()
			}
			result = append(result, *current)
			current = nil
			seen = map[string]bool{}
			continue
		}
		key, value, hasValue := strings.Cut(string(field), " ")
		if seen[key] {
			return nil, parseError()
		}
		seen[key] = true
		if key == "worktree" {
			if current != nil || !hasValue || value == "" {
				return nil, parseError()
			}
			current = &domain.WorktreeRecord{Path: value}
			continue
		}
		if current == nil {
			return nil, parseError()
		}
		switch key {
		case "HEAD":
			if !hasValue {
				return nil, parseError()
			}
			current.Head = value
		case "branch":
			if !strings.HasPrefix(value, "refs/") {
				return nil, parseError()
			}
			current.Branch = value
		case "bare":
			if hasValue {
				return nil, parseError()
			}
			current.Bare = true
		case "detached":
			if hasValue {
				return nil, parseError()
			}
			current.Detached = true
		case "locked":
			current.Locked = true
			current.LockReason = value
		case "prunable":
			current.Prunable = true
			current.PruneReason = value
		default:
			return nil, parseError()
		}
	}
	if current != nil {
		return nil, parseError()
	}
	return result, nil
}
