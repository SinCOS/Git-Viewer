package local

import (
	"regexp"
	"strconv"
	"strings"
)

type diffCell struct {
	text   string
	line   int
	source int
	kind   byte
}

type diffRow struct {
	left, right diffCell
	shared      bool
}

var diffHunkHeader = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@`)

// diffRows retains source indices so find navigation works after changes are paired.
func diffRows(lines []string, sideBySide bool) ([]diffRow, []int) {
	rows := make([]diffRow, 0, len(lines))
	indices := make([]int, len(lines))
	oldLine, newLine := 0, 0
	oldRemaining, newRemaining := 0, 0
	inHunk := false
	var removed, added []diffCell
	appendRow := func(row diffRow) {
		index := len(rows)
		if row.left.source >= 0 {
			indices[row.left.source] = index
		}
		if row.right.source >= 0 {
			indices[row.right.source] = index
		}
		rows = append(rows, row)
	}
	empty := diffCell{source: -1}
	flush := func() {
		for i := 0; i < max(len(removed), len(added)); i++ {
			left, right := empty, empty
			if i < len(removed) {
				left = removed[i]
			}
			if i < len(added) {
				right = added[i]
			}
			appendRow(diffRow{left: left, right: right})
		}
		removed, added = nil, nil
	}
	for i, line := range lines {
		cell := diffCell{text: line, source: i}
		if m := diffHunkHeader.FindStringSubmatch(line); m != nil {
			flush()
			oldLine, _ = strconv.Atoi(m[1])
			newLine, _ = strconv.Atoi(m[3])
			oldRemaining, newRemaining = 1, 1
			if m[2] != "" {
				oldRemaining, _ = strconv.Atoi(m[2])
			}
			if m[4] != "" {
				newRemaining, _ = strconv.Atoi(m[4])
			}
			inHunk = oldRemaining > 0 || newRemaining > 0
			appendRow(diffRow{left: cell, right: empty, shared: true})
			continue
		}
		if strings.HasPrefix(line, "diff --git ") || strings.HasPrefix(line, "diff --cc ") || strings.HasPrefix(line, "diff --combined ") {
			inHunk = false
		}
		if inHunk && len(line) > 0 {
			switch line[0] {
			case ' ':
				if oldRemaining == 0 || newRemaining == 0 {
					break
				}
				oldRemaining--
				newRemaining--
				flush()
				left, right := cell, cell
				left.line, right.line = oldLine, newLine
				oldLine++
				newLine++
				if sideBySide {
					appendRow(diffRow{left: left, right: right})
				} else {
					appendRow(diffRow{left: left, right: right, shared: true})
				}
				continue
			case '-':
				if oldRemaining == 0 {
					break
				}
				oldRemaining--
				cell.kind, cell.line = '-', oldLine
				oldLine++
				if sideBySide {
					removed = append(removed, cell)
				} else {
					appendRow(diffRow{left: cell, right: empty, shared: true})
				}
				continue
			case '+':
				if newRemaining == 0 {
					break
				}
				newRemaining--
				cell.kind, cell.line = '+', newLine
				newLine++
				if sideBySide {
					added = append(added, cell)
				} else {
					appendRow(diffRow{left: empty, right: cell, shared: true})
				}
				continue
			}
		}
		flush()
		appendRow(diffRow{left: cell, right: empty, shared: true})
	}
	flush()
	return rows, indices
}

// singleFileRows omits the redundant Git file header when a textual hunk is
// already identified by the preview header. Keep raw text and find mappings.
func singleFileRows(rows []diffRow, indices []int) ([]diffRow, []int) {
	firstHunk := -1
	for i, row := range rows {
		if diffHunkHeader.MatchString(row.left.text) {
			firstHunk = i
			break
		}
	}
	if firstHunk < 0 {
		return rows, indices
	}
	remap := make([]int, len(rows))
	visible := make([]diffRow, 0, len(rows))
	for i, row := range rows {
		text := row.left.text
		header := i < firstHunk && (strings.HasPrefix(text, "diff --git ") || strings.HasPrefix(text, "index ") || strings.HasPrefix(text, "--- ") || strings.HasPrefix(text, "+++ "))
		remap[i] = len(visible)
		if !header {
			visible = append(visible, row)
		}
	}
	for i := range indices {
		indices[i] = min(remap[indices[i]], len(visible)-1)
	}
	return visible, indices
}
