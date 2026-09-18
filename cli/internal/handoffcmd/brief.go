//go:build unix

package handoffcmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// safeRoot is the only alphabet a checkout path may have to reach the pointer line.
// The line is typed into another program's input: an agent reads it as text, but if
// that program were ever a shell, every word must stay an inert word - no `;`, `&`,
// `|`, redirect, quote, `$`, glob, `~` or control byte can appear.
var safeRoot = regexp.MustCompile(`^/[A-Za-z0-9._/ -]+$`)

var briefName = regexp.MustCompile(`^[A-Za-z0-9._-]+\.md$`)

// CheckBrief accepts only a brief artifact-path wrote: an absolute, clean path to a
// regular file directly in <root>/.lets/handoffs/ with a plain name, under a
// checkout path from the safe alphabet.
func CheckBrief(root, brief string) error {
	dir := filepath.Join(root, ".lets", "handoffs")
	switch {
	case !safeRoot.MatchString(root) || filepath.Clean(root) != root:
		return fmt.Errorf("checkout path %q has characters a one-line pointer cannot carry (allowed: letters, digits, . _ / - and space)", root)
	case !filepath.IsAbs(brief) || filepath.Clean(brief) != brief:
		return errors.New("--brief must be an absolute, clean path")
	case filepath.Dir(brief) != dir:
		return fmt.Errorf("--brief must be a file in %s", dir)
	case !briefName.MatchString(filepath.Base(brief)):
		return errors.New("--brief file name must match [A-Za-z0-9._-]+.md")
	}
	fi, err := os.Stat(brief)
	if err != nil || !fi.Mode().IsRegular() {
		return errors.New("--brief is not a regular file")
	}
	return nil
}

// OutBase is the brief path without ".md": every output is a sibling of the brief.
func OutBase(brief string) string { return strings.TrimSuffix(brief, ".md") }

// Pointer is the one line typed into the target terminal.
func Pointer(brief string) string {
	return "Read the hand-off brief at " + brief + " and follow it exactly."
}
