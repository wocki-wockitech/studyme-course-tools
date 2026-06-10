package coursefmt

import (
	"testing"
	"testing/fstest"
)

func TestLoadFromFS_MinimalCourse(t *testing.T) {
	fs := fstest.MapFS{
		"course.yaml": &fstest.MapFile{
			Data: []byte(`id: 11111111-1111-1111-1111-111111111111
slug: my-course
title: "My Course"
default_language: ru
languages: [ru]
blocks:
  - block-1
`),
		},
		"block-1/block.yaml": &fstest.MapFile{
			Data: []byte(`id: 22222222-2222-2222-2222-222222222222
title: "Block 1"
lessons:
  - lesson-1
test:
  question_count: 5
  pass_threshold: 0.7
`),
		},
		"block-1/lesson-1/lesson.md": &fstest.MapFile{
			Data: []byte(`---
id: 33333333-3333-3333-3333-333333333333
title: "Lesson 1"
---

# Hello

Some content.

> [!card] q1

> [!challenge] c1
`),
		},
		"block-1/lesson-1/cards/q1.yaml": &fstest.MapFile{
			Data: []byte(`id: 44444444-4444-4444-4444-444444444444
difficulty: 2
questions:
  - type: true_false
    text: "Is this a test?"
    correct: true
`),
		},
		"block-1/lesson-1/challenges/c1/challenge.yaml": &fstest.MapFile{
			Data: []byte(`id: 55555555-5555-5555-5555-555555555555
title: "Challenge 1"
language: go
files:
  template: solution.go
  tests: solution_test.go
  solution: solution/solution.go
`),
		},
	}

	cr, err := LoadFromFS(fs, "memfs")
	if err != nil {
		t.Fatalf("LoadFromFS: %v", err)
	}

	if cr.Course.Slug != "my-course" {
		t.Errorf("course slug: got %q", cr.Course.Slug)
	}
	if got := cr.Course.EffectiveDefaultLanguage(); got != "ru" {
		t.Errorf("default language: got %q", got)
	}
	if got := cr.Course.EffectiveLanguages(); len(got) != 1 || got[0] != "ru" {
		t.Errorf("languages: got %v", got)
	}

	if len(cr.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(cr.Blocks))
	}
	b := cr.Blocks[0]
	if b.Block.Title.Resolve("ru", "ru") != "Block 1" {
		t.Errorf("block title: got %q", b.Block.Title.Resolve("ru", "ru"))
	}

	if len(b.Lessons) != 1 {
		t.Fatalf("expected 1 lesson, got %d", len(b.Lessons))
	}
	l := b.Lessons[0]
	if l.Frontmatter.Title != "Lesson 1" {
		t.Errorf("lesson title: got %q", l.Frontmatter.Title)
	}

	// Verify card file was loaded
	if len(l.CardFiles) != 1 {
		t.Fatalf("expected 1 card file, got %d", len(l.CardFiles))
	}
	cf, ok := l.CardFiles["q1"]
	if !ok {
		t.Fatal("expected card file 'q1'")
	}
	if cf.ID != "44444444-4444-4444-4444-444444444444" {
		t.Errorf("card file ID: got %q", cf.ID)
	}
	if len(cf.Questions) != 1 {
		t.Fatalf("expected 1 question in card file, got %d", len(cf.Questions))
	}
	if cf.Questions[0].Type != "true_false" {
		t.Errorf("question type: got %q", cf.Questions[0].Type)
	}

	if len(l.CardRefs) != 1 || l.CardRefs[0] != "q1" {
		t.Errorf("card refs: got %v", l.CardRefs)
	}
	if len(l.ChallengeRefs) != 1 || l.ChallengeRefs[0] != "c1" {
		t.Errorf("challenge refs: got %v", l.ChallengeRefs)
	}
	if _, ok := l.Challenges["c1"]; !ok {
		t.Errorf("expected challenge c1, got %+v", l.Challenges)
	}
}

func TestLoadFromFS_CardSubfolder(t *testing.T) {
	fs := fstest.MapFS{
		"course.yaml": &fstest.MapFile{
			Data: []byte(`id: x
slug: c
blocks: [b]`),
		},
		"b/block.yaml": &fstest.MapFile{
			Data: []byte(`id: x
title: B
lessons: [l]
test:
  question_count: 1
  pass_threshold: 0.5`),
		},
		"b/l/lesson.md": &fstest.MapFile{
			Data: []byte("---\nid: x\ntitle: L\n---\n\n> [!card] code-q\n"),
		},
		"b/l/cards/code-q/card.yaml": &fstest.MapFile{
			Data: []byte(`id: abc
difficulty: 3
title: "Code Fill Card"
questions:
  - type: code_fill
    text: "Fill in the blank"
    file: main.go
    slots:
      - answer: ["fmt.Println"]
`),
		},
		"b/l/cards/code-q/main.go": &fstest.MapFile{
			Data: []byte("package main\n\nfunc main() {\n\t___\n}\n"),
		},
	}

	cr, err := LoadFromFS(fs, "memfs")
	if err != nil {
		t.Fatalf("LoadFromFS: %v", err)
	}

	l := cr.Blocks[0].Lessons[0]
	cf, ok := l.CardFiles["code-q"]
	if !ok {
		t.Fatal("expected card file 'code-q'")
	}
	if cf.Title != "Code Fill Card" {
		t.Errorf("card title: got %q", cf.Title)
	}
	if len(cf.Questions) != 1 {
		t.Fatalf("expected 1 question, got %d", len(cf.Questions))
	}
	q := cf.Questions[0]
	if q.File != "main.go" {
		t.Errorf("question file: got %q", q.File)
	}
	if q.FileContent == "" {
		t.Error("expected FileContent to be loaded")
	}
	if q.FileContent != "package main\n\nfunc main() {\n\t___\n}\n" {
		t.Errorf("unexpected file content: %q", q.FileContent)
	}
}

func TestLoadFromFS_MissingCourseYaml(t *testing.T) {
	fs := fstest.MapFS{}
	_, err := LoadFromFS(fs, "memfs")
	if err == nil {
		t.Fatal("expected error for missing course.yaml")
	}
}

func TestLoadFromFS_LessonWithoutFrontmatter(t *testing.T) {
	fs := fstest.MapFS{
		"course.yaml": &fstest.MapFile{Data: []byte(`id: x
slug: c
blocks: [b]`)},
		"b/block.yaml": &fstest.MapFile{Data: []byte(`id: x
title: B
lessons: [l]`)},
		"b/l/lesson.md": &fstest.MapFile{
			Data: []byte("# No frontmatter\n\nbody"),
		},
	}
	_, err := LoadFromFS(fs, "memfs")
	if err == nil {
		t.Fatal("expected error for lesson without frontmatter")
	}
}

func TestEffectiveDefaultLanguage_Backwards(t *testing.T) {
	cases := []struct {
		c    Course
		want string
	}{
		{Course{DefaultLanguage: "en"}, "en"},
		{Course{Language: "en"}, "en"},                        // legacy
		{Course{DefaultLanguage: "en", Language: "ru"}, "en"}, // new wins
		{Course{}, "ru"},                                      // platform default
	}
	for i, c := range cases {
		if got := c.c.EffectiveDefaultLanguage(); got != c.want {
			t.Errorf("case %d: got %q, want %q", i, got, c.want)
		}
	}
}
