// Package teamfile owns the Go side of the standing-team file
// `.lets/teams/<name>.md`: rendering it from the plugin template
// (plugins/lets/templates/team.md) and finding the team that owns a worktree.
// The file is model-read; Go reads only the frontmatter keys `team`, `worktree`
// and `git_dir`, and never anything below the frontmatter.
//
// `.lets/teams/` is shared by every worktree of the repo, so one broken file must
// not affect another worktree: FindByWorktree skips a malformed file that does not
// claim the caller's worktree and reports it as a warning.
//
// Leaf package: standard library only.
package teamfile

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var (
	// ErrAmbiguousTeam: two team files claim the same worktree.
	ErrAmbiguousTeam = errors.New("ambiguous team")
	// ErrStaleTeam: the team file's git_dir is not this worktree's git dir - the
	// path was reused by another worktree after the team's own was removed.
	ErrStaleTeam = errors.New("stale team")
	// ErrMalformedTeam: a team file that claims this worktree is malformed (a
	// name that is invalid or differs from the filename, a bad value).
	ErrMalformedTeam = errors.New("malformed team file")
	// ErrPlaceholder: a `{{` is left in the rendered output (an unknown key, or a
	// value that itself carries `{{`).
	ErrPlaceholder = errors.New("unresolved placeholder")
	// ErrControlChar: a value carries a control character.
	ErrControlChar = errors.New("control character in value")
)

// Values maps a template placeholder key to its value.
type Values map[string]string

var (
	placeholderRe = regexp.MustCompile(`\{\{([A-Za-z0-9_]+)\}\}`)
	nameRe        = regexp.MustCompile(`^[a-z0-9-]{1,40}$`)
	keyLineRe     = regexp.MustCompile(`^([A-Za-z0-9_]+):[ \t]*(.*?)[ \t]*$`)
)

// maxFrontmatterLines bounds how far FindByWorktree reads a file looking for the
// closing `---`; the team file's frontmatter is a dozen lines.
const maxFrontmatterLines = 64

// ValidName reports whether s is a usable team name (the callsign): lowercase
// letters, digits and hyphens, 1-40 characters. The `run-` prefix is reserved for
// execute scopes (`run-{RUN}`).
func ValidName(s string) bool {
	return nameRe.MatchString(s) && !strings.HasPrefix(s, "run-")
}

// Render replaces every `{{key}}` in tmpl with the YAML double-quoted value of
// v[key]. A value with a control character is refused; a `{{` left in the output
// (an unknown key, or a value carrying `{{`) fails the render. Nothing partial is
// returned on error.
func Render(tmpl []byte, v Values) ([]byte, error) {
	for k, val := range v {
		if strings.ContainsFunc(val, unicode.IsControl) {
			return nil, fmt.Errorf("%w: %s", ErrControlChar, k)
		}
	}
	out := placeholderRe.ReplaceAllFunc(tmpl, func(m []byte) []byte {
		val, ok := v[string(m[2:len(m)-2])]
		if !ok {
			return m
		}
		return []byte(strconv.Quote(val))
	})
	if i := strings.Index(string(out), "{{"); i >= 0 {
		line := 1 + strings.Count(string(out[:i]), "\n")
		return nil, fmt.Errorf("%w at line %d", ErrPlaceholder, line)
	}
	return out, nil
}

// header is the part of a team file's frontmatter Go reads.
type header struct {
	team, worktree, gitDir string
}

