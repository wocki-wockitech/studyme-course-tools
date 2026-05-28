// Package course defines the on-disk structure of a StudyMe course
// and the types used throughout the action.
package course

import "time"

// Course is the top-level manifest from course.yaml.
type Course struct {
	ID          string   `yaml:"id"`
	Slug        string   `yaml:"slug"`
	Title       string   `yaml:"title"`
	Description string   `yaml:"description"`
	Language    string   `yaml:"language"`
	Difficulty  string   `yaml:"difficulty"`
	Tags        []string `yaml:"tags"`
	Cover       string   `yaml:"cover"`
	License     string   `yaml:"license"`
	Authors     []Author `yaml:"authors"`
	Blocks      []string `yaml:"blocks"`
}

type Author struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

// Block is the manifest from block.yaml.
type Block struct {
	ID          string    `yaml:"id"`
	Title       string    `yaml:"title"`
	Description string    `yaml:"description"`
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
	Options []string `yaml:"options,omitempty"`
	Correct any      `yaml:"correct,omitempty"`
	// type=free_text
	EvaluationCriteria map[string]any `yaml:"evaluation_criteria,omitempty"`
	MaxScore           int            `yaml:"max_score,omitempty"`
	PassThreshold      float64        `yaml:"pass_threshold,omitempty"`
	// type=coding | git_interactive
	ChallengeSlug string `yaml:"challenge_slug,omitempty"`
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
	BlockSlug   string
	LessonSlug  string
	Path        string // path to lesson.md
	Frontmatter LessonFrontmatter
	Questions   []Question           // from questions.yaml (if present)
	Challenges  map[string]Challenge // by challenge slug
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
