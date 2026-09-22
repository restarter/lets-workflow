//go:build unix

package worktreecmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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
