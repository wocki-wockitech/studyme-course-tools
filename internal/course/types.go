// Package course defines the on-disk structure of a StudyMe course
// and the types used throughout the action.
package course

import "time"

// Course is the top-level manifest from course.yaml.
type Course struct {
	ID          string   `yaml:"id"`
	Slug        string   `yaml:"slug"`
	Title       any      `yaml:"title"`       // string | map[string]string (i18n)
	Description any      `yaml:"description"` // string | map[string]string (i18n)
	Language    string   `yaml:"language"`
	Difficulty  string   `yaml:"difficulty"`
	Tags        []string `yaml:"tags"`
	Cover       string   `yaml:"cover"`
	License     string   `yaml:"license"`
	Authors     []Author `yaml:"authors"`
	Blocks      []string `yaml:"blocks"`

	DefaultLanguage string   `yaml:"default_language"`
	Languages       []string `yaml:"languages"`
}

type Author struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// Block is the manifest from block.yaml.
type Block struct {
	ID          string    `yaml:"id"`
	Title       any       `yaml:"title"`       // string | map[string]string (i18n)
	Description any       `yaml:"description"` // string | map[string]string (i18n)
	Lessons     []string  `yaml:"lessons"`
	Test        BlockTest `yaml:"test"`
}

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

// QuestionsFile is the structure of questions.yaml.
type QuestionsFile struct {
	Questions []Question `yaml:"questions"`
}

// QuestionFile is the structure of a single question file in questions/<slug>.yaml.
// New format with variants support.
type QuestionFile struct {
	ID         string    `yaml:"id"`
	Difficulty int       `yaml:"difficulty"`
	Tags       []string  `yaml:"tags,omitempty"`
	Variants   []Variant `yaml:"variants"`
}

// Variant is one formulation of a question within a QuestionFile.
type Variant struct {
	Type               string         `yaml:"type"`
	Text               any            `yaml:"text"`                          // string | map[string]string
	Options            []RichOption   `yaml:"options,omitempty"`             // multiple_choice
	Correct            any            `yaml:"correct,omitempty"`             // true_false
	Items              []any          `yaml:"items,omitempty"`               // ordering
	Pairs              []Pair         `yaml:"pairs,omitempty"`               // matching
	Categories         []Category     `yaml:"categories,omitempty"`          // categorize
	Distractors        []any          `yaml:"distractors,omitempty"`         // matching/categorize
	ReferenceAnswer    any            `yaml:"reference_answer,omitempty"`    // string | map[string]string
	EvaluationCriteria map[string]any `yaml:"evaluation_criteria,omitempty"` // free_text
	MaxScore           int            `yaml:"max_score,omitempty"`
	PassThreshold      float64        `yaml:"pass_threshold,omitempty"`
	ChallengeSlug      string         `yaml:"challenge_slug,omitempty"`

	// code_fill / code_fill
	File        string `yaml:"file,omitempty"`  // filename of code file with ___ markers
	FileContent string `yaml:"-"`               // loaded content (not from YAML, loaded by reader)
	Slots       []Slot `yaml:"slots,omitempty"` // per-slot answers, hints, explanations
}

// Slot describes one fill-in slot (___) in a code_fill question.
type Slot struct {
	Answer      []string `yaml:"answer"`                // acceptable answers for this slot
	Hints       []any    `yaml:"hints,omitempty"`       // []string or []{lang:text}
	Explanation any      `yaml:"explanation,omitempty"` // string or {lang:text}
}

// RichOption is a multiple-choice option supporting i18n.
type RichOption struct {
	Text     any  `yaml:"text"` // string | map[string]string
	Correct  bool `yaml:"correct,omitempty"`
	Feedback any  `yaml:"feedback,omitempty"` // string | map[string]string
}

// Pair is a left-right matching pair.
type Pair struct {
	Left  any `yaml:"left"`  // string | map[string]string
	Right any `yaml:"right"` // string | map[string]string
}

// Category is one group in a categorize question.
type Category struct {
	Name  any   `yaml:"name"`  // string | map[string]string
	Items []any `yaml:"items"` // []string or []{lang:text}
}

// Option is one rich multiple-choice answer option.
type Option struct {
	Text     string `yaml:"text"`
	Correct  bool   `yaml:"correct,omitempty"`
	Feedback string `yaml:"feedback,omitempty"`
}

// Question represents one entry in questions.yaml.
// Type-specific fields are kept as raw maps because they vary per type.
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

// Challenge is the manifest from challenge.yaml.
type Challenge struct {
	ID          string            `yaml:"id"`
	Title       string            `yaml:"title"`
	Description string            `yaml:"description"`
	Language    string            `yaml:"language,omitempty"`
	Type        string            `yaml:"type,omitempty"`
	TimeoutSec  int               `yaml:"timeout_sec,omitempty"`
	Limits      map[string]any    `yaml:"limits,omitempty"`
	Files       map[string]string `yaml:"files,omitempty"`
	Hints       []string          `yaml:"hints,omitempty"`
	Comparison  map[string]any    `yaml:"comparison,omitempty"`
}

// LessonRef is a discovered lesson on disk with its parsed frontmatter
// and resolved references.
type LessonRef struct {
	BlockSlug     string
	LessonSlug    string
	Path          string // path to lesson.md
	Frontmatter   LessonFrontmatter
	Questions     []Question              // from questions.yaml (if present)
	QuestionFiles map[string]QuestionFile // from questions/<slug>.yaml (new format)
	Challenges    map[string]Challenge    // by challenge slug
	// QuizRefs lists slugs referenced from `> [!quiz] slug` callouts in lesson.md.
	QuizRefs []string
	// ChallengeRefs lists slugs referenced from `> [!challenge] slug`.
	ChallengeRefs []string
}

// BlockRef is a discovered block on disk with its parsed manifest.
type BlockRef struct {
	Slug    string
	Path    string // path to block.yaml
	Block   Block
	Lessons []LessonRef // ordered by Block.Lessons
}

// CourseRef is the full course tree loaded from disk.
type CourseRef struct {
	Path     string // path to course.yaml
	Course   Course
	Blocks   []BlockRef // ordered by Course.Blocks
	LoadedAt time.Time
}
