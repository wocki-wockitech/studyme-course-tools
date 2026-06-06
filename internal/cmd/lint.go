package cmd

import (
	"fmt"
	"sort"

	"github.com/google/uuid"

	"github.com/wockitech/studyme-action/internal/course"
)

// LintError is one validation problem reported by Lint.
type LintError struct {
	File    string `json:"file"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e LintError) String() string {
	if e.File == "" {
		return fmt.Sprintf("[%s] %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: [%s] %s", e.File, e.Code, e.Message)
}

// Lint validates the course at root. Returns a list of errors;
// an empty list means the course is valid.
func Lint(root string) ([]LintError, error) {
	var errs []LintError

	c, err := course.Load(root)
	if err != nil {
		return []LintError{{
			File:    root,
			Code:    "load_error",
			Message: err.Error(),
		}}, nil
	}

	errs = append(errs, validateCourse(c)...)
	errs = append(errs, validateUniqueIDs(c)...)
	errs = append(errs, validateReferences(c)...)
	errs = append(errs, validateQuestions(c)...)
	errs = append(errs, validateChallenges(c)...)

	sort.Slice(errs, func(i, j int) bool {
		if errs[i].File != errs[j].File {
			return errs[i].File < errs[j].File
		}
		return errs[i].Code < errs[j].Code
	})
	return errs, nil
}

func validateCourse(c *course.CourseRef) []LintError {
	var errs []LintError

	if c.Course.ID == "" {
		errs = append(errs, LintError{c.Path, "missing_id",
			"course.yaml: id is empty (run fix-ids to assign one)"})
	} else if !isUUID(c.Course.ID) {
		errs = append(errs, LintError{c.Path, "invalid_id",
			fmt.Sprintf("course.yaml: id %q is not a valid UUID", c.Course.ID)})
	}
	if c.Course.Slug == "" {
		errs = append(errs, LintError{c.Path, "missing_slug", "course.yaml: slug is empty"})
	} else if !isValidSlug(c.Course.Slug) {
		errs = append(errs, LintError{c.Path, "invalid_slug",
			fmt.Sprintf("course.yaml: slug %q must match [a-z0-9-]+", c.Course.Slug)})
	}
	if isEmptyTitle(c.Course.Title) {
		errs = append(errs, LintError{c.Path, "missing_title", "course.yaml: title is empty"})
	}
	if len(c.Course.Blocks) == 0 {
		errs = append(errs, LintError{c.Path, "no_blocks", "course.yaml: blocks list is empty"})
	}

	for _, b := range c.Blocks {
		if b.Block.ID == "" {
			errs = append(errs, LintError{b.Path, "missing_id",
				"block.yaml: id is empty (run fix-ids)"})
		} else if !isUUID(b.Block.ID) {
			errs = append(errs, LintError{b.Path, "invalid_id",
				fmt.Sprintf("block.yaml: id %q is not a valid UUID", b.Block.ID)})
		}
		if isEmptyTitle(b.Block.Title) {
			errs = append(errs, LintError{b.Path, "missing_title", "block.yaml: title is empty"})
		}
		if len(b.Block.Lessons) == 0 {
			errs = append(errs, LintError{b.Path, "no_lessons", "block.yaml: lessons list is empty"})
		}

		for _, l := range b.Lessons {
			if l.Frontmatter.ID == "" {
				errs = append(errs, LintError{l.Path, "missing_id",
					"lesson.md: frontmatter id is empty"})
			} else if !isUUID(l.Frontmatter.ID) {
				errs = append(errs, LintError{l.Path, "invalid_id",
					fmt.Sprintf("lesson.md: frontmatter id %q is not a valid UUID", l.Frontmatter.ID)})
			}
			if l.Frontmatter.Title == "" {
				errs = append(errs, LintError{l.Path, "missing_title",
					"lesson.md: frontmatter title is empty"})
			}
		}
	}
	return errs
}

func validateUniqueIDs(c *course.CourseRef) []LintError {
	var errs []LintError
	seen := map[string][]string{} // id → file paths

	track := func(id, file string) {
		if id == "" {
			return
		}
		seen[id] = append(seen[id], file)
	}

	track(c.Course.ID, c.Path)
	for _, b := range c.Blocks {
		track(b.Block.ID, b.Path)
		for _, l := range b.Lessons {
			track(l.Frontmatter.ID, l.Path)
			for _, q := range l.Questions {
				track(q.ID, l.Path+":questions["+q.Slug+"]")
			}
			for slug, qf := range l.QuestionFiles {
				track(qf.ID, l.Path+":questions/"+slug)
			}
			for chSlug, ch := range l.Challenges {
				track(ch.ID, l.Path+":challenges/"+chSlug)
			}
		}
	}

	for id, files := range seen {
		if len(files) > 1 {
			errs = append(errs, LintError{
				File:    files[0],
				Code:    "duplicate_id",
				Message: fmt.Sprintf("id %s is used in multiple places: %v", id, files),
			})
		}
	}
	return errs
}

func validateReferences(c *course.CourseRef) []LintError {
	var errs []LintError

	for _, b := range c.Blocks {
		for _, l := range b.Lessons {
			questionSlugs := map[string]bool{}
			for _, q := range l.Questions {
				questionSlugs[q.Slug] = true
			}
			for slug := range l.QuestionFiles {
				questionSlugs[slug] = true
			}
			challengeSlugs := map[string]bool{}
			for chSlug := range l.Challenges {
				challengeSlugs[chSlug] = true
			}

			// `> [!quiz] slug` — slug must exist in questions.yaml
			for _, ref := range l.QuizRefs {
				if !questionSlugs[ref] {
					errs = append(errs, LintError{
						File: l.Path, Code: "unknown_quiz_ref",
						Message: fmt.Sprintf("`> [!quiz] %s` references unknown question slug", ref),
					})
				}
			}

			// `> [!challenge] slug` — slug must exist in challenges/
			for _, ref := range l.ChallengeRefs {
				if !challengeSlugs[ref] {
					errs = append(errs, LintError{
						File: l.Path, Code: "unknown_challenge_ref",
						Message: fmt.Sprintf("`> [!challenge] %s` references unknown challenge folder", ref),
					})
				}
			}

			// Question of type=coding must reference an existing challenge.
			for _, q := range l.Questions {
				if (q.Type == "coding" || q.Type == "git_interactive") && q.ChallengeSlug != "" {
					if !challengeSlugs[q.ChallengeSlug] {
						errs = append(errs, LintError{
							File: l.Path, Code: "unknown_challenge_ref",
							Message: fmt.Sprintf("question %q references challenge %q which does not exist",
								q.Slug, q.ChallengeSlug),
						})
					}
				}
			}
		}
	}
	return errs
}

func validateQuestions(c *course.CourseRef) []LintError {
	var errs []LintError

	for _, b := range c.Blocks {
		for _, l := range b.Lessons {
			slugs := map[string]bool{}
			for _, q := range l.Questions {
				if q.ID == "" {
					errs = append(errs, LintError{
						File: l.Path, Code: "missing_id",
						Message: fmt.Sprintf("question %q has empty id", q.Slug),
					})
				} else if !isUUID(q.ID) {
					errs = append(errs, LintError{
						File: l.Path, Code: "invalid_id",
						Message: fmt.Sprintf("question id %q is not a valid UUID", q.ID),
					})
				}

				if q.Slug == "" {
					errs = append(errs, LintError{
						File: l.Path, Code: "missing_slug",
						Message: fmt.Sprintf("question id=%s has empty slug", q.ID),
					})
				} else if !isValidSlug(q.Slug) {
					errs = append(errs, LintError{
						File: l.Path, Code: "invalid_slug",
						Message: fmt.Sprintf("question slug %q must match [a-z0-9-]+", q.Slug),
					})
				} else if slugs[q.Slug] {
					errs = append(errs, LintError{
						File: l.Path, Code: "duplicate_slug",
						Message: fmt.Sprintf("question slug %q is used more than once in this lesson", q.Slug),
					})
				}
				slugs[q.Slug] = true

				if q.Text == "" {
					errs = append(errs, LintError{
						File: l.Path, Code: "missing_text",
						Message: fmt.Sprintf("question %q has empty text", q.Slug),
					})
				}

				switch q.Type {
				case "":
					errs = append(errs, LintError{
						File: l.Path, Code: "missing_type",
						Message: fmt.Sprintf("question %q has no type", q.Slug),
					})
				case "multiple_choice":
					if len(q.Options) < 2 {
						errs = append(errs, LintError{
							File: l.Path, Code: "invalid_options",
							Message: fmt.Sprintf("question %q: multiple_choice needs at least 2 options", q.Slug),
						})
					}
					hasCorrect := false
					for _, opt := range q.Options {
						if opt.Correct {
							hasCorrect = true
							break
						}
					}
					if !hasCorrect {
						errs = append(errs, LintError{
							File: l.Path, Code: "missing_correct",
							Message: fmt.Sprintf("question %q: multiple_choice needs at least one option with correct: true", q.Slug),
						})
					}
					for i, opt := range q.Options {
						if opt.Text == "" {
							errs = append(errs, LintError{
								File: l.Path, Code: "empty_option_text",
								Message: fmt.Sprintf("question %q: option %d has empty text", q.Slug, i),
							})
						}
					}
				case "true_false":
					if _, ok := q.Correct.(bool); !ok {
						errs = append(errs, LintError{
							File: l.Path, Code: "missing_correct",
							Message: fmt.Sprintf("question %q: true_false needs boolean `correct`", q.Slug),
						})
					}
				case "free_text":
					if q.ReferenceAnswer == "" {
						errs = append(errs, LintError{
							File: l.Path, Code: "missing_reference",
							Message: fmt.Sprintf("question %q: free_text needs reference_answer", q.Slug),
						})
					}
				case "coding", "git_interactive":
					if q.ChallengeSlug == "" {
						errs = append(errs, LintError{
							File: l.Path, Code: "missing_challenge_ref",
							Message: fmt.Sprintf("question %q: %s needs challenge_slug", q.Slug, q.Type),
						})
					}
				default:
					errs = append(errs, LintError{
						File: l.Path, Code: "unknown_type",
						Message: fmt.Sprintf("question %q has unknown type %q", q.Slug, q.Type),
					})
				}

				if q.Difficulty != 0 && (q.Difficulty < 1 || q.Difficulty > 5) {
					errs = append(errs, LintError{
						File: l.Path, Code: "invalid_difficulty",
						Message: fmt.Sprintf("question %q: difficulty must be 1-5, got %d", q.Slug, q.Difficulty),
					})
				}
			}

			// Validate new format: questions/ directory files.
			for slug, qf := range l.QuestionFiles {
				file := l.Path + ":questions/" + slug
				if !isValidSlug(slug) {
					errs = append(errs, LintError{
						File: file, Code: "invalid_slug",
						Message: fmt.Sprintf("question file name %q must match [a-z0-9-]+", slug),
					})
				}
				if slugs[slug] {
					errs = append(errs, LintError{
						File: file, Code: "duplicate_slug",
						Message: fmt.Sprintf("question slug %q conflicts with flat questions.yaml", slug),
					})
				}
				slugs[slug] = true

				if qf.ID == "" {
					errs = append(errs, LintError{
						File: file, Code: "missing_id",
						Message: fmt.Sprintf("question file %q has empty id", slug),
					})
				} else if !isUUID(qf.ID) {
					errs = append(errs, LintError{
						File: file, Code: "invalid_id",
						Message: fmt.Sprintf("question file id %q is not a valid UUID", qf.ID),
					})
				}

				if qf.Difficulty != 0 && (qf.Difficulty < 1 || qf.Difficulty > 5) {
					errs = append(errs, LintError{
						File: file, Code: "invalid_difficulty",
						Message: fmt.Sprintf("question file %q: difficulty must be 1-5, got %d", slug, qf.Difficulty),
					})
				}

				if len(qf.Variants) == 0 {
					errs = append(errs, LintError{
						File: file, Code: "no_variants",
						Message: fmt.Sprintf("question file %q has no variants", slug),
					})
				}

				for vi, v := range qf.Variants {
					errs = append(errs, validateVariant(file, slug, vi, v)...)
				}
			}
		}
	}
	return errs
}

func validateChallenges(c *course.CourseRef) []LintError {
	var errs []LintError

	for _, b := range c.Blocks {
		for _, l := range b.Lessons {
			for chSlug, ch := range l.Challenges {
				file := l.Path + ":challenges/" + chSlug

				if ch.ID == "" {
					errs = append(errs, LintError{
						File: file, Code: "missing_id",
						Message: "challenge.yaml: id is empty",
					})
				} else if !isUUID(ch.ID) {
					errs = append(errs, LintError{
						File: file, Code: "invalid_id",
						Message: fmt.Sprintf("challenge.yaml: id %q is not a valid UUID", ch.ID),
					})
				}
				if ch.Title == "" {
					errs = append(errs, LintError{
						File: file, Code: "missing_title",
						Message: "challenge.yaml: title is empty",
					})
				}
				if !isValidSlug(chSlug) {
					errs = append(errs, LintError{
						File: file, Code: "invalid_slug",
						Message: fmt.Sprintf("challenge folder name %q must match [a-z0-9-]+", chSlug),
					})
				}

				kind := ch.Type
				if kind == "" {
					if ch.Language != "" {
						kind = "coding"
					} else {
						errs = append(errs, LintError{
							File: file, Code: "missing_type",
							Message: "challenge.yaml: cannot determine type (set `type` or `language`)",
						})
						continue
					}
				}

				switch kind {
				case "coding":
					if ch.Language == "" {
						errs = append(errs, LintError{
							File: file, Code: "missing_language",
							Message: "challenge.yaml: coding challenge needs `language`",
						})
					}
					required := []string{"template", "tests", "solution"}
					for _, key := range required {
						if ch.Files[key] == "" {
							errs = append(errs, LintError{
								File: file, Code: "missing_file",
								Message: fmt.Sprintf("challenge.yaml: files.%s is required for coding challenges", key),
							})
						}
					}
				case "git_interactive":
					required := []string{"setup", "check"}
					for _, key := range required {
						if ch.Files[key] == "" {
							errs = append(errs, LintError{
								File: file, Code: "missing_file",
								Message: fmt.Sprintf("challenge.yaml: files.%s is required for git_interactive challenges", key),
							})
						}
					}
				default:
					errs = append(errs, LintError{
						File: file, Code: "unknown_type",
						Message: fmt.Sprintf("challenge.yaml: unknown type %q", kind),
					})
				}
			}
		}
	}
	return errs
}

func isUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

// validateVariant checks one variant in a question file.
func validateVariant(file, slug string, idx int, v course.Variant) []LintError {
	var errs []LintError
	prefix := fmt.Sprintf("question %q variant[%d]", slug, idx)

	if hasLocalizedText(v.Text) == "" {
		errs = append(errs, LintError{
			File: file, Code: "missing_text",
			Message: fmt.Sprintf("%s: text is empty", prefix),
		})
	}

	switch v.Type {
	case "":
		errs = append(errs, LintError{
			File: file, Code: "missing_type",
			Message: fmt.Sprintf("%s: type is empty", prefix),
		})
	case "multiple_choice":
		if len(v.Options) < 2 {
			errs = append(errs, LintError{
				File: file, Code: "invalid_options",
				Message: fmt.Sprintf("%s: multiple_choice needs at least 2 options", prefix),
			})
		}
		hasCorrect := false
		for _, opt := range v.Options {
			if opt.Correct {
				hasCorrect = true
				break
			}
		}
		if !hasCorrect {
			errs = append(errs, LintError{
				File: file, Code: "missing_correct",
				Message: fmt.Sprintf("%s: multiple_choice needs at least one option with correct: true", prefix),
			})
		}
		for i, opt := range v.Options {
			if hasLocalizedText(opt.Text) == "" {
				errs = append(errs, LintError{
					File: file, Code: "empty_option_text",
					Message: fmt.Sprintf("%s: option %d has empty text", prefix, i),
				})
			}
		}
	case "true_false":
		if v.Correct == nil {
			errs = append(errs, LintError{
				File: file, Code: "missing_correct",
				Message: fmt.Sprintf("%s: true_false needs `correct` field", prefix),
			})
		}
	case "ordering":
		if len(v.Items) < 2 {
			errs = append(errs, LintError{
				File: file, Code: "invalid_items",
				Message: fmt.Sprintf("%s: ordering needs at least 2 items", prefix),
			})
		}
	case "matching":
		if len(v.Pairs) < 2 {
			errs = append(errs, LintError{
				File: file, Code: "invalid_pairs",
				Message: fmt.Sprintf("%s: matching needs at least 2 pairs", prefix),
			})
		}
		for i, p := range v.Pairs {
			if hasLocalizedText(p.Left) == "" {
				errs = append(errs, LintError{
					File: file, Code: "empty_pair_text",
					Message: fmt.Sprintf("%s: pair %d has empty left", prefix, i),
				})
			}
			if hasLocalizedText(p.Right) == "" {
				errs = append(errs, LintError{
					File: file, Code: "empty_pair_text",
					Message: fmt.Sprintf("%s: pair %d has empty right", prefix, i),
				})
			}
		}
	case "categorize":
		if len(v.Categories) < 2 {
			errs = append(errs, LintError{
				File: file, Code: "invalid_categories",
				Message: fmt.Sprintf("%s: categorize needs at least 2 categories", prefix),
			})
		}
		for i, cat := range v.Categories {
			if hasLocalizedText(cat.Name) == "" {
				errs = append(errs, LintError{
					File: file, Code: "empty_category_name",
					Message: fmt.Sprintf("%s: category %d has empty name", prefix, i),
				})
			}
			if len(cat.Items) == 0 {
				errs = append(errs, LintError{
					File: file, Code: "empty_category_items",
					Message: fmt.Sprintf("%s: category %d has no items", prefix, i),
				})
			}
		}
	case "free_text":
		if hasLocalizedText(v.ReferenceAnswer) == "" {
			errs = append(errs, LintError{
				File: file, Code: "missing_reference",
				Message: fmt.Sprintf("%s: free_text needs reference_answer", prefix),
			})
		}
	case "coding", "git_interactive":
		if v.ChallengeSlug == "" {
			errs = append(errs, LintError{
				File: file, Code: "missing_challenge_ref",
				Message: fmt.Sprintf("%s: %s needs challenge_slug", prefix, v.Type),
			})
		}
	case "code_fill":
		if v.File == "" && v.FileContent == "" {
			errs = append(errs, LintError{
				File: file, Code: "missing_file",
				Message: fmt.Sprintf("%s: %s needs a code file (file: field)", prefix, v.Type),
			})
		}
		if len(v.Slots) == 0 {
			errs = append(errs, LintError{
				File: file, Code: "missing_slots",
				Message: fmt.Sprintf("%s: %s needs at least one slot", prefix, v.Type),
			})
		}
	default:
		errs = append(errs, LintError{
			File: file, Code: "unknown_type",
			Message: fmt.Sprintf("%s: unknown type %q", prefix, v.Type),
		})
	}
	return errs
}

// hasLocalizedText returns a non-empty string if the value contains text.
// Handles plain string and map[string]any / map[string]string (i18n).
func hasLocalizedText(val any) string {
	switch v := val.(type) {
	case string:
		return v
	case map[string]any:
		for _, s := range v {
			if str, ok := s.(string); ok && str != "" {
				return str
			}
		}
		return ""
	case map[string]string:
		for _, s := range v {
			if s != "" {
				return s
			}
		}
		return ""
	default:
		return ""
	}
}

func isEmptyTitle(v any) bool {
	if v == nil {
		return true
	}
	switch t := v.(type) {
	case string:
		return t == ""
	case map[string]any:
		return len(t) == 0
	case map[string]string:
		return len(t) == 0
	}
	return false
}

func isValidSlug(s string) bool {
	if len(s) < 1 || len(s) > 64 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-':
		default:
			return false
		}
	}
	return true
}
