//go:build unix

package agentrun

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type turnState int

const (
	stateNoMarker turnState = iota // the rollout never carried the delivered prompt
	statePending                   // the prompt's turn has not ended yet
	stateComplete
	stateAborted
)

// rolloutScan is what one rollout says about a delivered prompt.
type rolloutScan struct {
	sessionID string
	root      bool // session_meta names a user thread, not a subagent or a fork
	state     turnState
	message   string
}

type rolloutRecord struct {
	Timestamp string `json:"timestamp"`
	Type      string `json:"type"`
	Payload   struct {
		Type             string `json:"type"`
		Role             string `json:"role"`
		ID               string `json:"id"`
		ThreadSource     string `json:"thread_source"`
		TurnID           string `json:"turn_id"`
		LastAgentMessage string `json:"last_agent_message"`
	} `json:"payload"`
}

// at is the record's time; an unparsable timestamp is the zero time.
func (r rolloutRecord) at() time.Time {
	t, _ := time.Parse(time.RFC3339Nano, r.Timestamp)
	return t
}

var (
	kwTaskStarted  = []byte(`"task_started"`)
	kwTaskComplete = []byte(`"task_complete"`)
	kwTurnAborted  = []byte(`"turn_aborted"`)
)

// scanRollout reads one rollout. The delivered prompt is the first user message
// (response_item, payload message, role user) that carries the marker and is not
// older than `since`; its turn is the last task_started before it, and the
// task_complete or turn_aborted with that turn_id ends it (the first end after the
// prompt when the rollout has no turn ids). last_agent_message of that
// task_complete is the report. A marker in a tool output, an agent message, an
// item_completed echo or an older turn never matches, and a subagent's own
// agent_message never becomes the report. A nil marker (a headless run's rollout)
// takes the first task_complete. Only lines that can matter are decoded: a rollout
// holds whole tool outputs.
func scanRollout(path string, marker []byte, since time.Time) rolloutScan {
	out := rolloutScan{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer func() { _ = f.Close() }()
	rd := bufio.NewReader(f)
	matched := marker == nil
	if matched {
		out.state = statePending
	}
	turn, want := "", ""
	first := true
	for {
		line, err := rd.ReadBytes('\n')
		relevant := first || bytes.Contains(line, kwTaskStarted) ||
			(matched && (bytes.Contains(line, kwTaskComplete) || bytes.Contains(line, kwTurnAborted))) ||
			(!matched && bytes.Contains(line, marker))
		var rec rolloutRecord
		if relevant && len(bytes.TrimSpace(line)) > 0 && json.Unmarshal(line, &rec) == nil {
			switch {
			case first && rec.Type == "session_meta":
				out.sessionID = rec.Payload.ID
				out.root = rec.Payload.ThreadSource == "" || rec.Payload.ThreadSource == "user"
			case rec.Type == "event_msg" && rec.Payload.Type == "task_started":
				turn = rec.Payload.TurnID
			case !matched && bytes.Contains(line, marker) && rec.Type == "response_item" && rec.Payload.Type == "message" && rec.Payload.Role == "user" && !rec.at().Before(since):
				matched, want, out.state = true, turn, statePending
			case matched && rec.Type == "event_msg" && (want == "" || rec.Payload.TurnID == want):
				switch rec.Payload.Type {
				case "task_complete":
					out.state, out.message = stateComplete, rec.Payload.LastAgentMessage
					return out
				case "turn_aborted":
					out.state = stateAborted
					return out
				}
			}
		}
		if len(line) > 0 {
			first = false
		}
		if err != nil {
			return out
		}
	}
}

// recentRollouts lists every rollout modified since `since` (5 s skew allowed),
// newest first. All date directories are scanned: a rollout lives under the date
// its session STARTED, so a tab opened days ago still writes there.
func recentRollouts(since time.Time) []string {
	m, _ := filepath.Glob(filepath.Join(codexHome(), "sessions", "*", "*", "*", "rollout-*.jsonl"))
	floor := since.Add(-5 * time.Second)
	type file struct {
		path string
		mod  time.Time
	}
	var files []file
	for _, p := range m {
		if fi, err := os.Stat(p); err == nil && !fi.ModTime().Before(floor) {
			files = append(files, file{p, fi.ModTime()})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.path
	}
	return out
}

// Await waits for the turn the marker's prompt started in a Codex session someone
// else runs (an Orca tab). It polls the session store with a 1-5 s backoff,
// re-reading a rollout only when its mtime changed. Exactly one root rollout may
// carry the prompt; two are marker_ambiguous (the same brief sent twice).
func (codex) Await(ctx context.Context, r AwaitRequest) Result {
	report, _, _ := Outputs(r.OutBase)
	res := Result{Provider: "codex", Ran: true, ReportPath: report, Warnings: []string{}}
	if _, err := os.Lstat(report); err == nil {
		res.Reason = ReasonReportExists
		return res
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	start := now()
	deadline := start.Add(timeout)
	since := r.Since.Add(-5 * time.Second)
	mtimes := map[string]time.Time{}
	scans := map[string]rolloutScan{}
	backoff := time.Second
	for {
		for _, p := range recentRollouts(r.Since) {
			fi, err := os.Stat(p)
			if err != nil || mtimes[p].Equal(fi.ModTime()) {
				continue
			}
			mtimes[p] = fi.ModTime()
			scans[p] = scanRollout(p, []byte(r.Marker), since)
		}
		var hits []string
		for p, s := range scans {
			if s.root && s.state != stateNoMarker {
				hits = append(hits, p)
			}
		}
		sort.Strings(hits)
		switch {
		case len(hits) > 1:
			res.DurationMS = now().Sub(start).Milliseconds()
			res.Reason = ReasonMarkerAmbiguous
			res.Warnings = append(res.Warnings, "rollouts carrying the brief: "+strings.Join(hits, ", "))
			return res
		case len(hits) == 1:
			s := scans[hits[0]]
			res.RolloutPath, res.SessionID = hits[0], s.sessionID
			switch s.state {
			case stateComplete:
				res.DurationMS = now().Sub(start).Milliseconds()
				if strings.TrimSpace(s.message) == "" {
					res.Reason = ReasonReportEmpty
					return res
				}
				if err := writeNew(report, clean(s.message)); err != nil {
					res.Reason, res.StderrTail = ReasonIO, clip(err.Error())
					return res
				}
				checkDrift(&res, r)
				res.Complete = true
				return res
			case stateAborted:
				res.DurationMS = now().Sub(start).Milliseconds()
				res.Reason = ReasonTurnAborted
				return res
			}
		}
		if ctx.Err() != nil || !now().Before(deadline) {
			res.DurationMS = now().Sub(start).Milliseconds()
			switch {
			case ctx.Err() != nil && now().Before(deadline):
				res.Reason = ReasonCanceled
			case res.RolloutPath == "":
				res.Reason = ReasonMarkerNotFound
			default:
				res.Reason = ReasonTimeout
			}
			return res
		}
		sleep(backoff)
		if backoff < 5*time.Second {
			backoff += time.Second
		}
	}
}
