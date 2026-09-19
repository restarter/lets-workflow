//go:build unix

package peerscmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/restarter/lets-workflow/cli/internal/ccregistry"
	"github.com/restarter/lets-workflow/cli/internal/redact"
)

// AskROOptions configures AskRO.
type AskROOptions struct {
	Cwd       string // the hub's own checkout (holds the handoff)
	Repo      string // the other project's main checkout, or
	RepoIndex *int   // an index from `lets orca repos` (nil = unset)
	Session   string // the orchestrator's last-seen session
	Pid       int
	MsgID     string // from `lets peers frame --kind ask-ro`
}

// AskROResult is the ask-ro envelope.
type AskROResult struct {
	Envelope
	Answered bool   `json:"answered"`
	Reason   string `json:"reason,omitempty"` // main_alive | liveness_unknown | headless_readonly_unenforceable | headless_failed
	Answer   string `json:"answer,omitempty"`
}

// Seams.
var (
	claudeBin    = func() (string, error) { return exec.LookPath("claude") }
	askROTimeout = 5 * time.Minute
)

const askROAnswerCap = 8 << 10

// askROTools is the built-in tool allowlist (spike 6.0: with --tools the child has
// exactly these); the disallow list stays as a second fence.
const askROTools = "Read,Grep,Glob"

var askRORequiredFlags = []string{"--fork-session", "--permission-mode", "--tools", "--disallowedTools", "--strict-mcp-config", "--mcp-config"}

// askROArgs is the ONLY argv ask-ro runs. It narrows abilities at launch: a forked
// resume in plan mode, three read tools, no MCP servers, JSON output.
func askROArgs(sid, emptyMCP string) []string {
	args := []string{"-p", "--resume", sid, "--fork-session", "--permission-mode", "plan",
		"--tools", askROTools,
		"--disallowedTools", "Bash,Write,Edit,NotebookEdit,WebFetch,WebSearch,mcp__*",
		"--strict-mcp-config", "--mcp-config", emptyMCP,
		"--output-format", "json"}
	for _, a := range args {
		if strings.Contains(a, "dangerously") || strings.HasPrefix(a, "--allowed") {
			panic("ask-ro: widening flag " + a)
		}
	}
	return args
}

// askROEnv passes an allowlist, never the parent's environment: no session id, no
// Orca or LETS variables reach the child.
func askROEnv() []string {
	exact := map[string]bool{"HOME": true, "PATH": true, "LANG": true, "LC_ALL": true, "TERM": true, "USER": true, "TMPDIR": true, "SHELL": true,
		"CLAUDE_CONFIG_DIR": true, "ANTHROPIC_API_KEY": true, "ANTHROPIC_BASE_URL": true, "CLAUDE_CODE_USE_BEDROCK": true, "CLAUDE_CODE_USE_VERTEX": true,
		"GOOGLE_APPLICATION_CREDENTIALS": true, "CLOUD_ML_REGION": true, "HTTPS_PROXY": true, "HTTP_PROXY": true, "NO_PROXY": true,
		"SSL_CERT_FILE": true, "NODE_EXTRA_CA_CERTS": true}
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if exact[k] || strings.HasPrefix(k, "AWS_") {
			env = append(env, kv)
		}
	}
	return env
}

// AskRO asks a stopped orchestrator a read-only question by forking its session
// headlessly in its own checkout, with its abilities narrowed at launch rather than
// assumed. It refuses a live or unknown session (never a second process on a live
// MAIN) and a claude that cannot enforce the narrowing.
func AskRO(ctx context.Context, o AskROOptions) (*AskROResult, error) {
	res := &AskROResult{Envelope: newEnvelope("ask-ro")}
	fail := func(e *Error) (*AskROResult, error) {
		res.Error = &ErrorInfo{Kind: e.Kind, Message: e.Message}
		return res, e
	}
	if !ccregistry.ValidSession(o.Session) || o.Pid < 0 {
		return fail(&Error{Code: ExitUsage, Kind: "usage", Message: "ask-ro needs --session and a non-negative --pid"})
	}
	repo, given, e := otherRepo(ctx, o.Repo, o.RepoIndex)
	if e == nil && !given {
		e = &Error{Code: ExitNotInRepo, Kind: "repo_invalid", Message: "--repo is not a directory"}
	}
	if e != nil {
		return fail(e)
	}
	root, e := rootOf(o.Cwd)
	if e != nil {
		return fail(e)
	}
	res.OK = true
	switch ccregistry.Read(ccregistry.HomeDir()).Liveness(o.Session, o.Pid) {
	case ccregistry.Alive:
		res.Reason = "main_alive"
		return res, nil
	case ccregistry.Unknown:
		res.Reason = "liveness_unknown"
		return res, nil
	}
	bin, err := claudeBin()
	if err != nil {
		res.Reason = "headless_readonly_unenforceable"
		return res, nil
	}
	hctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	help, _ := exec.CommandContext(hctx, bin, "--help").CombinedOutput()
	cancel()
	for _, flag := range askRORequiredFlags {
		if !bytes.Contains(help, []byte(flag)) {
			res.Reason = "headless_readonly_unenforceable"
			return res, nil
		}
	}
	prompt, pe := readHandoff(root, o.MsgID)
	if pe != nil {
		res.OK = false
		if pe.Kind == "handoff_refused" { // a malformed handoff is not retryable; usage errors touch nothing
			consumeHandoff(root, o.MsgID)
		}
		return fail(pe)
	}
	if h, ok := leadingHeader(prompt); !ok || h.Kind != "ask-ro" || h.ToSID != o.Session {
		res.OK = false
		consumeHandoff(root, o.MsgID)
		return fail(&Error{Code: ExitGeneric, Kind: "handoff_refused", Message: "the handoff is not an ask-ro message to this session"})
	}
	// ask-ro is one-shot: the prompt becomes the forked process's stdin next, and there
	// is no separate "nothing was typed" case to keep it retryable for.
	consumeHandoff(root, o.MsgID)
	emptyMCP := filepath.Join(root, ".lets", "cache", "ask-ro-mcp-empty.json")
	if err := os.WriteFile(emptyMCP, []byte(`{"mcpServers":{}}`+"\n"), 0o600); err != nil {
		res.OK = false
		return fail(&Error{Code: ExitGeneric, Kind: "mcp_config_failed", Message: err.Error()})
	}
	_ = os.Chmod(emptyMCP, 0o600)
	rctx, rcancel := context.WithTimeout(ctx, askROTimeout)
	defer rcancel()
	cmd := exec.CommandContext(rctx, bin, askROArgs(o.Session, emptyMCP)...)
	cmd.Dir, cmd.Env, cmd.Stdin = repo, askROEnv(), strings.NewReader(prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		res.Reason = "headless_failed"
		if errors.Is(rctx.Err(), context.DeadlineExceeded) {
			res.Reason = "headless_timeout"
		}
		return res, nil
	}
	var out struct {
		Result string `json:"result"`
	}
	if json.Unmarshal(stdout.Bytes(), &out) != nil {
		res.Reason = "headless_output_unrecognized"
		return res, nil
	}
	res.Answered, res.Answer = true, redact.Cap(redact.Control(redact.Text(out.Result)), askROAnswerCap)
	return res, nil
}
