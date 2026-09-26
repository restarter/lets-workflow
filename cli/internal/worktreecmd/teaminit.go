//go:build unix

package worktreecmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/teamfile"
)

// TeamInitOptions configures TeamInit. AgentCommand is how the lead is launched
// (default `claude`); OrcaAgent is the shell command an Orca lead terminal runs
// (default `claude`). Suggest only proposes a free callsign and writes nothing.
type TeamInitOptions struct {
	Callsign, Area, Worktree, PluginRoot, AgentCommand, OrcaAgent string
	Suggest                                                       bool
}

// TeamInitResult is `lets worktree team-init`.
type TeamInitResult struct {
	Envelope
	Callsign  string `json:"callsign,omitempty"`
	TeamFile  string `json:"team_file,omitempty"`
	Suggested bool   `json:"suggested,omitempty"`
}

// TeamInit writes the standing team's file `.lets/teams/<callsign>.md` (in the
// main checkout's .lets, shared by every worktree) from the plugin template. The
// file is never overwritten, by any writer: it is rendered to a fsynced temp file
// in the same directory and published with os.Link, which the filesystem refuses
// when the target exists (exit 27). Two racing inits both pass the live-lead
// check; the link picks exactly one. A filesystem that refuses links fails the
// call with nothing written - no fallback, since O_EXCL would expose a partial
// file and rename would overwrite.
func TeamInit(ctx context.Context, dir string, o TeamInitOptions) (*TeamInitResult, error) {
	res := &TeamInitResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "team-init", Steps: []Step{}}}
	fail := func(e *Error) (*TeamInitResult, error) {
		res.OK = false
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message, Remediation: e.Remediation}
		return res, e
	}
	_, mainRoot := gitutil.DetectInsideWorktreeAt(dir)
	if mainRoot == "" {
		return fail(&Error{Code: ExitNotInRepo, Kind: "not_in_repo", Message: "not inside a git repository"})
	}
	res.ProjectRoot = mainRoot
	teamsDir := filepath.Join(mainRoot, ".lets", "teams")
	live := liveSessionNames(res)

	if o.Suggest {
		c, ok := teamfile.FreeCallsign(teamsDir, func(name string) bool { return live[name] })
		if !ok {
			return fail(&Error{Code: ExitGeneric, Kind: "no_free_callsign", Message: "every callsign has a team file or a live lead"})
		}
		res.Callsign, res.Suggested, res.OK = c, true, true
		res.Steps = append(res.Steps, Step{Status: StepOK, Message: "proposed callsign " + c + " (nothing written)"})
		return res, nil
	}

	if !teamfile.ValidName(o.Callsign) {
		return fail(&Error{Code: ExitUsage, Kind: "callsign_invalid", Message: fmt.Sprintf("--callsign %q is not a team name ([a-z0-9-]{1,40}, not run-*)", o.Callsign)})
	}
	res.Callsign = o.Callsign
	if strings.TrimSpace(o.Area) == "" || o.Worktree == "" || !filepath.IsAbs(o.Worktree) {
		return fail(&Error{Code: ExitUsage, Kind: "usage", Message: "--area and an absolute --worktree are required"})
	}
	agent, orca := o.AgentCommand, o.OrcaAgent
	if agent == "" {
		agent = "claude"
	}
	if orca == "" {
		orca = "claude"
	}
	for _, f := range []struct{ flag, v string }{{"--agent-command", agent}, {"--orca-agent", orca}, {"--area", o.Area}, {"--worktree", o.Worktree}} {
		if strings.ContainsFunc(f.v, unicode.IsControl) {
			return fail(&Error{Code: ExitUsage, Kind: "control_char", Message: f.flag + " carries a newline or control character"})
		}
	}

	root := o.PluginRoot
	if root == "" {
		root = os.Getenv("CLAUDE_PLUGIN_ROOT")
	}
	tmplPath := filepath.Join(root, "templates", "team.md")
	tmpl, err := os.ReadFile(tmplPath)
	if root == "" || err != nil {
		return fail(ErrTemplateMissing(tmplPath))
	}

	target := filepath.Join(teamsDir, o.Callsign+".md")
	if _, err := os.Lstat(target); err == nil {
		return fail(ErrTeamExists(target))
	}
	if live[o.Callsign+"-lead"] {
		return fail(ErrCallsignLive(o.Callsign))
	}

	git := func(args ...string) (string, error) {
		out, err := exec.CommandContext(ctx, "git", append([]string{"-C", o.Worktree}, args...)...).Output()
		return strings.TrimSpace(string(out)), err
	}
	gitDir, err := git("rev-parse", "--absolute-git-dir")
	if err != nil {
		return fail(&Error{Code: ExitGitFailed, Kind: "worktree_invalid", Message: o.Worktree + " is not a git worktree", Cause: err})
	}
	base, err := git("rev-parse", "HEAD")
	if err != nil {
		return fail(&Error{Code: ExitGitFailed, Kind: "git_failed", Message: "no HEAD commit in " + o.Worktree, Cause: err})
	}
	body, err := teamfile.Render(tmpl, teamfile.Values{
		"team": o.Callsign, "area": o.Area, "repo": mainRoot, "worktree": o.Worktree, "git_dir": gitDir,
		"base": base, "created": time.Now().UTC().Format(time.RFC3339), "lead_name": o.Callsign + "-lead",
		"agent_command": agent, "orca_agent": orca,
	})
	if err != nil {
		return fail(&Error{Code: ExitUsage, Kind: "render_failed", Message: err.Error(), Cause: err})
	}
	if e := publishNoOverwrite(teamsDir, target, body); e != nil {
		return fail(e)
	}
	res.TeamFile, res.OK = target, true
	res.Steps = append(res.Steps, Step{Status: StepOK, Message: "team file written: " + target})
	return res, nil
}

