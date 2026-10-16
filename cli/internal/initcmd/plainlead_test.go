package initcmd

import (
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// lets-eolec: a standing team's lead is a plain Claude Code session - launched
// `{agent_command} --name '<lead_name>' '/lets:start'`, never `--agent lets:lead`,
// so the default system prompt and native memory stay. agents/lead.md is
// retired; what a member or a briefed agent needs to know about the lead lives in
// protocol/roles.md, the lead-only procedure in the team template's lead
// paragraph, which /lets:start has a `match=lead` session read.
//
// Re-verify with `-count=1`: Go's test cache does not track files reached via
// ../../../plugins/.

const retiredLeadAgent = "--agent lets:lead"

// plainLeadScanExt are the text files the no-`--agent lets:lead` scan reads; a
// built binary (cli/lets) or a lockfile is never a source of instructions.
var plainLeadScanExt = map[string]bool{".md": true, ".go": true, ".js": true, ".json": true, ".jsonl": true, ".yml": true, ".yaml": true, ".sh": true, ".txt": true}

func TestPlainLead_NoLeadAgentLaunch(t *testing.T) {
	root := filepath.Dir(filepath.Dir(pluginDir(t)))
	var hits []string
	for _, dir := range []string{"plugins", "cli"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(p, "_test.go") || !plainLeadScanExt[filepath.Ext(p)] {
				return nil
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			for i, l := range strings.Split(string(b), "\n") {
				if strings.Contains(l, retiredLeadAgent) {
					hits = append(hits, filepath.ToSlash(rel)+":"+strconv.Itoa(i+1))
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// CHANGELOG: only the unreleased section - a released entry is history.
	b, err := os.ReadFile(filepath.Join(root, "CHANGELOG.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sectionSpan(string(b), "## [Unreleased]"), retiredLeadAgent) {
		hits = append(hits, "CHANGELOG.md [Unreleased]")
	}
	for _, h := range hits {
		t.Errorf("%s names %q - the lead launches as a plain session (lets-eolec)", h, retiredLeadAgent)
	}
}

func TestPlainLead_AgentFileRetired(t *testing.T) {
	p := filepath.Join(pluginDir(t), "agents", "lead.md")
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Errorf("plugins/lets/agents/lead.md must not exist - the lead is a plain session, not a session agent (lets-eolec); stat err=%v", err)
	}
}

func TestPlainLead_RolesSection(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(pluginDir(t), "protocol", "roles.md"))
	if err != nil {
		t.Fatal(err)
	}
	sec := sectionSpan(string(b), "\n## The standing-team lead\n")
	if sec == "" {
		t.Fatal("protocol/roles.md must carry `## The standing-team lead` - what every member and briefed agent knows about the lead")
	}
	for _, s := range []string{"only writer", "Code goes to an implementer by default"} {
		if !strings.Contains(sec, s) {
			t.Errorf("protocol/roles.md `## The standing-team lead` lacks %q", s)
		}
	}
}

func TestPlainLead_StartReadsTeamFile(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(pluginDir(t), "commands", "start.md"))
	if err != nil {
		t.Fatal(err)
	}
	const lineStart = "**Team worktree** (`lets worktree info --json` reports a `team`): read its `team_lead`"
	line := ""
	for _, l := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(l, lineStart) {
			line = l
			break
		}
	}
	if line == "" {
		t.Fatalf("commands/start.md: no team-worktree line starting %q", lineStart)
	}
	if !strings.Contains(line, "`match` `lead` -> Read the team file `.lets/teams/<team>.md`") {
		t.Errorf("commands/start.md: the team-worktree line must tell `match` `lead` to Read the team file - its lead paragraph is the session's procedure: %s", line)
	}
}
