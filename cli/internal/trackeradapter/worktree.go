// Package trackeradapter reads the `## Worktree` declaration of a tracker adapter
// (plugins/lets/rules/tracker-<name>.md): the store files a worktree must share
// with the main checkout (`links:`), and the task id + branch naming convention
// (`id:` / `branch:` / `worktree-branch:` / `accept:`).
//
// Leaf package: standard library only. initcmd imports it (ValidTrackerName and
// the adapter contract tests), so it must never import initcmd or any other
// non-leaf package.
package trackeradapter

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// NameRe is the ONE tracker adapter name shape; initcmd.ValidTrackerName delegates
// here. The value flows into filepath.Join (install target, cleanup glob, Load) and
// reaches model context via the SessionStart hook, so a hand-edited LETS_TRACKER
// like "../../etc/x" must never be used unvalidated.
var NameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

var (
	// ErrUndeclared: the adapter has no `links:` line in its `## Worktree` section.
	ErrUndeclared = errors.New("store links undeclared")
	// ErrInvalid: a `links:` line exists but does not parse into safe links.
	ErrInvalid = errors.New("store links declaration invalid")
)

// Link is one repo-relative file shared from the main checkout into a worktree.
type Link struct {
	Path string
	Mode os.FileMode
}

// Worktree is the loaded `links:` declaration and where it came from.
type Worktree struct {
	Links  []Link
	Source string // installed | plugin
}

const (
	SourceInstalled = "installed"
	SourcePlugin    = "plugin"
	SourceBoard     = "board"
	SourceDefault   = "default"
)

// Load reasons (non-empty reason = something the caller should surface as a warning).
const (
	ReasonTrackerNameInvalid           = "tracker_name_invalid"
	ReasonAdapterMissing               = "adapter_missing"
	ReasonStoreLinksUndeclared         = "store_links_undeclared"
	ReasonStoreLinksDeclarationInvalid = "store_links_declaration_invalid"
	ReasonAdapterLagsPlugin            = "adapter_lags_plugin"
)

var linkRe = regexp.MustCompile("`([^`]+)`\\s*\\((0[0-7]{3})\\)")

// declRe matches one declaration line of the `## Worktree` section. Line-anchored,
// so `branch:` never matches inside `worktree-branch:`.
var declRe = regexp.MustCompile(`^(links|id|branch|worktree-branch|accept):\s+(.+)\.$`)

// section returns the body of the `heading` section: the text after the heading
// line up to the next `## ` heading, or "" when the heading is absent.
func section(content, heading string) string {
	lines := strings.Split(content, "\n")
	start := -1
	for i, l := range lines {
		if strings.TrimRight(l, " \t\r") == heading {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return ""
	}
	end := len(lines)
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}

// declarations parses every declaration line of a section into key -> value
// (the text after "key: " without the closing period). A repeated key is
// reported as dup.
func declarations(sec string) (decls map[string]string, dup string) {
	decls = map[string]string{}
	for _, l := range strings.Split(sec, "\n") {
		m := declRe.FindStringSubmatch(strings.TrimRight(l, " \t\r"))
		if m == nil {
			continue
		}
		if _, seen := decls[m[1]]; seen && dup == "" {
			dup = m[1]
		}
		decls[m[1]] = strings.TrimSpace(m[2])
	}
	return decls, dup
}

// ParseWorktree reads ONLY the `links:` line of the `## Worktree` section. Pure.
func ParseWorktree(content string) ([]Link, error) {
	decls, dup := declarations(section(content, "## Worktree"))
	rest, ok := decls["links"]
	if !ok {
		return nil, ErrUndeclared
	}
	if dup == "links" {
		return nil, fmt.Errorf("%w: links declared twice", ErrInvalid)
	}
	if rest == "nothing" {
		return []Link{}, nil
	}
	var out []Link
	for _, m := range linkRe.FindAllStringSubmatch(rest, -1) {
		p := filepath.ToSlash(filepath.Clean(m[1]))
		mode, _ := strconv.ParseUint(m[2], 8, 32)
		switch {
		case filepath.IsAbs(m[1]), p == ".", p == "..", strings.HasPrefix(p, "../"), strings.Contains(p, "/../"),
			strings.HasPrefix(p, ".git"), strings.HasPrefix(p, ".lets"),
			mode != 0o600 && mode != 0o640 && mode != 0o644:
			return nil, fmt.Errorf("%w: %q (%s)", ErrInvalid, m[1], m[2])
		}
		out = append(out, Link{Path: p, Mode: os.FileMode(mode)})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: %q", ErrInvalid, rest)
	}
	return out, nil
}

// adapterFile is the installed adapter path for tracker under a main checkout.
func adapterFile(mainRoot, tracker string) string {
	return filepath.Join(mainRoot, ".claude", "rules", "tracker-"+tracker+".md")
}

// boardFile is the user-owned board profile path for tracker under a main checkout.
func boardFile(mainRoot, tracker string) string {
	return filepath.Join(mainRoot, ".claude", "rules", "tracker-"+tracker+".board.md")
}

// pluginAdapterFile is the plugin's shipped adapter path, or "" without a plugin root.
func pluginAdapterFile(pluginRoot, tracker string) string {
	if pluginRoot == "" {
		return ""
	}
	return filepath.Join(pluginRoot, "rules", "tracker-"+tracker+".md")
}

// Load reads the store links for tracker. mainRoot MUST be the main checkout.
// pluginRoot is already validated by the caller ("" disables the plugin fallback).
// The installed adapter wins; the plugin copy is used when the installed one is
// missing or predates the declaration. A non-empty reason is a warning to surface.
func Load(mainRoot, tracker, pluginRoot string) (Worktree, string) {
	if !NameRe.MatchString(tracker) {
		return Worktree{}, ReasonTrackerNameInvalid
	}
	fromPlugin := func(reasonIfUsed, reasonIfNot string) (Worktree, string) {
		if pf := pluginAdapterFile(pluginRoot, tracker); pf != "" {
			if data, err := os.ReadFile(pf); err == nil {
				if links, err := ParseWorktree(string(data)); err == nil {
					return Worktree{Links: links, Source: SourcePlugin}, reasonIfUsed
				}
			}
		}
		return Worktree{}, reasonIfNot
	}
	data, err := os.ReadFile(adapterFile(mainRoot, tracker))
	if err != nil {
		return fromPlugin(ReasonAdapterMissing, ReasonAdapterMissing)
	}
	links, err := ParseWorktree(string(data))
	switch {
	case err == nil:
		return Worktree{Links: links, Source: SourceInstalled}, ""
	case errors.Is(err, ErrUndeclared):
		return fromPlugin(ReasonAdapterLagsPlugin, ReasonStoreLinksUndeclared)
	default:
		return Worktree{}, ReasonStoreLinksDeclarationInvalid
	}
}
