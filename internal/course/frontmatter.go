package course

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"github.com/goccy/go-yaml"
)

// ErrNoFrontmatter indicates a markdown file has no YAML frontmatter block.
var ErrNoFrontmatter = errors.New("frontmatter: no leading --- block")

// SplitFrontmatter splits a markdown file into the YAML frontmatter and
// the body. If no frontmatter delimiter is found, returns ErrNoFrontmatter.
//
// The format is:
//
//	---
//	yaml: data
//	---
//	body...
//
// The returned `yaml` does NOT include the surrounding `---` markers.
// The `body` starts immediately after the closing `---\n`.
func SplitFrontmatter(content []byte) (yamlPart, body []byte, err error) {
	const delim = "---\n"

	// Allow optional UTF-8 BOM.
	bom := []byte{0xEF, 0xBB, 0xBF}
	c := bytes.TrimPrefix(content, bom)

	if !bytes.HasPrefix(c, []byte(delim)) && !bytes.HasPrefix(c, []byte("---\r\n")) {
		// Some editors use CRLF. Also accept a final --- without trailing newline
		// by checking just "---" + newline.
		if !bytes.HasPrefix(c, []byte("---")) {
			return nil, nil, ErrNoFrontmatter
		}
	}

	// Find the first newline after the leading ---
	firstNL := bytes.IndexByte(c, '\n')
	if firstNL < 0 {
		return nil, nil, ErrNoFrontmatter
	}
	rest := c[firstNL+1:]

	// Look for the closing --- on its own line.
	end := findClosingDelim(rest)
	if end < 0 {
		return nil, nil, ErrNoFrontmatter
	}

	yamlPart = rest[:end]
	// Skip past closing delimiter line.
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

// findClosingDelim returns the index in `s` of the start of a line
// equal to "---" (optionally followed by CR/LF or EOF), or -1.
func findClosingDelim(s []byte) int {
	for i := 0; i < len(s); {
		// Match "---" at the start of a line.
		if i+3 <= len(s) && s[i] == '-' && s[i+1] == '-' && s[i+2] == '-' {
			j := i + 3
			if j == len(s) || s[j] == '\n' || s[j] == '\r' {
				return i
			}
		}
		// Advance to the start of the next line.
		nl := bytes.IndexByte(s[i:], '\n')
		if nl < 0 {
			return -1
		}
		i += nl + 1
	}
	return -1
}

// JoinFrontmatter rebuilds a markdown file from frontmatter YAML and body.
func JoinFrontmatter(w io.Writer, yamlPart, body []byte) error {
	if _, err := io.WriteString(w, "---\n"); err != nil {
		return err
	}
	// Ensure exactly one trailing newline before the closing delimiter.
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

// ParseFrontmatter unmarshals the frontmatter YAML into a LessonFrontmatter.
func ParseFrontmatter(yamlPart []byte) (LessonFrontmatter, error) {
	var fm LessonFrontmatter
	if err := yaml.Unmarshal(yamlPart, &fm); err != nil {
		return fm, fmt.Errorf("frontmatter: %w", err)
	}
	return fm, nil
}
