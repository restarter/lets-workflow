package trackeradapter

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// ErrConventionInvalid: an id / branch / worktree-branch / accept declaration does
// not validate. The error text names the key.
var ErrConventionInvalid = errors.New("convention declaration invalid")

// Convention reasons (LoadConvention).
const (
	ReasonConventionUndeclared         = "convention_undeclared"
	ReasonConventionDeclarationInvalid = "convention_declaration_invalid"
	ReasonBoardLinksIgnored            = "board_links_ignored"
	// ReasonKeysIgnoredNoID: branch: / worktree-branch: / accept: were declared
	// (typically in a user-owned board file) but no id: is declared anywhere, so
	// build() drops the whole convention. Without this reason the templates are
	// discarded in silence and the caller renders the built-in default instead.
	ReasonKeysIgnoredNoID = "convention_keys_ignored_no_id"
)

// Default templates for a declared convention that omits branch: / worktree-branch:.
const (
	DefaultBranch         = "feature/{id}-{slug}"
	DefaultWorktreeBranch = "worktree-{id}-{slug}"
)

// Convention is the task id + branch naming a tracker adapter teaches LETS. It is
// the ONE parser and renderer of branch names; markdown consumers get its results
// through `lets worktree info --task-candidate` and `lets worktree branch-name`,
// never by matching patterns themselves. It returns raw ids: every caller gates
// them with taskid.Valid.
type Convention struct {
	Declared       bool              // false: no id: line anywhere (convention_undeclared) - consumers keep legacy behavior
	ID             *regexp.Regexp    // nil with Declared: `id: nothing` - never derive an id from a name
	Branch         string            // created shape (take-task, lets orca open)
	WorktreeBranch string            // created shape (/lets:worktree create <id>)
	Accept         []string          // recognized by adopt only, never created
	Source         map[string]string // key -> installed | board | plugin | default
}

// ShapeSet selects which templates ParseBranch tries.
type ShapeSet int

const (
	Created ShapeSet = iota
	CreatedAndAccepted
)

const (
	maxIDLen       = 200
	maxTemplateLen = 100
	slugPattern    = `[a-z0-9][a-z0-9-]*`
)

