package course

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestSetTopLevelIDIfMissing_FillsEmpty(t *testing.T) {
	in := []byte("id:\ntitle: Hello\n")
	out, changed, id := SetTopLevelIDIfMissing(in)
	if !changed {
		t.Fatal("expected changed=true")
	}
	if _, err := uuid.Parse(id); err != nil {
		t.Fatalf("expected valid UUID, got %q: %v", id, err)
	}
	want := "id: " + id + "\ntitle: Hello\n"
	if string(out) != want {
		t.Errorf("output mismatch:\nwant %q\ngot  %q", want, string(out))
	}
}

func TestSetTopLevelIDIfMissing_PreservesExisting(t *testing.T) {
	existing := "abc-123-existing"
	in := []byte("id: " + existing + "\ntitle: Hello\n")
	out, changed, _ := SetTopLevelIDIfMissing(in)
	if changed {
		t.Fatal("expected changed=false when id is present")
	}
	if string(out) != string(in) {
		t.Errorf("output should be unchanged")
	}
}

func TestSetTopLevelIDIfMissing_PreservesQuotedEmptyAsMissing(t *testing.T) {
	in := []byte(`id: ""
title: Hello
`)
	_, changed, id := SetTopLevelIDIfMissing(in)
	if !changed {
		t.Fatal("expected empty quoted string to be treated as missing")
	}
	if _, err := uuid.Parse(id); err != nil {
		t.Fatalf("expected valid UUID, got %q", id)
	}
}

func TestSetTopLevelIDIfMissing_PreservesComments(t *testing.T) {
	in := []byte(`# Top comment
id:                 # auto-filled by the bot, never change
title: "Hello"
# Another comment
slug: my-course
`)
	out, changed, id := SetTopLevelIDIfMissing(in)
	if !changed {
		t.Fatal("expected changed=true")
	}
	s := string(out)

	// Top comment preserved.
	if !strings.HasPrefix(s, "# Top comment\n") {
		t.Errorf("top comment missing:\n%s", s)
	}
	// Trailing comment preserved on id line.
	if !strings.Contains(s, id+"  # auto-filled by the bot") {
		t.Errorf("trailing comment on id line not preserved:\n%s", s)
	}
	// Other lines untouched.
	if !strings.Contains(s, "\n# Another comment\nslug: my-course\n") {
		t.Errorf("other content not preserved:\n%s", s)
	}
}

func TestSetTopLevelIDIfMissing_PrependsWhenAbsent(t *testing.T) {
	in := []byte("title: Hello\nslug: my-course\n")
	out, changed, id := SetTopLevelIDIfMissing(in)
	if !changed {
		t.Fatal("expected changed=true when id field is absent")
	}
	want := "id: " + id + "\ntitle: Hello\nslug: my-course\n"
	if string(out) != want {
		t.Errorf("expected id prepended:\nwant %q\ngot  %q", want, string(out))
	}
}

func TestSetTopLevelIDIfMissing_DoesNotMatchIndentedKey(t *testing.T) {
	// `id` is nested under another key — must not be confused with top-level.
	in := []byte(`title: Hello
test:
  id: should-not-touch
`)
	out, changed, _ := SetTopLevelIDIfMissing(in)
	if !changed {
		t.Fatal("expected to prepend a top-level id (the existing one is nested)")
	}
	s := string(out)
	if !strings.Contains(s, "  id: should-not-touch\n") {
		t.Errorf("nested id was modified:\n%s", s)
	}
	// First line should be the new top-level id.
	if !strings.HasPrefix(s, "id: ") {
		t.Errorf("expected top-level id at start:\n%s", s)
	}
}

func TestSetTopLevelIDIfMissing_NoTrailingNewlinePreserved(t *testing.T) {
	in := []byte("id:\ntitle: Hello") // no trailing \n
	out, changed, _ := SetTopLevelIDIfMissing(in)
	if !changed {
		t.Fatal("expected changed=true")
	}
	if strings.HasSuffix(string(out), "\n") {
		t.Errorf("trailing newline should be preserved as absent, got %q", string(out))
	}
}

func TestSetQuestionsIDsIfMissing(t *testing.T) {
	in := []byte(`questions:
  - id:
    slug: q1
    type: multiple_choice
    text: "Q1"
  - id: 11111111-2222-3333-4444-555555555555
    slug: q2
    type: true_false
  - id:
    slug: q3
    type: free_text
`)
	out, filled := SetQuestionsIDsIfMissing(in)
	if filled != 2 {
		t.Errorf("expected 2 ids filled, got %d", filled)
	}

	s := string(out)

	// Existing id preserved.
	if !strings.Contains(s, "id: 11111111-2222-3333-4444-555555555555") {
		t.Errorf("existing id was lost:\n%s", s)
	}

	// Both new ids should be valid UUIDs.
	count := 0
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- id:") {
			continue
		}
		v := strings.TrimSpace(strings.TrimPrefix(trimmed, "- id:"))
		if v == "" {
			t.Fatalf("blank id remained: %s", line)
		}
		if _, err := uuid.Parse(v); err != nil {
			t.Fatalf("invalid UUID: %s", v)
		}
		count++
	}
	if count != 3 {
		t.Errorf("expected 3 question id lines, got %d", count)
	}
}

func TestSetQuestionsIDsIfMissing_NoChangesWhenAllPresent(t *testing.T) {
	in := []byte(`questions:
  - id: 11111111-2222-3333-4444-555555555555
    slug: q1
  - id: 66666666-7777-8888-9999-000000000000
    slug: q2
`)
	out, filled := SetQuestionsIDsIfMissing(in)
	if filled != 0 {
		t.Errorf("expected no changes, got %d", filled)
	}
	if string(out) != string(in) {
		t.Errorf("output should be byte-identical when no changes needed")
	}
}

func TestIsTopLevelIDLine(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"id:", true},
		{"id: foo", true},
		{"id: # comment", true},
		{" id:", false},  // indented
		{"  id:", false}, // indented
		{"identity:", false},
		{"\tid:", false},
		{"# id:", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isTopLevelIDLine(c.line); got != c.want {
			t.Errorf("isTopLevelIDLine(%q) = %v, want %v", c.line, got, c.want)
		}
	}
}

func TestHasYAMLValue(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"id:", false},
		{"id: ", false},
		{"id:    ", false},
		{"id: # comment only", false},
		{`id: ""`, false},
		{`id: ''`, false},
		{"id: foo", true},
		{"id: foo  # trailing", true},
		{"id: 11111111-2222-3333-4444-555555555555", true},
	}
	for _, c := range cases {
		if got := hasYAMLValue(c.line); got != c.want {
			t.Errorf("hasYAMLValue(%q) = %v, want %v", c.line, got, c.want)
		}
	}
}

// (helper removed — was used to work around a shadowed t variable in tests)
