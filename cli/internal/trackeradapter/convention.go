package trackeradapter

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"strings"
	"unicode"
)

// ErrConventionInvalid: an id / branch / worktree-branch / accept declaration does
// not validate. The error text names the key.
var ErrConventionInvalid = errors.New("convention declaration invalid")

// Convention reasons (LoadConvention).
const (
	ReasonConventionUndeclared         = "convention_undeclared"
	ReasonConventionDeclarationInvalid = "convention_declaration_invalid"
	ReasonBoardLinksIgnored            = "board_links_ignored"
	// ReasonTemplatesWithoutID: branch: / worktree-branch: are declared but no id:
	// is - they name the branches LETS creates, and nothing reads an id back off a
	// branch name (lets-puvic).
	ReasonTemplatesWithoutID = "convention_templates_without_id"
	// ReasonKeysIgnoredNoID: accept: (parse-only) is declared but no id: is, so it
	// is ignored. Before lets-puvic it also covered branch: / worktree-branch:.
	ReasonKeysIgnoredNoID = "convention_keys_ignored_no_id"
	// ReasonLinesUnread: the board holds a line that starts with a naming key
	// outside its ## Worktree section - LETS does not read it.
	ReasonLinesUnread = "convention_lines_unread"
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
// them with taskid.Valid. LoadConvention always sets Branch and WorktreeBranch
// (board, adapter, plugin or default); Declared says only whether an id grammar
// exists.
type Convention struct {
	Declared       bool              // false: no id: line anywhere - nothing reads an id off a name (legacy parsing); Branch / WorktreeBranch still render
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

// nearMissRe: a line that starts with a bare naming key (optionally behind a list
// marker or indentation). One that declRe does not match is a mistake, not prose -
// reading past it would drop the user's naming in silence (lets-puvic). A key in
// backticks is prose ("`branch:` / `worktree-branch:` are the names ...", the
// TEMPLATE's "- **`id:`**" bullets) and never matches.
var nearMissRe = regexp.MustCompile(`^\s*(?:[-*+]\s+)?(id|branch|worktree-branch|accept)\s*:`)

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
	for _, l := range strings.Split(section(content, "## Worktree"), "\n") {
		l = strings.TrimRight(l, " \t\r")
		if m := nearMissRe.FindStringSubmatch(l); m != nil && !declRe.MatchString(l) {
			return rawConvention{}, fmt.Errorf("%w: %s: line %q is not a declaration - one per line, `%s: ` then the value in backticks, ending with a period", ErrConventionInvalid, m[1], l, m[1])
		}
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

// build overlays board keys onto base keys, applies defaults, and compiles the
// result. branch: / worktree-branch: name what LETS creates with or without id:;
// only reading an id back off a name needs the grammar (lets-puvic).
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
	idVal, hasID := get("id")
	if !hasID {
		return c, nil // Declared=false: nothing parses; accept: is parse-only and unused
	}
	c.Declared = true
	if idVal != "nothing" {
		re, err := compileID(idVal)
		if err != nil {
			return Convention{}, err
		}
		c.ID = re
	}
	if v, ok := get("accept"); ok {
		if c.Accept, err = templateList(v); err != nil {
			return Convention{}, err
		}
	}
	return c, nil
}

// fallback is the convention of a failed load: undeclared, default templates.
func fallback() Convention {
	c, _ := build(rawConvention{keys: map[string]string{}}, rawConvention{keys: map[string]string{}}, SourceInstalled)
	return c
}

// Diagnosis is LoadConvention's account of what it could not use and why.
type Diagnosis struct {
	Reasons []string
	// Warnings: one line per drop that changes what branch-name / switch produce,
	// kind-prefixed ("<reason>: ..."), with its remediation. Never multi-line.
	Warnings []string
	// Invalid: "<repo-relative file>: <parser error>" when a naming declaration
	// could not be read or parsed; a renderer must refuse rather than guess.
	Invalid string
}

// LoadConvention loads the convention for tracker (see LoadConventionDiagnosed).
func LoadConvention(mainRoot, tracker, pluginRoot string) (Convention, []string) {
	c, d := LoadConventionDiagnosed(mainRoot, tracker, pluginRoot)
	return c, d.Reasons
}

// LoadConventionDiagnosed loads the convention for tracker. mainRoot MUST be the
// main checkout (callers resolve it with git common-dir). The adapter comes from
// <main>/.claude/rules (else the plugin copy, as in Load), overlaid per key by the
// user-owned <main>/.claude/rules/tracker-<name>.board.md. pluginRoot is already
// validated by the caller ("" disables the fallback). Reasons: convention_undeclared,
// adapter_missing, convention_declaration_invalid, adapter_lags_plugin,
// board_links_ignored, convention_templates_without_id, convention_keys_ignored_no_id,
// tracker_name_invalid. A failed load returns fallback() - undeclared, default
// templates - and says why.
func LoadConventionDiagnosed(mainRoot, tracker, pluginRoot string) (Convention, Diagnosis) {
	var d Diagnosis
	warn := func(reason, msg string) { d.Warnings = append(d.Warnings, reason+": "+msg) }
	invalid := func(rel string, err error) (Convention, Diagnosis) {
		d.Reasons = append(d.Reasons, ReasonConventionDeclarationInvalid)
		// No warning line: Invalid is consumed only by the renderers, which refuse
		// with it - a warning too would print the same fact twice.
		d.Invalid = rel + ": " + oneLine(strings.TrimPrefix(err.Error(), ErrConventionInvalid.Error()+": "))
		return fallback(), d
	}
	unread := func(rel, content string) {
		if n := linesOutsideSection(content, "## Worktree"); n > 0 {
			d.Reasons = append(d.Reasons, ReasonLinesUnread)
			warn(ReasonLinesUnread, fmt.Sprintf("%s has %d line(s) starting with id: / branch: / worktree-branch: / accept: that LETS does not read - naming is read only under the first `## Worktree` heading", rel, n))
		}
	}
	if !NameRe.MatchString(tracker) {
		d.Reasons = append(d.Reasons, ReasonTrackerNameInvalid)
		warn(ReasonTrackerNameInvalid, "LETS_TRACKER is not a valid adapter name - the default naming is used. Fix LETS_TRACKER in .lets/.env")
		return fallback(), d
	}
	adapterRel, boardRel := adapterFile("", tracker), boardFile("", tracker)
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
	data, err := os.ReadFile(adapterFile(mainRoot, tracker))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		d.Reasons = append(d.Reasons, ReasonAdapterMissing)
		if raw, ok := loadPlugin(); ok {
			base, baseSource = raw, SourcePlugin
		} else {
			warn(ReasonAdapterMissing, adapterRel+" is not installed in the main checkout, so no ## Worktree naming is read from it. Run /lets:update for a shipped tracker, or put your adapter file there")
		}
	case err != nil:
		return invalid(adapterRel, err)
	default:
		raw, err := parseRaw(string(data))
		if err != nil {
			return invalid(adapterRel, err)
		}
		base = raw
		unread(adapterRel, string(data))
		if _, declared := raw.keys["id"]; !declared {
			if praw, ok := loadPlugin(); ok {
				if declaresKeys(raw, "branch", "worktree-branch") {
					warn(ReasonAdapterLagsPlugin, "branch: / worktree-branch: in "+adapterRel+" are ignored - the plugin's copy, which declares id:, is used instead. Run /lets:update, or add id: to the installed adapter")
				}
				base, baseSource = praw, SourcePlugin
				d.Reasons = append(d.Reasons, ReasonAdapterLagsPlugin)
			}
		}
	}
	board := rawConvention{keys: map[string]string{}}
	data, err = os.ReadFile(boardFile(mainRoot, tracker))
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return invalid(boardRel, err)
	default:
		raw, err := parseRaw(string(data))
		if err != nil {
			return invalid(boardRel, err)
		}
		board = raw
		if decls, _ := declarations(section(string(data), "## Worktree")); decls["links"] != "" {
			d.Reasons = append(d.Reasons, ReasonBoardLinksIgnored)
			warn(ReasonBoardLinksIgnored, "links: in "+boardRel+" is ignored - store links come from the adapter only")
		}
		unread(boardRel, string(data))
	}
	c, err := build(base, board, baseSource)
	if err != nil {
		return invalid("the merged ## Worktree convention", err)
	}
	if !c.Declared {
		d.Reasons = append(d.Reasons, ReasonConventionUndeclared)
		if declaresKeys(board, "branch", "worktree-branch") || declaresKeys(base, "branch", "worktree-branch") {
			d.Reasons = append(d.Reasons, ReasonTemplatesWithoutID)
			warn(ReasonTemplatesWithoutID, templatesWithoutIDMessage(c, tracker))
		}
		if declaresKeys(board, "accept") || declaresKeys(base, "accept") {
			d.Reasons = append(d.Reasons, ReasonKeysIgnoredNoID)
			warn(ReasonKeysIgnoredNoID, "accept: is ignored - it only tells adopt which names carry a task id, and no id: is declared. Declare id: under ## Worktree in "+boardRel)
		}
	}
	return c, d
}

