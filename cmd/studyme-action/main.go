// studyme-action is the CLI entry point used by both the GitHub Action
// and local pre-commit hooks.
//
// Usage:
//
//	studyme-action fix-ids [path]   Fill missing UUIDs in course content
//	studyme-action lint    [path]   Validate course structure
//
// When invoked from GitHub Actions, also writes outputs to $GITHUB_OUTPUT.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wocki-wockitech/studyme-course-tools/internal/cmd"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	mode := os.Args[1]
	root := "."
	if len(os.Args) >= 3 {
		root = os.Args[2]
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		fail("resolve path: %v", err)
	}

	switch mode {
	case "fix-ids":
		runFixIDs(abs)
	case "lint":
		runLint(abs)
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown mode: %q\n\n", mode)
		usage()
		os.Exit(2)
	}
}

func runFixIDs(root string) {
	changed, err := cmd.FixIDs(root)
	if err != nil {
		fail("fix-ids: %v", err)
	}

	if len(changed) == 0 {
		fmt.Println("All IDs present, nothing to fix.")
	} else {
		fmt.Printf("Filled IDs in %d file(s):\n", len(changed))
		for _, p := range changed {
			rel, _ := filepath.Rel(root, p)
			fmt.Println("  " + rel)
		}
	}
	writeGitHubOutput("changed_files", strings.Join(changed, "\n"))
}

func runLint(root string) {
	errors, err := cmd.Lint(root)
	if err != nil {
		fail("lint: %v", err)
	}

	if len(errors) == 0 {
		fmt.Println("✓ Course is valid.")
		writeGitHubOutput("errors", "[]")
		return
	}

	fmt.Fprintf(os.Stderr, "%d issue(s) found:\n\n", len(errors))
	for _, e := range errors {
		fmt.Fprintln(os.Stderr, "  "+e.String())
	}

	out, _ := json.Marshal(errors)
	writeGitHubOutput("errors", string(out))

	os.Exit(1)
}

// writeGitHubOutput appends `name=value` to $GITHUB_OUTPUT if running
// inside a GitHub Action. No-op otherwise.
//
// Multi-line values are written using the heredoc syntax that GitHub
// requires.
func writeGitHubOutput(name, value string) {
	path := os.Getenv("GITHUB_OUTPUT")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	if strings.ContainsRune(value, '\n') {
		// Generate a delimiter unlikely to appear in the value.
		delim := "STUDYME_EOF_" + randSuffix()
		fmt.Fprintf(f, "%s<<%s\n%s\n%s\n", name, delim, value, delim)
		return
	}
	fmt.Fprintf(f, "%s=%s\n", name, value)
}

func randSuffix() string {
	// Simple time-based suffix; sufficient because the value content is
	// course author markdown, not adversarial input.
	return fmt.Sprintf("%d", os.Getpid())
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "studyme-action: "+format+"\n", args...)
	os.Exit(1)
}

func usage() {
	fmt.Fprintln(os.Stderr, `studyme-action — maintains StudyMe course repositories

Usage:
  studyme-action fix-ids [path]   Fill missing UUID 'id:' fields
  studyme-action lint    [path]   Validate course structure

If [path] is omitted, the current directory is used.`)
}
