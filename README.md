# StudyMe Action

GitHub Action that maintains StudyMe course repositories.

Two modes:

- **`fix-ids`** — fills missing `id:` UUIDs in `course.yaml`, `block.yaml`,
  `challenge.yaml`, `questions.yaml` entries, and `lesson.md` frontmatter.
  Existing IDs are preserved.
- **`lint`** — validates the course structure: schema correctness,
  unique IDs within the course, slug references resolve, callouts point
  to existing questions/challenges.

## Usage

```yaml
- uses: wockitech/studyme-action@v1
  with:
    mode: fix-ids       # or: lint
    path: .             # course root (default: repo root)
```

## Outputs

| Output | Description |
|--------|-------------|
| `changed_files` | Newline-separated list of files modified by `fix-ids` |
| `errors` | JSON array of lint errors (only in `lint` mode) |

## Local usage

The same binary works as a CLI. Useful for pre-commit hooks:

```bash
go install github.com/wockitech/studyme-action/cmd/studyme-action@latest

studyme-action fix-ids ./my-course
studyme-action lint    ./my-course
```

## Development

```bash
go test ./...
go run ./cmd/studyme-action lint ../course-template
```
