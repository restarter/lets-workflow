// Package frontmatter parses YAML frontmatter from markdown files.
//
// Used by both the `lets init` and `lets hook session-start` subcommands to read
// the `version:` key from rules-file frontmatter (drift detection, install).
// Strict semver-shaped regex prevents prompt-injection via attacker-controlled
// rules content: the version string is interpolated into hook stdout / LETS Notice.
package frontmatter

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

var versionRe = regexp.MustCompile(`^version:\s+(\d+\.\d+\.\d+(?:-[\w.]+)?)\s*$`)

// ReadVersion returns the `version` field from the YAML frontmatter of the
// given markdown file. Returns empty string on any failure (missing file, no
// frontmatter, no version key, malformed value).
func ReadVersion(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return ""
	}
	if strings.TrimSpace(scanner.Text()) != "---" {
		return ""
	}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			return ""
		}
		if m := versionRe.FindStringSubmatch(line); m != nil {
			return m[1]
		}
	}
	return ""
}
