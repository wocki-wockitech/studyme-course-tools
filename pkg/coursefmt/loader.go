package coursefmt

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

// Load is a convenience wrapper around LoadFromFS for real filesystem paths.
// It uses os.DirFS to open the directory and delegates to LoadFromFS.
func Load(root string) (*CourseRef, error) {
	return LoadFromFS(os.DirFS(root), root)
}

// LoadFromFS walks a course tree rooted at the given fs.FS and returns
// a populated CourseRef.
//
// Using fs.FS lets callers parse from disk (`os.DirFS(path)`), from a
// Git tarball stream (custom FS implementation), or from a unit-test
// in-memory FS — the loader doesn't care.
func LoadFromFS(fsys fs.FS, rootDescription string) (*CourseRef, error) {
	const courseFile = "course.yaml"

	data, err := fs.ReadFile(fsys, courseFile)
	if err != nil {
		return nil, fmt.Errorf("read course.yaml: %w", err)
	}

	var c Course
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse course.yaml: %w", err)
	}

	ref := &CourseRef{
		Root:     rootDescription,
		Path:     courseFile,
		Course:   c,
		LoadedAt: time.Now(),
	}

	for _, blockSlug := range c.Blocks {
		bref, err := loadBlock(fsys, blockSlug)
		if err != nil {
			return nil, fmt.Errorf("block %q: %w", blockSlug, err)
		}
		ref.Blocks = append(ref.Blocks, *bref)
	}
	return ref, nil
}

func loadBlock(fsys fs.FS, slug string) (*BlockRef, error) {
	blockPath := path.Join(slug, "block.yaml")
	data, err := fs.ReadFile(fsys, blockPath)
	if err != nil {
		return nil, fmt.Errorf("read block.yaml: %w", err)
	}

	var b Block
	if err := yaml.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("parse block.yaml: %w", err)
	}

	bref := &BlockRef{Slug: slug, Path: blockPath, Block: b}

	for _, lessonSlug := range b.Lessons {
		lref, err := loadLesson(fsys, slug, lessonSlug)
		if err != nil {
			return nil, fmt.Errorf("lesson %q: %w", lessonSlug, err)
		}
		bref.Lessons = append(bref.Lessons, *lref)
	}
	return bref, nil
}

