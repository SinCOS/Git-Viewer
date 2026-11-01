package git

import "strings"

func addedTextFromPatch(data []byte) (string, bool) {
	var lines []string
	inHunk := false
	sawHunk := false
	noNewline := false
	for _, line := range strings.Split(string(data), "\n") {
		switch {
		case strings.HasPrefix(line, "@@ "):
			inHunk, sawHunk = true, true
		case strings.HasPrefix(line, "diff --git "):
			inHunk = false
		case !inHunk:
			continue
		case line == "\\ No newline at end of file":
			noNewline = true
		case strings.HasPrefix(line, "+"):
			lines = append(lines, strings.TrimSuffix(line[1:], "\r"))
		}
	}
	if !sawHunk {
		return "", true
	}
	text := strings.Join(lines, "\n")
	if !noNewline {
		text += "\n"
	}
	return text, true
}