var (
	literalRe = regexp.MustCompile(`^[A-Za-z0-9/._-]*$`)
	slugRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,49}$`)
	// backticked extracts the backticked tokens of a declaration value.
	backticked = regexp.MustCompile("`([^`]*)`")
)

// rawConvention is one file's declarations before overlay and defaults.
type rawConvention struct {
	keys map[string]string // id | branch | worktree-branch | accept -> raw value
}

// parseRaw validates the convention keys of one `## Worktree` section.
func parseRaw(content string) (rawConvention, error) {
	decls, dup := declarations(section(content, "## Worktree"))
	if dup != "" && dup != "links" {
		return rawConvention{}, fmt.Errorf("%w: %s declared twice", ErrConventionInvalid, dup)
	}
	raw := rawConvention{keys: map[string]string{}}
	for _, k := range []string{"id", "branch", "worktree-branch", "accept"} {
		if v, ok := decls[k]; ok {
			raw.keys[k] = v
		}
	}
	if v, ok := raw.keys["id"]; ok && v != "nothing" {
		if _, err := compileID(v); err != nil {
			return rawConvention{}, err
		}
	}
	for _, k := range []string{"branch", "worktree-branch"} {
		if v, ok := raw.keys[k]; ok {
			if _, err := singleTemplate(k, v); err != nil {
				return rawConvention{}, err
			}
		}
	}
	if v, ok := raw.keys["accept"]; ok {
		if _, err := templateList(v); err != nil {
			return rawConvention{}, err
		}
	}
	return raw, nil
}

// compileID validates and compiles a backticked id declaration.
func compileID(v string) (*regexp.Regexp, error) {
	toks := backticked.FindAllStringSubmatch(v, -1)
	if len(toks) != 1 || strings.TrimSpace(backticked.ReplaceAllString(v, "")) != "" {
		return nil, fmt.Errorf("%w: id must be one backticked RE2 fragment or nothing", ErrConventionInvalid)
	}
	src := toks[0][1]
	switch {
	case src == "", len(src) > maxIDLen:
		return nil, fmt.Errorf("%w: id is empty or longer than %d bytes", ErrConventionInvalid, maxIDLen)
	case strings.ContainsAny(src, "^$"):
		return nil, fmt.Errorf("%w: id must not carry anchors", ErrConventionInvalid)
	}
	re, err := regexp.Compile(src)
	if err != nil {
		return nil, fmt.Errorf("%w: id does not compile: %v", ErrConventionInvalid, err)
	}
	for _, name := range re.SubexpNames() {
		if name != "" {
			return nil, fmt.Errorf("%w: id must not use named groups", ErrConventionInvalid)
		}
	}
	return re, nil
}

// singleTemplate validates a one-template value (branch: / worktree-branch:).
func singleTemplate(key, v string) (string, error) {
	list, err := templateList(v)
	if err != nil {
		return "", err
	}
	if len(list) != 1 {
		return "", fmt.Errorf("%w: %s takes exactly one template", ErrConventionInvalid, key)
	}
	return list[0], nil
}

// templateList validates a comma-separated list of backticked templates.
func templateList(v string) ([]string, error) {
	toks := backticked.FindAllStringSubmatch(v, -1)
	rest := strings.TrimSpace(strings.ReplaceAll(backticked.ReplaceAllString(v, ""), ",", ""))
	if len(toks) == 0 || rest != "" {
		return nil, fmt.Errorf("%w: templates must be backticked and comma-separated", ErrConventionInvalid)
	}
	var out []string
	for _, t := range toks {
		if err := validateTemplate(t[1]); err != nil {
			return nil, err
		}
		out = append(out, t[1])
	}
	return out, nil
}

// validateTemplate: at most 100 bytes, exactly one {id}, at most one {slug}, literal
// parts in [A-Za-z0-9/._-]. Every rendered branch is therefore shell-safe.
func validateTemplate(tmpl string) error {
	switch {
	case len(tmpl) > maxTemplateLen:
		return fmt.Errorf("%w: template %q longer than %d bytes", ErrConventionInvalid, tmpl, maxTemplateLen)
	case strings.Count(tmpl, "{id}") != 1:
		return fmt.Errorf("%w: template %q must hold exactly one {id}", ErrConventionInvalid, tmpl)
	case strings.Count(tmpl, "{slug}") > 1:
		return fmt.Errorf("%w: template %q holds more than one {slug}", ErrConventionInvalid, tmpl)
	}
	literal := strings.ReplaceAll(strings.ReplaceAll(tmpl, "{id}", ""), "{slug}", "")
	if !literalRe.MatchString(literal) {
		return fmt.Errorf("%w: template %q has characters outside [A-Za-z0-9/._-]", ErrConventionInvalid, tmpl)
	}
	return nil
}

// ParseConvention reads the id/branch keys of the `## Worktree` section. Pure. An
// adapter without an id: line is returned with Declared=false.
func ParseConvention(content string) (Convention, error) {
	raw, err := parseRaw(content)
	if err != nil {
		return Convention{}, err
	}
	return build(raw, rawConvention{}, SourceInstalled)
}

// build overlays board keys onto base keys, applies defaults, and compiles the result.
func build(base, board rawConvention, baseSource string) (Convention, error) {
	c := Convention{Source: map[string]string{}}
	get := func(k string) (string, bool) {
		if v, ok := board.keys[k]; ok {
			c.Source[k] = SourceBoard
			return v, true
		}
		if v, ok := base.keys[k]; ok {
			c.Source[k] = baseSource
			return v, true
		}
		return "", false
	}
	idVal, hasID := get("id")
	if !hasID {
		return Convention{Declared: false, Source: map[string]string{}}, nil
	}
	c.Declared = true
	if idVal != "nothing" {
		re, err := compileID(idVal)
		if err != nil {
			return Convention{}, err
		}
		c.ID = re
	}
	var err error
	if v, ok := get("branch"); ok {
		if c.Branch, err = singleTemplate("branch", v); err != nil {
			return Convention{}, err
		}
	} else {
		c.Branch, c.Source["branch"] = DefaultBranch, SourceDefault
	}
	if v, ok := get("worktree-branch"); ok {
		if c.WorktreeBranch, err = singleTemplate("worktree-branch", v); err != nil {
			return Convention{}, err
		}
	} else {
		c.WorktreeBranch, c.Source["worktree-branch"] = DefaultWorktreeBranch, SourceDefault
	}
	if v, ok := get("accept"); ok {
		if c.Accept, err = templateList(v); err != nil {
			return Convention{}, err
		}
	}
	return c, nil
}

// LoadConvention loads the convention for tracker. mainRoot MUST be the main
// checkout (callers resolve it with git common-dir). The adapter comes from
// <main>/.claude/rules (else the plugin copy, as in Load), overlaid per key by the
// user-owned <main>/.claude/rules/tracker-<name>.board.md. pluginRoot is already
// validated by the caller ("" disables the fallback). Reasons: convention_undeclared,
// adapter_missing, convention_declaration_invalid, adapter_lags_plugin,
// board_links_ignored. An invalid declaration yields Declared=false (legacy behavior).
func LoadConvention(mainRoot, tracker, pluginRoot string) (Convention, []string) {
	if !NameRe.MatchString(tracker) {
		return Convention{Source: map[string]string{}}, []string{ReasonTrackerNameInvalid}
	}
	var reasons []string
	base, baseSource := rawConvention{keys: map[string]string{}}, SourceInstalled
	loadPlugin := func() (rawConvention, bool) {
		pf := pluginAdapterFile(pluginRoot, tracker)
		if pf == "" {
			return rawConvention{}, false
		}
		data, err := os.ReadFile(pf)
		if err != nil {
			return rawConvention{}, false
		}
		raw, err := parseRaw(string(data))
		if err != nil {
			return rawConvention{}, false
		}
		_, declared := raw.keys["id"]
		return raw, declared
	}
	if data, err := os.ReadFile(adapterFile(mainRoot, tracker)); err != nil {
		reasons = append(reasons, ReasonAdapterMissing)
		if raw, ok := loadPlugin(); ok {
			base, baseSource = raw, SourcePlugin
		}
	} else {
		raw, err := parseRaw(string(data))
		if err != nil {
			return Convention{Source: map[string]string{}}, append(reasons, ReasonConventionDeclarationInvalid)
		}
		base = raw
		if _, declared := raw.keys["id"]; !declared {
			if praw, ok := loadPlugin(); ok {
				base, baseSource = praw, SourcePlugin
				reasons = append(reasons, ReasonAdapterLagsPlugin)
			}
		}
	}
	board := rawConvention{keys: map[string]string{}}
	if data, err := os.ReadFile(boardFile(mainRoot, tracker)); err == nil {
		raw, err := parseRaw(string(data))
		if err != nil {
			return Convention{Source: map[string]string{}}, append(reasons, ReasonConventionDeclarationInvalid)
		}
		board = raw
		if decls, _ := declarations(section(string(data), "## Worktree")); decls["links"] != "" {
			reasons = append(reasons, ReasonBoardLinksIgnored)
		}
	}
	c, err := build(base, board, baseSource)
	if err != nil {
		return Convention{Source: map[string]string{}}, append(reasons, ReasonConventionDeclarationInvalid)
	}
	if !c.Declared {
		reasons = append(reasons, ReasonConventionUndeclared)
		if declaresWorktreeKeys(board) || declaresWorktreeKeys(base) {
			reasons = append(reasons, ReasonKeysIgnoredNoID)
		}
	}
	return c, reasons
}

// declaresWorktreeKeys reports whether raw carries a naming key that build()
// discards when no id: is declared.
func declaresWorktreeKeys(raw rawConvention) bool {
	for _, k := range []string{"branch", "worktree-branch", "accept"} {
		if _, ok := raw.keys[k]; ok {
			return true
		}
	}
	return false
}

// templateRegex compiles an anchored regex for tmpl and returns the capture index of {id}.
func (c Convention) templateRegex(tmpl string) (*regexp.Regexp, int, error) {
	if c.ID == nil {
		return nil, 0, errors.New("no id pattern")
	}
	var b strings.Builder
	b.WriteString("^")
	group, idIndex := 0, 0
	for rest := tmpl; rest != ""; {
		i, j := strings.Index(rest, "{id}"), strings.Index(rest, "{slug}")
		next, token := -1, ""
		switch {
		case i >= 0 && (j < 0 || i < j):
			next, token = i, "{id}"
		case j >= 0:
			next, token = j, "{slug}"
		}
		if next < 0 {
			b.WriteString(regexp.QuoteMeta(rest))
			break
		}
		b.WriteString(regexp.QuoteMeta(rest[:next]))
		if token == "{id}" {
			group++
			idIndex = group
			b.WriteString("(" + c.ID.String() + ")")
			group += c.ID.NumSubexp()
		} else {
			group++
			b.WriteString("(" + slugPattern + ")")
		}
		rest = rest[next+len(token):]
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	return re, idIndex, err
}

// ParseBranch tries Branch, WorktreeBranch and, only for CreatedAndAccepted, each
// Accept template. The first anchored match wins. ok is false when the convention is
// undeclared or its id is `nothing`.
func (c Convention) ParseBranch(branch string, set ShapeSet) (id, template string, ok bool) {
	if !c.Declared || c.ID == nil || branch == "" {
		return "", "", false
	}
	templates := []string{c.Branch, c.WorktreeBranch}
	if set == CreatedAndAccepted {
		templates = append(templates, c.Accept...)
	}
	for _, tmpl := range templates {
		if tmpl == "" {
			continue
		}
		re, idx, err := c.templateRegex(tmpl)
		if err != nil {
			continue
		}
		if m := re.FindStringSubmatch(branch); m != nil && m[idx] != "" {
			return m[idx], tmpl, true
		}
	}
	return "", "", false
}

// Render fills {id} and {slug} into tmpl. A slug outside [a-z0-9][a-z0-9-]{0,49} is
// rejected when the template uses one; with a declared id pattern the id must match
// it in full. Every rendered name is therefore shell-safe by construction.
func (c Convention) Render(tmpl, id, slug string) (string, error) {
	if err := validateTemplate(tmpl); err != nil {
		return "", err
	}
	if id == "" {
		return "", errors.New("empty id")
	}
	if c.ID != nil {
		full, err := regexp.Compile("^(?:" + c.ID.String() + ")$")
		if err != nil || !full.MatchString(id) {
			return "", fmt.Errorf("id %q does not match the convention's id pattern", id)
		}
	}
	if strings.Contains(tmpl, "{slug}") && !slugRe.MatchString(slug) {
		return "", fmt.Errorf("slug %q must match [a-z0-9][a-z0-9-]{0,49}", slug)
	}
	return strings.ReplaceAll(strings.ReplaceAll(tmpl, "{id}", id), "{slug}", slug), nil
}
