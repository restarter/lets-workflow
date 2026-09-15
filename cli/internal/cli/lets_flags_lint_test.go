package cli_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/restarter/lets-workflow/cli/internal/cli"
)

// pendingSubcommands are `lets <sub>` commands a later unit of lets-ip06f builds; an
// invocation of one is reported as pending (not failed) until that unit lands. The
// task that wires a subcommand removes it from this list.
var pendingSubcommands = map[string]string{
	"orca":  "U1 1.13",
	"peers": "U2 2.10",
}

// TestLetsFlagsLint pins the markdown-to-CLI contract: every `lets <sub> ... --flag`
// invocation in a command or skill file names a flag the resolved cobra command
// defines, a value flag is given its value, and a leaf command that takes no
// positional arguments is not handed a stray word. A flag the binary does not
// define otherwise fails only at runtime, in a user's session.
func TestLetsFlagsLint(t *testing.T) {
	root := cli.NewRootCmd()
	pluginDir := filepath.Join("..", "..", "..", "plugins", "lets")
	var files []string
	for _, glob := range []string{"commands/*.md", "skills/*/SKILL.md"} {
		m, err := filepath.Glob(filepath.Join(pluginDir, glob))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, m...)
	}
	if len(files) == 0 {
		t.Fatal("no plugin markdown found - test wiring broken")
	}
	checked := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, snippet := range codeSnippets(string(data)) {
			for _, inv := range letsInvocations(snippet) {
				if msg := lintInvocation(root, inv); msg != "" {
					if !strings.HasPrefix(msg, "pending:") {
						t.Errorf("%s: `lets %s`: %s", filepath.Base(filepath.Dir(f))+"/"+filepath.Base(f), strings.Join(inv, " "), msg)
					}
					continue
				}
				checked++
			}
		}
	}
	if checked == 0 {
		t.Fatal("no lets invocation with a flag was checked - scanner broken")
	}
}

var inlineCode = regexp.MustCompile("`([^`]+)`")

// codeSnippets returns every fenced code line and every inline code span.
func codeSnippets(md string) []string {
	var out []string
	inFence := false
	for _, line := range strings.Split(md, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			out = append(out, line)
			continue
		}
		for _, m := range inlineCode.FindAllStringSubmatch(line, -1) {
			out = append(out, m[1])
		}
	}
	return out
}

var letsWord = regexp.MustCompile(`(^|[\s(;&|$"'])lets\s+[a-z]`)

// letsInvocations tokenizes each `lets ...` command in a snippet up to a shell
// operator, and keeps only the ones that pass at least one --flag.
func letsInvocations(s string) [][]string {
	var out [][]string
	for _, loc := range letsWord.FindAllStringIndex(s, -1) {
		start := strings.Index(s[loc[0]:], "lets") + loc[0] + len("lets")
		toks := shellTokens(s[start:])
		hasFlag := false
		for _, tk := range toks {
			if strings.HasPrefix(tk, "--") {
				hasFlag = true
			}
		}
		if hasFlag {
			out = append(out, toks)
		}
	}
	return out
}

// shellTokens splits on whitespace honoring single and double quotes and $( ),
// stopping at an unquoted shell operator.
func shellTokens(s string) []string {
	var toks []string
	var cur strings.Builder
	inS, inD, depth := false, false, 0
	flush := func() {
		if cur.Len() > 0 {
			toks = append(toks, cur.String())
			cur.Reset()
		}
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inS:
			if c == '\'' {
				inS = false
			}
			cur.WriteByte(c)
		case inD:
			if c == '"' && depth == 0 {
				inD = false
			}
			if c == '(' && i > 0 && s[i-1] == '$' {
				depth++
			}
			if c == ')' && depth > 0 {
				depth--
			}
			cur.WriteByte(c)
		case c == '\'':
			inS = true
			cur.WriteByte(c)
		case c == '"':
			inD = true
			cur.WriteByte(c)
		case c == '(' && i > 0 && s[i-1] == '$':
			depth++
			cur.WriteByte(c)
		case c == ')' && depth > 0:
			depth--
			cur.WriteByte(c)
		case depth == 0 && (c == '|' || c == ';' || c == '&' || c == '>' || c == ')' || c == '#' || c == '\\'):
			flush()
			return toks
		case depth == 0 && (c == ' ' || c == '\t'):
			flush()
		default:
			cur.WriteByte(c)
		}
	}
	flush()
	return toks
}

// placeholder reports a token the markdown fills at run time.
func placeholder(tk string) bool {
	return strings.ContainsAny(tk, "{}<>$'\"") || strings.HasSuffix(tk, "...") || tk == "…"
}

// lintInvocation resolves the subcommand path and checks the flags. "" = clean.
func lintInvocation(root *cobra.Command, toks []string) string {
	cmd := root
	i := 0
	for ; i < len(toks); i++ {
		tk := toks[i]
		if strings.HasPrefix(tk, "-") || placeholder(tk) {
			break
		}
		next := childNamed(cmd, tk)
		if next == nil {
			if cmd == root {
				if unit, ok := pendingSubcommands[tk]; ok {
					return "pending: " + unit
				}
				return "unknown subcommand " + tk
			}
			break
		}
		cmd = next
	}
	var stray []string
	for ; i < len(toks); i++ {
		tk := toks[i]
		if !strings.HasPrefix(tk, "-") {
			if !placeholder(tk) {
				stray = append(stray, tk)
			}
			continue
		}
		name, _, hasValue := strings.Cut(strings.TrimLeft(tk, "-"), "=")
		if placeholder(name) || name == "" {
			continue
		}
		var fl *pflag.Flag
		if strings.HasPrefix(tk, "--") {
			fl = lookupFlag(cmd, name)
		} else if len(name) == 1 {
			fl = cmd.Flags().ShorthandLookup(name)
			if fl == nil {
				fl = cmd.InheritedFlags().ShorthandLookup(name)
			}
		}
		if fl == nil {
			return "flag " + tk + " is not defined on `" + cmd.CommandPath() + "`"
		}
		if fl.Value.Type() != "bool" && !hasValue {
			switch {
			case i+1 >= len(toks):
				// a trailing value flag is a mention of the flag (prose), not an invocation
			case strings.HasPrefix(toks[i+1], "--"):
				return "flag " + tk + " needs a value"
			default:
				i++ // its value
			}
		}
	}
	if len(stray) > 0 && !cmd.HasSubCommands() && len(strings.Fields(cmd.Use)) == 1 {
		return "`" + cmd.CommandPath() + "` takes no positional arguments, got " + strings.Join(stray, " ")
	}
	return ""
}

func childNamed(cmd *cobra.Command, name string) *cobra.Command {
	for _, c := range cmd.Commands() {
		if c.Name() == name || c.HasAlias(name) {
			return c
		}
	}
	return nil
}

func lookupFlag(cmd *cobra.Command, name string) *pflag.Flag {
	if f := cmd.Flags().Lookup(name); f != nil {
		return f
	}
	return cmd.InheritedFlags().Lookup(name)
}