// BeforeTeamLink, when set, runs right before the team file's os.Link - a seam,
// exported for the external test package, that lets a test create the target
// between the early Lstat and the link (the race the link exists to win).
var BeforeTeamLink func(target string)

// publishNoOverwrite writes body to a fsynced `.<name>.tmp-<random>` in dir, links
// it to target (EEXIST -> ErrTeamExists) and always removes the temp file.
func publishNoOverwrite(dir, target string, body []byte) *Error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &Error{Code: ExitFilesystem, Kind: "mkdir_failed", Message: err.Error(), Cause: err}
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(target)+".tmp-*")
	if err != nil {
		return &Error{Code: ExitFilesystem, Kind: "write_failed", Message: err.Error(), Cause: err}
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	_, werr := tmp.Write(body)
	if werr == nil {
		werr = tmp.Chmod(0o644)
	}
	if werr == nil {
		werr = tmp.Sync()
	}
	if cerr := tmp.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return &Error{Code: ExitFilesystem, Kind: "write_failed", Message: werr.Error(), Cause: werr}
	}
	if BeforeTeamLink != nil {
		BeforeTeamLink(target)
	}
	if err := os.Link(tmpPath, target); err != nil {
		if errors.Is(err, os.ErrExist) || errors.Is(err, syscall.EEXIST) {
			return ErrTeamExists(target)
		}
		return &Error{Code: ExitFilesystem, Kind: "link_refused", Message: "the filesystem refused to link the team file (" + err.Error() + "); nothing was written", Cause: err}
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}

// liveSessionNames reads the Claude Code session registry once: the name of every
// live session. A degraded registry cannot prove a name free; it is named as a
// warning, and only the readable rows count.
func liveSessionNames(res *TeamInitResult) map[string]bool {
	snap := ccregistry.Read(ccregistry.HomeDir())
	if snap.Degraded != nil {
		res.Steps = append(res.Steps, Step{Status: StepWarn, Message: "session registry degraded (" + snap.Degraded.Reason + "): a live lead there cannot be seen"})
	}
	names := map[string]bool{}
	for _, e := range snap.Entries {
		if e.NameOK {
			names[e.Name] = true
		}
	}
	return names
}