func loadLesson(fsys fs.FS, blockSlug, lessonSlug string) (*LessonRef, error) {
	dir := path.Join(blockSlug, lessonSlug)
	lessonPath := path.Join(dir, "lesson.md")

	mdContent, err := fs.ReadFile(fsys, lessonPath)
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
		CardFiles:     map[string]CardFile{},
		QuestionFiles: map[string]CardFile{},
		Challenges:    map[string]Challenge{},
		Definitions:   map[string]DefinitionFile{},
		Sandboxes:     map[string]SandboxFile{},
	}

	lref.CardRefs = extractCalloutRefs(body, "card")
	lref.ChallengeRefs = extractCalloutRefs(body, "challenge")
	lref.SandboxRefs = extractCalloutRefs(body, "sandbox")

	// cards/ directory (one file per card with questions)
	// Supports both flat YAML files and subfolders with card.yaml + code files.
	cDir := path.Join(dir, "cards")
	cEntries, cErr := fs.ReadDir(fsys, cDir)
	if cErr != nil && !errIsNotExist(cErr) {
		return nil, fmt.Errorf("read cards/: %w", cErr)
	}
	for _, ce := range cEntries {
		name := ce.Name()

		if ce.IsDir() {
			// Subfolder format: cards/<slug>/card.yaml + code files
			slug := name
			cfPath := path.Join(cDir, slug, "card.yaml")
			cfData, readErr := fs.ReadFile(fsys, cfPath)
			if readErr != nil {
				if errIsNotExist(readErr) {
					continue // folder without card.yaml — skip
				}
				return nil, fmt.Errorf("read %s: %w", cfPath, readErr)
			}
			var cf CardFile
			if err := yaml.Unmarshal(cfData, &cf); err != nil {
				return nil, fmt.Errorf("parse %s: %w", cfPath, err)
			}
			// Load referenced code files for code_fill questions.
			for i := range cf.Questions {
				if cf.Questions[i].File != "" {
					codePath := path.Join(cDir, slug, cf.Questions[i].File)
					codeData, codeErr := fs.ReadFile(fsys, codePath)
					if codeErr != nil {
						return nil, fmt.Errorf("read code file %s: %w", codePath, codeErr)
					}
					cf.Questions[i].FileContent = string(codeData)
				}
			}
			lref.CardFiles[slug] = cf
			continue
		}

		// Flat file format: cards/<slug>.yaml
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		slug := strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
		cfPath := path.Join(cDir, name)
		cfData, readErr := fs.ReadFile(fsys, cfPath)
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", cfPath, readErr)
		}
		var cf CardFile
		if err := yaml.Unmarshal(cfData, &cf); err != nil {
			return nil, fmt.Errorf("parse %s: %w", cfPath, err)
		}
		lref.CardFiles[slug] = cf
	}

	// defs/ directory (one YAML file per glossary definition)
	dDir := path.Join(dir, "defs")
	dEntries, dErr := fs.ReadDir(fsys, dDir)
	if dErr != nil && !errIsNotExist(dErr) {
		return nil, fmt.Errorf("read defs/: %w", dErr)
	}
	if len(dEntries) > 0 {
		lref.Definitions = map[string]DefinitionFile{}
	}
	for _, de := range dEntries {
		name := de.Name()
		if de.IsDir() {
			continue
		}
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		slug := strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
		dfPath := path.Join(dDir, name)
		dfData, readErr := fs.ReadFile(fsys, dfPath)
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", dfPath, readErr)
		}
		var df DefinitionFile
		if err := yaml.Unmarshal(dfData, &df); err != nil {
			return nil, fmt.Errorf("parse %s: %w", dfPath, err)
		}
		lref.Definitions[slug] = df
	}

	// Backward compat: load questions.yaml (old flat format).
	qPath := path.Join(dir, "questions.yaml")
	if qData, qErr := fs.ReadFile(fsys, qPath); qErr == nil {
		var qf QuestionsFile
		if err := yaml.Unmarshal(qData, &qf); err != nil {
			return nil, fmt.Errorf("parse questions.yaml: %w", err)
		}
		lref.Questions = qf.Questions
	} else if !errIsNotExist(qErr) {
		return nil, fmt.Errorf("read questions.yaml: %w", qErr)
	}

	// Backward compat: load questions/ directory (old per-file format).
	qDir := path.Join(dir, "questions")
	qEntries, qErr := fs.ReadDir(fsys, qDir)
	if qErr != nil && !errIsNotExist(qErr) {
		return nil, fmt.Errorf("read questions/: %w", qErr)
	}
	for _, qe := range qEntries {
		name := qe.Name()

		if qe.IsDir() {
			slug := name
			qfPath := path.Join(qDir, slug, "question.yaml")
			qfData, readErr := fs.ReadFile(fsys, qfPath)
			if readErr != nil {
				if errIsNotExist(readErr) {
					continue
				}
				return nil, fmt.Errorf("read %s: %w", qfPath, readErr)
			}
			var qf CardFile
			if err := yaml.Unmarshal(qfData, &qf); err != nil {
				return nil, fmt.Errorf("parse %s: %w", qfPath, err)
			}
			for i := range qf.Questions {
				if qf.Questions[i].File != "" {
					codePath := path.Join(qDir, slug, qf.Questions[i].File)
					codeData, codeErr := fs.ReadFile(fsys, codePath)
					if codeErr != nil {
						return nil, fmt.Errorf("read code file %s: %w", codePath, codeErr)
					}
					qf.Questions[i].FileContent = string(codeData)
				}
			}
			lref.QuestionFiles[slug] = qf
			continue
		}

		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		slug := strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
		qfPath := path.Join(qDir, name)
		qfData, readErr := fs.ReadFile(fsys, qfPath)
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", qfPath, readErr)
		}
		var qf CardFile
		if err := yaml.Unmarshal(qfData, &qf); err != nil {
			return nil, fmt.Errorf("parse %s: %w", qfPath, err)
		}
		lref.QuestionFiles[slug] = qf
	}

	// Optional sandboxes/<slug>/sandbox.yaml folders
	sbRoot := path.Join(dir, "sandboxes")
	sbEntries, sbErr := fs.ReadDir(fsys, sbRoot)
	if sbErr != nil && !errIsNotExist(sbErr) {
		return nil, fmt.Errorf("read sandboxes/: %w", sbErr)
	}
	for _, e := range sbEntries {
		if !e.IsDir() {
			continue
		}
		sbSlug := e.Name()
		sbPath := path.Join(sbRoot, sbSlug, "sandbox.yaml")
		sbData, readErr := fs.ReadFile(fsys, sbPath)
		if readErr != nil {
			if errIsNotExist(readErr) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", sbPath, readErr)
		}
		var sb SandboxFile
		if err := yaml.Unmarshal(sbData, &sb); err != nil {
			return nil, fmt.Errorf("parse %s: %w", sbPath, err)
		}
		// Auto-discover files if not explicitly listed in sandbox.yaml.
		if len(sb.Files) == 0 {
			sb.Files = discoverSandboxFiles(fsys, path.Join(sbRoot, sbSlug))
		} else {
			// Load content from disk for entries without inline content.
			for i := range sb.Files {
				if sb.Files[i].Content == "" {
					filePath := path.Join(sbRoot, sbSlug, sb.Files[i].Path)
					fileData, fileErr := fs.ReadFile(fsys, filePath)
					if fileErr != nil && !errIsNotExist(fileErr) {
						return nil, fmt.Errorf("read sandbox file %s: %w", filePath, fileErr)
					}
					if fileErr == nil {
						sb.Files[i].Content = string(fileData)
					}
				}
			}
		}
		lref.Sandboxes[sbSlug] = sb
	}

	// Optional challenges/<slug>/challenge.yaml folders
	chRoot := path.Join(dir, "challenges")
	entries, err := fs.ReadDir(fsys, chRoot)
	if err != nil && !errIsNotExist(err) {
		return nil, fmt.Errorf("read challenges/: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		chSlug := e.Name()
		chPath := path.Join(chRoot, chSlug, "challenge.yaml")
		chData, readErr := fs.ReadFile(fsys, chPath)
		if readErr != nil {
			if errIsNotExist(readErr) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", chPath, readErr)
		}
		var ch Challenge
		if err := yaml.Unmarshal(chData, &ch); err != nil {
			return nil, fmt.Errorf("parse %s: %w", chPath, err)
		}
		lref.Challenges[chSlug] = ch
	}
	return lref, nil
}