// FindByWorktree returns the team whose file in teamsDir claims the worktree at
// toplevel. gitDir is that worktree's git dir (`git rev-parse --absolute-git-dir`;
// a relative one is taken from toplevel). Both paths are compared after symlink
// resolution. Only the frontmatter is read.
//
//   - A file with no frontmatter is not a team file and is skipped silently.
//   - A malformed file that does not claim toplevel is skipped and named in
//     warnings - it cannot affect this worktree.
//   - Two files claiming toplevel -> ErrAmbiguousTeam; the claiming file is
//     malformed or its name differs from the filename -> ErrMalformedTeam; its
//     git_dir is set and differs from gitDir -> ErrStaleTeam. Every error names
//     the file(s).
//
// A missing teamsDir is no team (ok=false, nil error).
func FindByWorktree(teamsDir, toplevel, gitDir string) (name string, ok bool, warnings []string, err error) {
	entries, err := os.ReadDir(teamsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil, nil
		}
		return "", false, nil, err
	}
	top := resolve(toplevel, "")
	type claim struct {
		path, stem string
		h          header
		bad        error
	}
	var claims []claim
	for _, e := range entries {
		fname := e.Name()
		stem, isMD := strings.CutSuffix(fname, ".md")
		if !isMD || strings.HasPrefix(fname, ".") || !e.Type().IsRegular() {
			continue
		}
		path := filepath.Join(teamsDir, fname)
		h, found, perr := readHeader(path)
		if !found && perr == nil {
			continue
		}
		if h.worktree == "" || resolve(h.worktree, "") != top {
			if perr != nil || !ValidName(stem) || h.team != stem || h.worktree == "" {
				warnings = append(warnings, fmt.Sprintf("skipped malformed team file %q", path))
			}
			continue
		}
		c := claim{path: path, stem: stem, h: h, bad: perr}
		if c.bad == nil && (!ValidName(stem) || h.team != stem) {
			c.bad = fmt.Errorf("team %q does not match the filename", h.team)
		}
		claims = append(claims, c)
	}
	switch len(claims) {
	case 0:
		return "", false, warnings, nil
	case 1:
	default:
		paths := make([]string, len(claims))
		for i, c := range claims {
			paths[i] = strconv.Quote(c.path)
		}
		return "", false, warnings, fmt.Errorf("%w: %s all claim %q", ErrAmbiguousTeam, strings.Join(paths, ", "), toplevel)
	}
	c := claims[0]
	if c.bad != nil {
		return "", false, warnings, fmt.Errorf("%w: %q: %v", ErrMalformedTeam, c.path, c.bad)
	}
	if c.h.gitDir != "" && resolve(c.h.gitDir, c.h.worktree) != resolve(gitDir, toplevel) {
		return "", false, warnings, fmt.Errorf("%w: %q records git_dir %q, the worktree's is %q", ErrStaleTeam, c.path, c.h.gitDir, gitDir)
	}
	return c.h.team, true, warnings, nil
}

// readHeader reads the frontmatter keys Go uses. found=false with a nil error
// means the file has no frontmatter (not a team file). A frontmatter that does not
// close, repeats a key Go reads, or carries a value that does not parse is an
// error; the keys read so far are still returned so the caller can tell whether
// the file claims its worktree.
func readHeader(path string) (h header, found bool, err error) {
	f, err := os.Open(path)
	if err != nil {
		return h, true, err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	if !sc.Scan() || strings.TrimSpace(sc.Text()) != "---" {
		return h, false, sc.Err()
	}
	seen := map[string]bool{}
	for n := 0; n < maxFrontmatterLines && sc.Scan(); n++ {
		line := sc.Text()
		if strings.TrimSpace(line) == "---" {
			return h, true, err
		}
		m := keyLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		var dst *string
		switch m[1] {
		case "team":
			dst = &h.team
		case "worktree":
			dst = &h.worktree
		case "git_dir":
			dst = &h.gitDir
		default:
			continue
		}
		if seen[m[1]] {
			err = errors.Join(err, fmt.Errorf("duplicate key %s", m[1]))
			continue
		}
		seen[m[1]] = true
		val, verr := scalar(m[2])
		if verr != nil {
			err = errors.Join(err, fmt.Errorf("key %s: %w", m[1], verr))
			continue
		}
		*dst = val
	}
	if serr := sc.Err(); serr != nil {
		return h, true, serr
	}
	return h, true, errors.Join(err, errors.New("frontmatter does not close"))
}

// scalar parses a one-line YAML scalar: double-quoted (as Render writes it),
// single-quoted, or plain (a hand-made file).
func scalar(raw string) (string, error) {
	var val string
	switch {
	case strings.HasPrefix(raw, `"`):
		v, err := strconv.Unquote(raw)
		if err != nil {
			return "", errors.New("bad double-quoted value")
		}
		val = v
	case strings.HasPrefix(raw, `'`):
		if len(raw) < 2 || !strings.HasSuffix(raw, `'`) {
			return "", errors.New("bad single-quoted value")
		}
		val = strings.ReplaceAll(raw[1:len(raw)-1], `''`, `'`)
	default:
		if i := strings.Index(raw, " #"); i >= 0 {
			raw = strings.TrimSpace(raw[:i])
		}
		val = raw
	}
	if strings.ContainsFunc(val, unicode.IsControl) {
		return "", ErrControlChar
	}
	return val, nil
}

// resolve makes p absolute (a relative p is taken from base, else from the
// process cwd), then resolves symlinks. A path that does not exist stays cleaned.
func resolve(p, base string) string {
	if p == "" {
		return ""
	}
	if !filepath.IsAbs(p) && base != "" {
		p = filepath.Join(base, p)
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	return filepath.Clean(p)
}
