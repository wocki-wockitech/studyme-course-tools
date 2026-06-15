// Package coursefmt defines the on-disk format of a StudyMe course
// repository and a loader that parses it into typed structures.
//
// The format is shared between two consumers:
//
//   - studyme-action (CI: validates and auto-fills missing UUIDs)
//   - StudyMe backend (sync pipeline: indexes a Git tag into the platform)
//
// Keeping the parser here ensures both tools agree on the format.
//
// File layout (single-language; see package docs for multi-language):
//
//	course.yaml
//	<block-slug>/
//	  block.yaml
//	  <lesson-slug>/
//	    lesson.md            ← markdown with YAML frontmatter
//	    cards/               ← one YAML file per card (with questions)
//	      <slug>.yaml
//	    challenges/
//	      <challenge-slug>/
//	        challenge.yaml
//	        ... (language-specific files)
package coursefmt

import (
	"fmt"
	"time"
)

// Course is the top-level manifest parsed from course.yaml.
//
// Translatable display fields (Title, Description) accept both plain strings
// (uses default_language) and localized maps {lang: text}.
type Course struct {
	ID    string        `yaml:"id"`
	Slug  string        `yaml:"slug"`
	Title LocalizedText `yaml:"title"`

	// Description / language metadata shown in the catalog.
	Description LocalizedText `yaml:"description"`
	Difficulty  string        `yaml:"difficulty"`
	Tags        []string      `yaml:"tags"`
	Cover       string        `yaml:"cover"`
	License     string        `yaml:"license"`
	Authors     []Author      `yaml:"authors"`

	// Multi-language fields. `default_language` is the language used
	// when a translation is missing; `languages` is the full set the
	// course is translated into. If neither is set, the deprecated
	// `language` field is used as default and the only language.
	DefaultLanguage string   `yaml:"default_language"`
	Languages       []string `yaml:"languages"`
	Language        string   `yaml:"language"` // deprecated single-language form

	// Block slugs in display order.
	Blocks []string `yaml:"blocks"`
}

// Author is one display credit in course.yaml.
type Author struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// Block is the manifest parsed from block.yaml.
//
// Title and Description are translatable (string or {lang: text} map);
// everything else is structural and shared across all languages.
type Block struct {
	ID          string        `yaml:"id"`
	Title       LocalizedText `yaml:"title"`
	Description LocalizedText `yaml:"description"`
	Lessons     []string      `yaml:"lessons"`
	Test        BlockTest     `yaml:"test"`
}

// BlockTest configures the end-of-block exam.
type BlockTest struct {
	QuestionCount        int     `yaml:"question_count"`
	PassThreshold        float64 `yaml:"pass_threshold"`
	PreferFailed         bool    `yaml:"prefer_failed"`
	TimeLimitPerQuestion *int    `yaml:"time_limit_per_question,omitempty"`
}

// LessonFrontmatter is the YAML block at the top of lesson.md.
type LessonFrontmatter struct {
	ID               string   `yaml:"id"`
	Title            string   `yaml:"title"`
	EstimatedMinutes int      `yaml:"estimated_minutes,omitempty"`
	Tags             []string `yaml:"tags,omitempty"`
}

// Option is one multiple-choice answer option (legacy flat format).
//
// Deprecated: Use RichOption in CardQuestion instead.
type Option struct {
	Text     string `yaml:"text" json:"text"`
	Correct  bool   `yaml:"correct,omitempty" json:"correct,omitempty"`
	Feedback string `yaml:"feedback,omitempty" json:"feedback,omitempty"`
}

