// Package cmd implements the `fix-ids` and `lint` subcommands of studyme-action.
package cmd

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wocki-wockitech/studyme-course-tools/internal/course"
	"github.com/wocki-wockitech/studyme-course-tools/pkg/coursefmt"
)

// FixIDs walks the course tree at root and fills missing `id:` fields
// in every relevant YAML file and lesson.md frontmatter.
//
// Returns the list of files that were modified.
//
// This function does NOT validate or load the course — it operates
// purely on text. That's intentional: a freshly-cloned repo with
// missing IDs may not be loadable yet, but we still need to fix it.
func FixIDs(root string) (changed []string, err error) {
	// Find every file we care about by walking the tree.
	files, err := discoverContentFiles(root)
	if err != nil {
		return nil, err
	}

	for _, p := range files {
		mod, err := fixOne(p)
		if err != nil {
			return changed, fmt.Errorf("%s: %w", p, err)
		}
		if mod {
			changed = append(changed, p)
		}
	}
	sort.Strings(changed)
	return changed, nil
}

// discoverContentFiles returns paths to every file the action understands.
// Skips dot-folders, underscore-folders (templates/drafts), and node_modules.
func discoverContentFiles(root string) ([]string, error) {
	var out []string

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()

		if d.IsDir() {
			// Skip ignored folders, but always descend into root.
			if path != root && shouldSkipDir(name) {
				return fs.SkipDir
			}
			return nil
		}

		switch name {
		case "course.yaml", "block.yaml", "challenge.yaml", "questions.yaml", "question.yaml", "card.yaml":
			out = append(out, path)
		case "lesson.md":
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func shouldSkipDir(name string) bool {
	if name == "" {
		return false
	}
	if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
		return true
	}
	// Common build/dependency folders that should never contain course content.
	switch name {
	case "node_modules", "vendor", "dist", "build", "target":
		return true
	}
	return false
}

// fixOne dispatches to the right handler based on filename.
// Returns true if the file was modified.
func fixOne(path string) (bool, error) {
	name := filepath.Base(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}

	switch name {
	case "course.yaml", "block.yaml", "challenge.yaml", "question.yaml", "card.yaml":
		out, changed, _ := course.SetTopLevelIDIfMissing(data)
		if !changed {
			return false, nil
		}
		return true, writeFile(path, out)

	case "questions.yaml":
		out, filled := course.SetQuestionsIDsIfMissing(data)
		if filled == 0 {
			return false, nil
		}
		return true, writeFile(path, out)

	case "lesson.md":
		out, changed, err := fixLessonFrontmatter(data)
		if err != nil {
			return false, err
		}
		if !changed {
			return false, nil
		}
		return true, writeFile(path, out)
	}
	return false, nil
}

// fixLessonFrontmatter splits the markdown into frontmatter+body, fills
// the id, and rejoins. Preserves body byte-for-byte.
func fixLessonFrontmatter(data []byte) ([]byte, bool, error) {
	yamlPart, body, err := coursefmt.SplitFrontmatter(data)
	if err != nil {
		// No frontmatter — synthesize a minimal one with just the id.
		// We do this so a freshly-created lesson.md without frontmatter
		// becomes valid after one PR.
		var buf bytes.Buffer
		fixed, _, _ := course.SetTopLevelIDIfMissing([]byte(""))
		if err := coursefmt.JoinFrontmatter(&buf, fixed, data); err != nil {
			return nil, false, err
		}
		return buf.Bytes(), true, nil
	}

	fixed, changed, _ := course.SetTopLevelIDIfMissing(yamlPart)
	if !changed {
		return data, false, nil
	}

	var buf bytes.Buffer
	if err := coursefmt.JoinFrontmatter(&buf, fixed, body); err != nil {
		return nil, false, err
	}
	return buf.Bytes(), true, nil
}

func writeFile(path string, data []byte) error {
	// Preserve original file mode if possible.
	info, err := os.Stat(path)
	mode := os.FileMode(0644)
	if err == nil {
		mode = info.Mode().Perm()
	}
	return os.WriteFile(path, data, mode)
}