// linesOutsideSection counts lines that start with a bare naming key outside the
// span section(content, heading) returns - the first heading's body, up to the
// next `## `. It mirrors section() line for line, so "unread" means exactly "not
// parsed": a key under another heading, before the section, or in a second
// section with the same heading.
func linesOutsideSection(content, heading string) int {
	lines := strings.Split(content, "\n")
	start, end := -1, len(lines)
	for i, l := range lines {
		if strings.TrimRight(l, " \t\r") == heading {
			start = i + 1
			break
		}
	}
	if start >= 0 {
		for i := start; i < len(lines); i++ {
			if strings.HasPrefix(lines[i], "## ") {
				end = i
				break
			}
		}
	}
	n := 0
	for i, l := range lines {
		if start >= 0 && i >= start && i < end {
			continue
		}
		if nearMissRe.MatchString(strings.TrimRight(l, " \t\r")) {
			n++
		}
	}
	return n
}

// declaresKeys reports whether raw carries any of keys.
func declaresKeys(raw rawConvention, keys ...string) bool {
	for _, k := range keys {
		if _, ok := raw.keys[k]; ok {
			return true
		}
	}
	return false
}

// templatesWithoutIDMessage names each honoured template and where it came from.
func templatesWithoutIDMessage(c Convention, tracker string) string {
	var parts []string
	for _, k := range []string{"branch", "worktree-branch"} {
		src := c.Source[k]
		if src == SourceDefault {
			continue
		}
		tmpl := c.Branch
		if k == "worktree-branch" {
			tmpl = c.WorktreeBranch
		}
		parts = append(parts, fmt.Sprintf("%s: `%s` (%s)", k, tmpl, sourceFile(src, tracker)))
	}
	return "new branches follow " + strings.Join(parts, "; ") + ", but no id: is declared - LETS reads no task id back off a branch name (detect-task, adopt, sweep and the statusline keep the legacy shapes; the task-state file still names the task). Declare id: (your task-id pattern, e.g. `[0-9]+`) under ## Worktree in " + boardFile("", tracker)
}

// sourceFile names the file behind a Source value.
func sourceFile(src, tracker string) string {
	switch src {
	case SourceBoard:
		return boardFile("", tracker)
	case SourcePlugin:
		return "the plugin's rules/tracker-" + tracker + ".md"
	default:
		return adapterFile("", tracker)
	}
}

// oneLine makes untrusted text (a parser error quoting a user file) safe for one
// warning line in model context: control characters (C0, DEL, C1) and the Unicode
// line / paragraph separators become `?`, at most 200 bytes.
func oneLine(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == 0x2028 || r == 0x2029 {
			return '?'
		}
		return r
	}, s)
	if len(s) > 200 {
		s = strings.ToValidUTF8(s[:200], "") + "..."
	}
	return s
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
