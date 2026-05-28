package course

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// SetTopLevelIDIfMissing sets the value of the top-level `id:` field
// in YAML text to a new UUID v4 if it's missing or empty. Returns the
// modified text and a bool indicating whether a change was made.
//
// Preserves comments, blank lines, indentation, and field order.
//
// "Top-level" means a key at indentation 0. This is correct for
// course.yaml, block.yaml, challenge.yaml, and frontmatter blocks.
func SetTopLevelIDIfMissing(yamlText []byte) (out []byte, changed bool, newID string) {
	scanner := bufio.NewScanner(bytes.NewReader(yamlText))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var buf bytes.Buffer
	idLineIdx := -1
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	for i, line := range lines {
		if isTopLevelIDLine(line) {
			idLineIdx = i
			break
		}
	}

	if idLineIdx == -1 {
		// No `id:` field at all — prepend one at the top.
		newID = uuid.NewString()
		buf.WriteString("id: " + newID + "\n")
		for _, l := range lines {
			buf.WriteString(l)
			buf.WriteByte('\n')
		}
		// Preserve trailing newline behavior.
		out = preserveTrailingNewline(yamlText, buf.Bytes())
		return out, true, newID
	}

	// `id:` is present. Check whether it has a value.
	if hasYAMLValue(lines[idLineIdx]) {
		// Already has a value — leave alone.
		return yamlText, false, ""
	}

	newID = uuid.NewString()
	lines[idLineIdx] = setYAMLValue(lines[idLineIdx], newID)
	for i, l := range lines {
		buf.WriteString(l)
		if i < len(lines)-1 {
			buf.WriteByte('\n')
		}
	}
	out = preserveTrailingNewline(yamlText, buf.Bytes())
	return out, true, newID
}

// isTopLevelIDLine reports whether a line declares the top-level `id` key.
// A top-level key has zero leading whitespace.
func isTopLevelIDLine(line string) bool {
	if len(line) == 0 || line[0] == ' ' || line[0] == '\t' {
		return false
	}
	// Strip optional UTF-8 BOM on first line.
	line = strings.TrimPrefix(line, "\uFEFF")
	// Match "id:" optionally followed by whitespace, value, or comment.
	if !strings.HasPrefix(line, "id") {
		return false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, "id"))
	return strings.HasPrefix(rest, ":")
}

// hasYAMLValue reports whether a "key: ..." line has a non-empty value.
// Trailing comments after `#` are ignored. Empty quoted strings count as
// "no value" (so the action will fill them).
func hasYAMLValue(line string) bool {
	colon := strings.Index(line, ":")
	if colon < 0 {
		return false
	}
	after := line[colon+1:]
	// Strip trailing comment.
	if hash := strings.Index(after, "#"); hash >= 0 {
		after = after[:hash]
	}
	v := strings.TrimSpace(after)
	if v == "" {
		return false
	}
	// Empty quoted strings.
	if v == `""` || v == `''` {
		return false
	}
	// Block scalars like `id: |` or `id: >` are not valid for our schema,
	// but if we see one, treat as having a value (don't touch).
	return true
}

// setYAMLValue rewrites "key: <value>" on the line, preserving the key,
// indentation (which should be zero for top-level), and any trailing comment.
func setYAMLValue(line, value string) string {
	colon := strings.Index(line, ":")
	if colon < 0 {
		return line
	}
	prefix := line[:colon+1] // "id:"
	rest := line[colon+1:]

	// Preserve trailing comment if any.
	var comment string
	if hash := strings.Index(rest, "#"); hash >= 0 {
		comment = rest[hash:]
	}

	if comment != "" {
		// Match the original spacing before the comment.
		return fmt.Sprintf("%s %s  %s", prefix, value, comment)
	}
	return fmt.Sprintf("%s %s", prefix, value)
}

// preserveTrailingNewline keeps the original file's trailing-newline state.
func preserveTrailingNewline(orig, modified []byte) []byte {
	hadFinalNL := bytes.HasSuffix(orig, []byte("\n"))
	hasFinalNL := bytes.HasSuffix(modified, []byte("\n"))
	switch {
	case hadFinalNL && !hasFinalNL:
		return append(modified, '\n')
	case !hadFinalNL && hasFinalNL:
		return modified[:len(modified)-1]
	default:
		return modified
	}
}

// SetQuestionsIDsIfMissing fills missing `id:` for every entry in a
// questions.yaml file. Operates on the text directly to preserve
// comments and formatting.
//
// A "questions entry" begins with a line matching `^( {0,4}- id\b|- id:|- ...)`
// in our format every entry starts with `  - id:`. We rely on this convention.
//
// Returns the number of IDs that were filled.
func SetQuestionsIDsIfMissing(yamlText []byte) (out []byte, filled int) {
	scanner := bufio.NewScanner(bytes.NewReader(yamlText))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	for i, line := range lines {
		if !isListItemIDLine(line) {
			continue
		}
		if hasYAMLValue(line) {
			continue
		}
		newID := uuid.NewString()
		lines[i] = setYAMLValue(line, newID)
		filled++
	}

	if filled == 0 {
		return yamlText, 0
	}

	var buf bytes.Buffer
	for i, l := range lines {
		buf.WriteString(l)
		if i < len(lines)-1 {
			buf.WriteByte('\n')
		}
	}
	return preserveTrailingNewline(yamlText, buf.Bytes()), filled
}

// isListItemIDLine matches lines of the form `<indent>- id:` (with any
// whitespace between `-` and `id`). This is the canonical format for
// the first key in a questions entry.
func isListItemIDLine(line string) bool {
	// Find the dash.
	trimmed := strings.TrimLeft(line, " \t")
	if !strings.HasPrefix(trimmed, "-") {
		return false
	}
	rest := strings.TrimLeft(trimmed[1:], " \t")
	if !strings.HasPrefix(rest, "id") {
		return false
	}
	afterID := strings.TrimSpace(strings.TrimPrefix(rest, "id"))
	return strings.HasPrefix(afterID, ":")
}