// calloutRefRe matches Obsidian callout headers, e.g. `> [!card] some-slug`.
var calloutRefRe = regexp.MustCompile(`(?m)^>\s*\[!([\w-]+)\][+-]?\s*(.*)$`)

// extractCalloutRefs returns slugs referenced from callouts of the given type.
// Lines inside fenced code blocks (``` or ~~~) are skipped.
func extractCalloutRefs(body []byte, calloutType string) []string {
	// Strip fenced code blocks before searching for callouts.
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
// This prevents example code blocks from being parsed as real callout refs.
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
			// Close fence: same or longer sequence of the same char
			if strings.HasPrefix(trimmed, fenceMarker) && strings.TrimLeft(trimmed, string(fenceMarker[0])) == "" || trimmed == fenceMarker {
				inFence = false
			}
			// Skip line (inside code block)
		}
	}
	return []byte(strings.Join(out, "\n"))
}

// errIsNotExist reports whether err means "file or directory does not exist".
// All fs.FS implementations should wrap fs.ErrNotExist for this case.
func errIsNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}

// discoverSandboxFiles walks a sandbox directory and returns all non-YAML
// files as SandboxFileEntry values with content loaded from disk.
// sandbox.yaml itself is excluded.
func discoverSandboxFiles(fsys fs.FS, dir string) []SandboxFileEntry {
	var entries []SandboxFileEntry
	_ = fs.WalkDir(fsys, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		// Skip the manifest itself.
		name := path.Base(p)
		if name == "sandbox.yaml" || name == "sandbox.yml" {
			return nil
		}
		// Compute relative path from sandbox dir.
		rel := strings.TrimPrefix(p, dir+"/")
		if rel == p {
			rel = strings.TrimPrefix(p, dir)
		}
		data, readErr := fs.ReadFile(fsys, p)
		if readErr != nil {
			return nil
		}
		entries = append(entries, SandboxFileEntry{
			Path:    rel,
			Content: string(data),
		})
		return nil
	})
	return entries
}
