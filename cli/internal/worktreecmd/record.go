//go:build unix

package worktreecmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/restarter/lets-workflow/cli/internal/gitutil"
	"github.com/restarter/lets-workflow/cli/internal/taskid"
	"github.com/restarter/lets-workflow/cli/internal/taskstate"
	"github.com/restarter/lets-workflow/cli/internal/trackeradapter"
)

// Record states: did a session leave a snapshot that covers the task's tip.
const (
	RecordPresent = "present"
	RecordStale   = "stale"
	RecordMissing = "missing"
)

// headLineRe is the anchor session-snapshot writes into its ### Record block.
// Keep in sync with plugins/lets/skills/session-snapshot/SKILL.md (Step 3 template).
var headLineRe = regexp.MustCompile(`(?m)^- head: ([0-9a-f]{40})\s*$`)

// SnapshotRecord is the answer for one task.
type SnapshotRecord struct {
	State    string `json:"state"`              // present | stale | missing
	Snapshot string `json:"snapshot,omitempty"` // the covering snapshot, else the newest
	Head     string `json:"head,omitempty"`     // its recorded head, when anchored
	Detail   string `json:"detail,omitempty"`   // unanchored | tip_unknown
}

// recordOf reads the task's snapshots (artifact-path names them
// {date}-{HHMM}-{task}-snapshot[-vN].md) newest first. present: the tip is an
// ancestor of (or equal to) a recorded head. Anything else with at least one
// snapshot is stale - including a snapshot written before the head line existed.
// Ancestry, never time: clocks drift and a rebase rewrites committer dates.
func recordOf(ctx context.Context, repo, letsDir, task, tip string) SnapshotRecord {
	files, _ := filepath.Glob(filepath.Join(letsDir, "sessions", "*-"+task+"-snapshot*.md"))
	if len(files) == 0 {
		return SnapshotRecord{State: RecordMissing}
	}
	newestFirst(files)
	anchored := false
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		all := headLineRe.FindAllSubmatch(data, -1)
		if len(all) == 0 {
			continue
		}
		anchored = true
		head := string(all[len(all)-1][1])
		if tip != "" && isAncestor(ctx, repo, tip, head) {
			return SnapshotRecord{State: RecordPresent, Snapshot: f, Head: head}
		}
	}
	rec := SnapshotRecord{State: RecordStale, Snapshot: files[0]}
	switch {
	case !anchored:
		rec.Detail = "unanchored"
	case tip == "":
		rec.Detail = "tip_unknown"
	}
	return rec
}

