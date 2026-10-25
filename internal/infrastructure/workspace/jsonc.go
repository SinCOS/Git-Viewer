package workspace

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"gitviewer/internal/domain"
	"strings"
)

var ErrConflict = errors.New("WorkspaceConflict")
var ErrInvalid = errors.New("InvalidWorkspace")

type span struct{ start, end int }
type Document struct {
	Text     []byte
	Folders  []domain.WorkspaceFolder
	Revision string
	folders  span
	comments []span
}

// sanitize keeps byte offsets intact. Strings are scanned before comment markers;
// trailing commas are blanked only outside strings and comments.
func sanitize(text []byte) ([]byte, []span, error) {
	clean := bytes.Clone(text)
	comments := []span{}
	for i := 0; i < len(text); {
		if text[i] == '"' {
			i++
			closed := false
			for i < len(text) {
				if text[i] == '\\' {
					i += 2
					continue
				}
				if text[i] == '"' {
					i++
					closed = true
					break
				}
				i++
			}
			if !closed {
				return nil, nil, ErrInvalid
			}
			continue
		}
		if i+1 < len(text) && text[i] == '/' && (text[i+1] == '/' || text[i+1] == '*') {
			start := i
			if text[i+1] == '/' {
				i += 2
				for i < len(text) && text[i] != '\n' {
					i++
				}
			} else {
				i += 2
				for i+1 < len(text) && !(text[i] == '*' && text[i+1] == '/') {
					i++
				}
				if i+1 >= len(text) {
					return nil, nil, ErrInvalid
				}
				i += 2
			}
			comments = append(comments, span{start, i})
			for j := start; j < i; j++ {
				if clean[j] != '\n' && clean[j] != '\r' {
					clean[j] = ' '
				}
			}
			continue
		}
		i++
	}
	inString := false
	escaped := false
	for i, c := range clean {
		if inString {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			continue
		}
		if c == ',' {
			j := i + 1
			for j < len(clean) && strings.ContainsRune(" \r\n\t", rune(clean[j])) {
				j++
			}
			if j < len(clean) && (clean[j] == ']' || clean[j] == '}') {
				clean[i] = ' '
			}
		}
	}
	return clean, comments, nil
}
func Parse(text []byte) (Document, error) {
	d := Document{Text: bytes.Clone(text), folders: span{-1, -1}}
	clean, comments, err := sanitize(text)
	if err != nil {
		return d, err
	}
	d.comments = comments
	var raw map[string]json.RawMessage
	if json.Unmarshal(clean, &raw) != nil || raw == nil {
		return d, ErrInvalid
	}
	folders, ok := raw["folders"]
	if !ok || bytes.Equal(bytes.TrimSpace(folders), []byte("null")) {
		return d, ErrInvalid
	}
	if err := json.Unmarshal(folders, &d.Folders); err != nil || d.Folders == nil {
		return d, ErrInvalid
	}
	for _, f := range d.Folders {
		if f.Path == "" || strings.ContainsRune(f.Path, 0) {
			return d, ErrInvalid
		}
	}
	// Decoder offsets identify top-level values; duplicate keys are rejected.
	dec := json.NewDecoder(bytes.NewReader(clean))
	if _, err := dec.Token(); err != nil {
		return d, ErrInvalid
	}
	seen := map[string]bool{}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return d, ErrInvalid
		}
		name, ok := key.(string)
		if !ok || seen[name] {
			return d, ErrInvalid
		}
		seen[name] = true
		start := int(dec.InputOffset())
		for start < len(clean) && (clean[start] == ':' || strings.ContainsRune(" \t\r\n", rune(clean[start]))) {
			start++
		}
		var v json.RawMessage
		if dec.Decode(&v) != nil {
			return d, ErrInvalid
		}
		if name == "folders" {
			d.folders = span{start, int(dec.InputOffset())}
		}
	}
	return d, nil
}

// Edit changes only the folders value. Comments within the old array are kept
// inside the new array; all bytes outside that value are preserved verbatim.
func (d Document) Edit(folders []domain.WorkspaceFolder) ([]byte, error) {
	if folders == nil {
		folders = []domain.WorkspaceFolder{}
	}
	for _, f := range folders {
		if f.Path == "" || strings.ContainsRune(f.Path, 0) {
			return nil, ErrInvalid
		}
	}
	old, _ := json.Marshal(d.Folders)
	next, _ := json.Marshal(folders)
	if bytes.Equal(old, next) {
		return bytes.Clone(d.Text), nil
	}
	// Unknown per-folder properties cannot safely be rewritten by this DTO.
	clean, _, _ := sanitize(d.Text[d.folders.start:d.folders.end])
	var items []map[string]json.RawMessage
	if json.Unmarshal(clean, &items) != nil {
		return nil, ErrInvalid
	}
	for _, item := range items {
		for key := range item {
			if key != "path" && key != "name" {
				return nil, fmt.Errorf("%w: unsupported folder property", ErrInvalid)
			}
		}
	}
	encoded, err := json.MarshalIndent(folders, "  ", "  ")
	if err != nil {
		return nil, err
	}
	var kept strings.Builder
	for _, c := range d.comments {
		if c.start >= d.folders.start && c.end <= d.folders.end {
			kept.WriteString("\n    ")
			kept.Write(d.Text[c.start:c.end])
			kept.WriteByte('\n')
		}
	}
	if kept.Len() > 0 {
		encoded = append([]byte("["+kept.String()), encoded[1:]...)
	}
	result := append(bytes.Clone(d.Text[:d.folders.start]), encoded...)
	result = append(result, d.Text[d.folders.end:]...)
	if _, err = Parse(result); err != nil {
		return nil, err
	}
	return result, nil
}
