package course

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

// Load walks the course tree starting at root and returns a populated
// CourseRef. Errors during traversal are returned as a multi-error.
func Load(root string) (*CourseRef, error) {
	coursePath := filepath.Join(root, "course.yaml")
	data, err := os.ReadFile(coursePath)
	if err != nil {
		return nil, fmt.Errorf("read course.yaml: %w", err)
	}

	var c Course
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse course.yaml: %w", err)
	}

	ref := &CourseRef{
		Path:     coursePath,
		Course:   c,
		LoadedAt: time.Now(),
	}

	for _, blockSlug := range c.Blocks {
		bref, err := loadBlock(root, blockSlug)
		if err != nil {
			return nil, fmt.Errorf("block %q: %w", blockSlug, err)
		}
		ref.Blocks = append(ref.Blocks, *bref)
	}
	return ref, nil
}

func loadBlock(root, slug string) (*BlockRef, error) {
	blockPath := filepath.Join(root, slug, "block.yaml")
	data, err := os.ReadFile(blockPath)
	if err != nil {
		return nil, fmt.Errorf("read block.yaml: %w", err)
	}

	var b Block
	if err := yaml.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("parse block.yaml: %w", err)
	}

	bref := &BlockRef{Slug: slug, Path: blockPath, Block: b}

	for _, lessonSlug := range b.Lessons {
		lref, err := loadLesson(root, slug, lessonSlug)
		if err != nil {
			return nil, fmt.Errorf("lesson %q: %w", lessonSlug, err)
		}
		bref.Lessons = append(bref.Lessons, *lref)
	}
	return bref, nil
}

func loadLesson(root, blockSlug, lessonSlug string) (*LessonRef, error) {
	dir := filepath.Join(root, blockSlug, lessonSlug)
	lessonPath := filepath.Join(dir, "lesson.md")

	mdContent, err := os.ReadFile(lessonPath)
	if err != nil {
		return nil, fmt.Errorf("read lesson.md: %w", err)
	}

	yamlPart, body, err := SplitFrontmatter(mdContent)
	if err != nil {
		return nil, fmt.Errorf("frontmatter: %w", err)
	}

	fm, err := ParseFrontmatter(yamlPart)
	if err != nil {
		return nil, err
	}

	lref := &LessonRef{
		BlockSlug:     blockSlug,
		LessonSlug:    lessonSlug,
		Path:          lessonPath,
		Frontmatter:   fm,
		QuestionFiles: map[string]QuestionFile{},
		Challenges:    map[string]Challenge{},
	}

	// Extract widget references from markdown body.
	lref.QuizRefs = extractCalloutRefs(body, "quiz")
	lref.ChallengeRefs = extractCalloutRefs(body, "challenge")

	// Load questions.yaml if present.
	qPath := filepath.Join(dir, "questions.yaml")
	if qData, err := os.ReadFile(qPath); err == nil {
		var qf QuestionsFile
		if err := yaml.Unmarshal(qData, &qf); err != nil {
			return nil, fmt.Errorf("parse questions.yaml: %w", err)
		}
		lref.Questions = qf.Questions
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read questions.yaml: %w", err)
	}

	// Load questions/ directory if present (new format: one file per question).
	qDir := filepath.Join(dir, "questions")
	qEntries, qErr := os.ReadDir(qDir)
	if qErr != nil && !os.IsNotExist(qErr) {
		return nil, fmt.Errorf("read questions/: %w", qErr)
	}
	for _, qe := range qEntries {
		if qe.IsDir() {
			continue
		}
		name := qe.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		slug := strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
		qfPath := filepath.Join(qDir, name)
		qfData, readErr := os.ReadFile(qfPath)
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", qfPath, readErr)
		}
		var qf QuestionFile
		if err := yaml.Unmarshal(qfData, &qf); err != nil {
			return nil, fmt.Errorf("parse %s: %w", qfPath, err)
		}
		lref.QuestionFiles[slug] = qf
	}

	// Load challenges/<slug>/challenge.yaml for each challenge folder.
	chDir := filepath.Join(dir, "challenges")
	entries, err := os.ReadDir(chDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read challenges/: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		chSlug := e.Name()
		chPath := filepath.Join(chDir, chSlug, "challenge.yaml")
		chData, err := os.ReadFile(chPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue // folder without challenge.yaml — skip
			}
			return nil, fmt.Errorf("read %s: %w", chPath, err)
		}
		var ch Challenge
		if err := yaml.Unmarshal(chData, &ch); err != nil {
			return nil, fmt.Errorf("parse %s: %w", chPath, err)
		}
		lref.Challenges[chSlug] = ch
	}
	return lref, nil
}

// calloutRefRe matches Obsidian callout headers like:
//
//	> [!quiz] some-slug
//	> [!challenge]+ another-slug
//
// Capturing the metadata after the type marker.
var calloutRefRe = regexp.MustCompile(`(?m)^>\s*\[!([\w-]+)\][+-]?\s*(.*)$`)

// extractCalloutRefs returns slugs referenced from callouts of the given type.
// Lines inside fenced code blocks (``` or ~~~) are skipped.
func extractCalloutRefs(body []byte, calloutType string) []string {
	cleaned := stripFencedCodeBlocks(body)

	var refs []string
	matches := calloutRefRe.FindAllStringSubmatch(string(cleaned), -1)
	for _, m := range matches {
		if !strings.EqualFold(m[1], calloutType) {
			continue
		}
		slug := strings.TrimSpace(m[2])
		if slug == "" {
			continue
		}
		refs = append(refs, slug)
	}
	return refs
}

// stripFencedCodeBlocks removes content between ``` or ~~~ fence markers.
func stripFencedCodeBlocks(body []byte) []byte {
	lines := strings.Split(string(body), "\n")
	var out []string
	inFence := false
	var fenceMarker string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inFence {
			if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
				inFence = true
				fenceMarker = trimmed[:3]
				continue
			}
			out = append(out, line)
		} else {
			if strings.HasPrefix(trimmed, fenceMarker) {
				inFence = false
			}
		}
	}
	return []byte(strings.Join(out, "\n"))
}