// snapNameRe splits an artifact-path snapshot name into its stamp and its -vN
// collision suffix (keep in sync with skills/artifact-path/SKILL.md).
var snapNameRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}-\d{4})-.*-snapshot(?:-v(\d+))?\.md$`)

// newestFirst orders snapshot paths by (stamp, collision version) descending. A plain
// reverse sort gets both wrong: '.' sorts after '-' (the base before its own -v2) and
// v9 after v10.
func newestFirst(files []string) {
	key := func(p string) (string, int) {
		m := snapNameRe.FindStringSubmatch(filepath.Base(p))
		if m == nil {
			return "", 0
		}
		v := 1
		if m[2] != "" {
			v, _ = strconv.Atoi(m[2])
		}
		return m[1], v
	}
	sort.SliceStable(files, func(i, j int) bool {
		si, vi := key(files[i])
		sj, vj := key(files[j])
		if si != sj {
			return si > sj
		}
		return vi > vj
	})
}

// isAncestor reports whether a is an ancestor of (or equal to) b in repo. An unknown
// object (a gc'd head) is not an ancestor.
func isAncestor(ctx context.Context, repo, a, b string) bool {
	return exec.CommandContext(ctx, "git", "-C", repo, "merge-base", "--is-ancestor", a, b).Run() == nil
}

// TaskTrace is what this checkout still holds for one task, and whether that makes
// it an orphan: a local trace, no worktree holding it, no released marker.
type TaskTrace struct {
	Task      string         `json:"task"`
	TaskState []string       `json:"task_state"` // slugs whose .task-<slug> names the task
	Branches  []string       `json:"branches"`   // refs/heads/ branches carrying the id
	Worktrees []string       `json:"worktrees"`  // live worktrees holding one of them
	Marker    bool           `json:"marker"`
	Record    SnapshotRecord `json:"record"`
	Orphan    bool           `json:"orphan"`
}

// RecordOptions configures TaskRecord. Ref is the commit the record must cover and
// is only accepted with exactly one task.
type RecordOptions struct {
	Tasks []string
	Ref   string
}

// TaskRecord answers, per task: did a session leave a record covering its tip, and
// is the task orphaned in this checkout. Read-only; never calls a tracker - the
// caller decides which ids are in progress.
func TaskRecord(ctx context.Context, dir string, o RecordOptions) (*RecordResult, error) {
	res := &RecordResult{Envelope: Envelope{SchemaVersion: SchemaVersion, Subcommand: "record", Steps: []Step{}}, Tasks: []TaskTrace{}}
	fail := func(e *Error) (*RecordResult, error) {
		res.OK = false
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message, Remediation: e.Remediation}
		return res, e
	}
	if len(o.Tasks) == 0 {
		return fail(&Error{Code: ExitUsage, Kind: "usage", Message: "pass at least one --task"})
	}
	if o.Ref != "" && len(o.Tasks) != 1 {
		return fail(&Error{Code: ExitUsage, Kind: "usage", Message: "--ref needs exactly one --task"})
	}
	for _, id := range o.Tasks {
		if !taskid.Valid(id) {
			return fail(&Error{Code: ExitUsage, Kind: "usage", Message: fmt.Sprintf("not a task id: %q", stripControl(id))})
		}
	}
	_, mainRoot := gitutil.DetectInsideWorktreeAt(dir)
	if mainRoot == "" {
		return fail(&Error{Code: ExitNotInRepo, Kind: "not_in_repo", Message: "not inside a git repository"})
	}
	res.ProjectRoot = mainRoot
	letsDir := filepath.Join(mainRoot, ".lets")
	out, err := exec.CommandContext(ctx, "git", "-C", mainRoot, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return fail(&Error{Code: ExitGitFailed, Kind: "git_failed", Message: "git worktree list failed"})
	}
	var live []porcelainEntry
	for _, e := range parsePorcelain(string(out)) {
		if !e.Prunable && !e.Bare {
			live = append(live, e)
		}
	}
	// An incomplete inventory is an error, never an empty one: orphan=false is a
	// safety answer (the merge rule, /lets:start --main) and must not come from a scan
	// that failed half-way.
	states, err := taskStatesByTask(letsDir)
	if err != nil {
		return fail(&Error{Code: ExitFilesystem, Kind: "trace_unreadable", Message: err.Error(), Remediation: "fix the file's permissions or content, then run record again"})
	}
	branches, grammar, err := branchesByTask(ctx, mainRoot)
	if err != nil {
		return fail(&Error{Code: ExitGitFailed, Kind: "git_failed", Message: "git for-each-ref failed: " + err.Error()})
	}
	if !grammar {
		res.Steps = append(res.Steps, Step{Status: StepWarn, Message: "the tracker convention declares no task-id grammar - branch traces skipped, task-state traces only"})
	}
	for _, id := range o.Tasks {
		tr := TaskTrace{Task: id, TaskState: nonNil(states[id]), Branches: nonNil(branches[id]), Worktrees: []string{}}
		tip := ""
		for _, e := range live {
			slug, _ := taskstate.Slug(e.Branch)
			if e.Branch != "" && (contains(tr.Branches, e.Branch) || contains(tr.TaskState, slug)) {
				tr.Worktrees = append(tr.Worktrees, e.Path)
				if tip == "" {
					tip = e.HEAD
				}
			}
		}
		if tip == "" && len(tr.Branches) > 0 {
			tip = revParse(ctx, mainRoot, "refs/heads/"+tr.Branches[0])
		}
		if o.Ref != "" {
			if tip = revParse(ctx, mainRoot, o.Ref); tip == "" {
				return fail(&Error{Code: ExitUsage, Kind: "ref_unknown", Message: "--ref does not name a commit"})
			}
		}
		if _, err := os.Stat(filepath.Join(letsDir, "cache", "released-"+id)); err == nil {
			tr.Marker = true
		} else if !errors.Is(err, os.ErrNotExist) {
			// Unknown is not absent: an orphan=true from an unreadable marker would offer a
			// task for Reopen that already has its record.
			res.Tasks = []TaskTrace{}
			return fail(&Error{Code: ExitFilesystem, Kind: "trace_unreadable", Message: "released marker of " + id + ": " + err.Error()})
		}
		tr.Record = recordOf(ctx, mainRoot, letsDir, id, tip)
		tr.Orphan = (len(tr.TaskState) > 0 || len(tr.Branches) > 0) && len(tr.Worktrees) == 0 && !tr.Marker
		res.Tasks = append(res.Tasks, tr)
	}
	res.OK = true
	return res, nil
}

// taskStatesByTask maps task id -> slugs of the .task-<slug> files that name it.
// taskstate enumerates its own files (it alone knows its temp-file names); a file that
// cannot be read is an error, not an absence.
func taskStatesByTask(letsDir string) (map[string][]string, error) {
	out := map[string][]string{}
	slugs, err := taskstate.Slugs(letsDir)
	if err != nil {
		return nil, err
	}
	for _, slug := range slugs {
		st, err := taskstate.Read(letsDir, slug)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			continue // removed since the listing
		case err != nil:
			return nil, fmt.Errorf("task-state file %q: %w", stripControl(slug), err)
		case taskid.Valid(st.Task):
			out[st.Task] = append(out[st.Task], slug)
		}
	}
	return out, nil
}

// branchesByTask maps task id -> local branches (refs/heads/ only - a fetched
// colleague's branch is never a trace) in a created or accepted shape. grammar=false:
// the convention declares no id grammar, so no branch name can be read as an id -
// guessing one would false-match (a numeric-id tracker's feature/48647-lifecycle-test
// under the beads shape reads as `lifecycle-test`).
func branchesByTask(ctx context.Context, mainRoot string) (out map[string][]string, grammar bool, err error) {
	out = map[string][]string{}
	conv := loadConvention(mainRoot)
	if !conv.Declared || conv.ID == nil {
		return out, false, nil
	}
	refs, err := forEachRef(ctx, mainRoot)
	if err != nil {
		return nil, true, err
	}
	for _, b := range strings.Split(strings.TrimSpace(string(refs)), "\n") {
		if b == "" {
			continue
		}
		if id, _, ok := conv.ParseBranch(b, trackeradapter.CreatedAndAccepted); ok {
			out[id] = append(out[id], b)
		}
	}
	return out, true, nil
}

// forEachRef lists the local branch names (a seam: a test makes it fail).
var forEachRef = func(ctx context.Context, mainRoot string) ([]byte, error) {
	return exec.CommandContext(ctx, "git", "-C", mainRoot, "for-each-ref", "--format=%(refname:short)", "refs/heads/").Output()
}

func revParse(ctx context.Context, repo, ref string) string {
	out, err := exec.CommandContext(ctx, "git", "-C", repo, "rev-parse", "--verify", "--quiet", ref+"^{commit}").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func nonNil(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}