// Challenge is the manifest parsed from challenge.yaml.
type Challenge struct {
	ID          string            `yaml:"id"`
	Title       string            `yaml:"title"`
	Description string            `yaml:"description"`
	Language    string            `yaml:"language,omitempty"` // code language: go, python, …
	Type        string            `yaml:"type,omitempty"`     // coding (default) | git_interactive
	TimeoutSec  int               `yaml:"timeout_sec,omitempty"`
	Limits      map[string]any    `yaml:"limits,omitempty"`
	Files       map[string]string `yaml:"files,omitempty"`
	Hints       []string          `yaml:"hints,omitempty"`
	Comparison  map[string]any    `yaml:"comparison,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────
// cards/ directory format (one file per card with questions)
// ─────────────────────────────────────────────────────────────────────

// CardFile is the structure of a single card file in cards/<slug>.yaml.
// It supports multiple questions (alternative formulations) of the same concept.
type CardFile struct {
	ID         string         `yaml:"id"`
	Difficulty int            `yaml:"difficulty"`
	Tags       []string       `yaml:"tags,omitempty"`
	Title      string         `yaml:"title,omitempty"`
	Questions  []CardQuestion `yaml:"questions"`
}

// CardQuestion is one question formulation within a CardFile.
// (Formerly "Variant")
type CardQuestion struct {
	Type               string         `yaml:"type"`
	Text               any            `yaml:"text"`                          // string | map[string]string
	Options            []RichOption   `yaml:"options,omitempty"`             // multiple_choice
	Correct            any            `yaml:"correct,omitempty"`             // true_false
	Items              []any          `yaml:"items,omitempty"`               // ordering: []string or []{lang:text}
	Pairs              []Pair         `yaml:"pairs,omitempty"`               // matching
	Categories         []Category     `yaml:"categories,omitempty"`          // categorize
	Distractors        []any          `yaml:"distractors,omitempty"`         // matching/categorize
	ReferenceAnswer    any            `yaml:"reference_answer,omitempty"`    // string | map[string]string
	EvaluationCriteria map[string]any `yaml:"evaluation_criteria,omitempty"` // free_text
	MaxScore           int            `yaml:"max_score,omitempty"`
	PassThreshold      float64        `yaml:"pass_threshold,omitempty"`
	ChallengeSlug      string         `yaml:"challenge_slug,omitempty"` // coding/git_interactive

	// code_fill
	File        string `yaml:"file,omitempty"`  // filename of code file with ___ markers
	FileContent string `yaml:"-"`               // loaded content (not from YAML, loaded by reader)
	Slots       []Slot `yaml:"slots,omitempty"` // per-slot answers, hints, explanations
}

// AllowMultiple reports whether this multiple_choice question accepts
// more than one answer (i.e. has multiple options marked correct).
func (q CardQuestion) AllowMultiple() bool {
	count := 0
	for _, o := range q.Options {
		if o.Correct {
			count++
		}
	}
	return count > 1
}

// Slot describes one fill-in slot (___) in a code_fill question.
type Slot struct {
	Answer      []string `yaml:"answer"`                // acceptable answers for this slot
	Hints       []any    `yaml:"hints,omitempty"`       // []string or []{lang:text}
	Explanation any      `yaml:"explanation,omitempty"` // string or {lang:text}
}

// RichOption is one multiple-choice answer option supporting i18n.
type RichOption struct {
	Text     any  `yaml:"text"` // string | map[string]string
	Correct  bool `yaml:"correct,omitempty"`
	Feedback any  `yaml:"feedback,omitempty"` // string | map[string]string
}

// Pair is a left-right matching pair supporting i18n.
type Pair struct {
	Left  any `yaml:"left"`  // string | map[string]string
	Right any `yaml:"right"` // string | map[string]string
}

// Category is one group in a categorize question.
type Category struct {
	Name  any   `yaml:"name"`  // string | map[string]string
	Items []any `yaml:"items"` // []string or []{lang:text}
}

// LocalizedText is a YAML-friendly type that accepts both a plain string
// and a map of language → string. Implements yaml.Unmarshaler.
//
//	title: "Мой курс"                    → LocalizedText{values: {"": "Мой курс"}}
//	title:
//	  ru: "Мой курс"
//	  en: "My Course"                    → LocalizedText{values: {"ru": "Мой курс", "en": "My Course"}}
type LocalizedText struct {
	values map[string]string
}

// Resolve returns text for the given language with fallback.
// For plain strings (no lang keys), always returns the string regardless of lang.
func (lt LocalizedText) Resolve(lang, defaultLang string) string {
	if lt.values == nil {
		return ""
	}
	// Plain string case — stored with empty key.
	if s, ok := lt.values[""]; ok {
		return s
	}
	if s, ok := lt.values[lang]; ok && s != "" {
		return s
	}
	if s, ok := lt.values[defaultLang]; ok && s != "" {
		return s
	}
	// Any first value as last resort.
	for _, s := range lt.values {
		return s
	}
	return ""
}

// Languages returns all language keys (empty for plain strings).
func (lt LocalizedText) Languages() []string {
	if lt.values == nil {
		return nil
	}
	if _, ok := lt.values[""]; ok {
		return nil // plain string, no explicit languages
	}
	var langs []string
	for k := range lt.values {
		langs = append(langs, k)
	}
	return langs
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (lt *LocalizedText) UnmarshalYAML(unmarshal func(any) error) error {
	// Try string first.
	var s string
	if err := unmarshal(&s); err == nil {
		lt.values = map[string]string{"": s}
		return nil
	}
	// Try map.
	var m map[string]string
	if err := unmarshal(&m); err == nil {
		lt.values = m
		return nil
	}
	return fmt.Errorf("LocalizedText: expected string or map[string]string")
}

// ResolveLocalizedString extracts the text for a given language from a
// LocalizedString value (either a plain string or map[string]string).
// If the value is a plain string, it is returned directly. If it's a
// map, the language key is looked up with a fallback to defaultLang.
func ResolveLocalizedString(val any, lang, defaultLang string) string {
	switch v := val.(type) {
	case string:
		return v
	case map[string]any:
		if s, ok := v[lang]; ok {
			if str, ok := s.(string); ok {
				return str
			}
		}
		if s, ok := v[defaultLang]; ok {
			if str, ok := s.(string); ok {
				return str
			}
		}
		// Return any first value as last resort.
		for _, s := range v {
			if str, ok := s.(string); ok {
				return str
			}
		}
		return ""
	case map[string]string:
		if s, ok := v[lang]; ok {
			return s
		}
		if s, ok := v[defaultLang]; ok {
			return s
		}
		for _, s := range v {
			return s
		}
		return ""
	default:
		return ""
	}
}

// DefaultLangText extracts the default language text from a LocalizedString value.
// Used for deterministic item ID generation.
func DefaultLangText(val any, defaultLang string) string {
	return ResolveLocalizedString(val, defaultLang, defaultLang)
}

// ─────────────────────────────────────────────────────────────────────
// defs/ directory format (one file per glossary definition)
// ─────────────────────────────────────────────────────────────────────

// DefinitionFile is the structure of a single definition in defs/<slug>.yaml.
// It supports LocalizedText for term, definition, and example fields, allowing
// both plain strings (single-language) and {lang: text} maps (multi-language).
type DefinitionFile struct {
	ID         string        `yaml:"id"`
	Term       LocalizedText `yaml:"term"`
	Aliases    []string      `yaml:"aliases"`
	Tags       []string      `yaml:"tags,omitempty"`
	Definition LocalizedText `yaml:"definition"`
	Example    LocalizedText `yaml:"example"`
	Related    []string      `yaml:"related,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────
// Legacy flat format (deprecated — kept for backward compat in CI)
// ─────────────────────────────────────────────────────────────────────

// QuestionsFile is the structure of questions.yaml (old flat format).
//
// Deprecated: Use CardFile in the cards/ directory instead.
type QuestionsFile struct {
	Questions []Question `yaml:"questions"`
}

// Question represents one entry in questions.yaml (old flat format).
//
// Deprecated: Use CardFile/CardQuestion in the cards/ directory instead.
type Question struct {
	ID              string   `yaml:"id"`
	Slug            string   `yaml:"slug"`
	Type            string   `yaml:"type"`
	Difficulty      int      `yaml:"difficulty"`
	Text            string   `yaml:"text"`
	Tags            []string `yaml:"tags,omitempty"`
	ReferenceAnswer string   `yaml:"reference_answer,omitempty"`
	// type=multiple_choice
	Options []Option `yaml:"options,omitempty"`
	// type=true_false
	Correct any `yaml:"correct,omitempty"`
	// type=free_text
	EvaluationCriteria map[string]any `yaml:"evaluation_criteria,omitempty"`
	MaxScore           int            `yaml:"max_score,omitempty"`
	PassThreshold      float64        `yaml:"pass_threshold,omitempty"`
	// type=coding | git_interactive
	ChallengeSlug string `yaml:"challenge_slug,omitempty"`
}

// CorrectIndices returns 0-based indices of correct options.
func (q Question) CorrectIndices() []int {
	var out []int
	for i, o := range q.Options {
		if o.Correct {
			out = append(out, i)
		}
	}
	return out
}

// AllowMultiple reports whether this multiple_choice question has more
// than one correct answer.
func (q Question) AllowMultiple() bool {
	return len(q.CorrectIndices()) > 1
}

// ─────────────────────────────────────────────────────────────────────
// Loaded tree (a parsed snapshot of a course on disk)
// ─────────────────────────────────────────────────────────────────────

// LessonRef is a discovered lesson with its parsed contents.
type LessonRef struct {
	BlockSlug   string
	LessonSlug  string
	Path        string // path to lesson.md (default language file)
	Frontmatter LessonFrontmatter
	CardFiles   map[string]CardFile       // from cards/<slug>.yaml (keyed by slug)
	Challenges  map[string]Challenge      // by challenge slug
	Definitions map[string]DefinitionFile // from defs/<slug>.yaml (keyed by slug)
	// CardRefs lists slugs referenced from `> [!card] slug` callouts.
	CardRefs []string
	// ChallengeRefs lists slugs referenced from `> [!challenge] slug`.
	ChallengeRefs []string

	// Deprecated: Questions is from questions.yaml (old flat format).
	// Kept for backward compat in fix-ids and lint modes.
	Questions []Question
	// Deprecated: QuestionFiles from questions/<slug>.yaml — replaced by CardFiles.
	QuestionFiles map[string]CardFile
}

// BlockRef is a discovered block with its parsed manifest and lessons.
type BlockRef struct {
	Slug    string
	Path    string // path to block.yaml
	Block   Block
	Lessons []LessonRef // in the order specified by Block.Lessons
}

// CourseRef is the full course tree loaded from disk.
type CourseRef struct {
	Root     string // filesystem root from which the course was loaded
	Path     string // path to course.yaml
	Course   Course
	Blocks   []BlockRef // in the order specified by Course.Blocks
	LoadedAt time.Time
}

// EffectiveDefaultLanguage returns the course's default language with
// backwards-compatibility fallback to the deprecated `language` field.
// Returns "ru" if neither is set (matches platform default).
func (c Course) EffectiveDefaultLanguage() string {
	switch {
	case c.DefaultLanguage != "":
		return c.DefaultLanguage
	case c.Language != "":
		return c.Language
	default:
		return "ru"
	}
}

// EffectiveLanguages returns the full language set, falling back to a
// single-element list of the default language if `languages` is empty.
func (c Course) EffectiveLanguages() []string {
	if len(c.Languages) > 0 {
		return c.Languages
	}
	return []string{c.EffectiveDefaultLanguage()}
}
