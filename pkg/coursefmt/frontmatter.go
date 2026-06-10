package coursefmt

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/goccy/go-yaml"
)

// ErrNoFrontmatter is returned when a markdown file has no leading
// YAML frontmatter block.
var ErrNoFrontmatter = errors.New("frontmatter: no leading --- block")

// SplitFrontmatter splits a markdown file into its YAML frontmatter and
// body. The format is:
//
//	---
//	yaml: data
//	---
//	body...
//
// The returned `yaml` does NOT include the surrounding `---` markers.
// The `body` starts immediately after the closing `---\n`.
func SplitFrontmatter(content []byte) (yamlPart, body []byte, err error) {
	// Strip optional UTF-8 BOM.
	c := bytes.TrimPrefix(content, []byte{0xEF, 0xBB, 0xBF})

	// Accept "---\n" or "---\r\n", or just "---" at EOF.
	if !bytes.HasPrefix(c, []byte("---")) {
		return nil, nil, ErrNoFrontmatter
	}

	// Skip past the opening delimiter line.
	firstNL := bytes.IndexByte(c, '\n')
	if firstNL < 0 {
		return nil, nil, ErrNoFrontmatter
	}
	rest := c[firstNL+1:]

	end := findClosingDelim(rest)
	if end < 0 {
		return nil, nil, ErrNoFrontmatter
	}

	yamlPart = rest[:end]

	// Skip past the closing "---" plus optional CR/LF.
	bodyStart := end + len("---")
	if bodyStart < len(rest) && rest[bodyStart] == '\r' {
		bodyStart++
	}
	if bodyStart < len(rest) && rest[bodyStart] == '\n' {
		bodyStart++
	}
	body = rest[bodyStart:]
	return yamlPart, body, nil
}

// findClosingDelim returns the index of "---" at the start of a line
// in s, or -1 if not found.
func findClosingDelim(s []byte) int {
	for i := 0; i < len(s); {
		if i+3 <= len(s) && s[i] == '-' && s[i+1] == '-' && s[i+2] == '-' {
			j := i + 3
			if j == len(s) || s[j] == '\n' || s[j] == '\r' {
				return i
			}
		}
		nl := bytes.IndexByte(s[i:], '\n')
		if nl < 0 {
			return -1
		}
		i += nl + 1
	}
	return -1
}

// JoinFrontmatter rebuilds a markdown file from its frontmatter YAML and body.
//
// Used by writers (the action's fix-ids mode); not used during read paths.
func JoinFrontmatter(w io.Writer, yamlPart, body []byte) error {
	if _, err := io.WriteString(w, "---\n"); err != nil {
		return err
	}
	yp := bytes.TrimRight(yamlPart, "\n")
	if _, err := w.Write(yp); err != nil {
		return err
	}
	if _, err := io.WriteString(w, "\n---\n"); err != nil {
		return err
	}
	if _, err := w.Write(body); err != nil {
		return err
	}
	return nil
}

// ParseFrontmatter decodes the YAML frontmatter into a LessonFrontmatter.
func ParseFrontmatter(yamlPart []byte) (LessonFrontmatter, error) {
	var fm LessonFrontmatter
	if err := yaml.Unmarshal(yamlPart, &fm); err != nil {
		return fm, fmt.Errorf("frontmatter: %w", err)
	}
	return fm, nil
}

// ParseFrontmatterTitle extracts just the title from a markdown file's
// YAML frontmatter. Returns empty string if parsing fails or title is absent.
func ParseFrontmatterTitle(content []byte) string {
	yamlPart, _, err := SplitFrontmatter(content)
	if err != nil {
		return ""
	}
	var fm LessonFrontmatter
	if err := yaml.Unmarshal(yamlPart, &fm); err != nil {
		return ""
	}
	return fm.Title
}
