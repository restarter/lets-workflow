# goal-test findings

Empirical observations from `/goal` experiments + web-research log. Newest at top.

---

### 2026-05-25 · Web research · `/goal` best practices and gotchas

Delegated to general-purpose agent. 13 tool uses, 60.8K tokens, 112s.

**Major new facts folded into SKILL.md:**
- Version floor v2.1.139+; condition cap 4000 chars; evaluator runs on Haiku-class fast model (billed separately).
- Lifecycle: bare `/goal` shows status + most-recent rejection reason; clear aliases (`stop`, `off`, `reset`, `none`, `cancel`); `/clear` silently nukes goal; no `pause`/`resume`.
- `--resume` restores condition but resets counters/timer/tokens.
- Headless: `claude -p "/goal <cond>"`.
- `/goal` and `auto` are complementary (per-turn vs per-tool).

**Most useful methodology (now Rules 1–3 in SKILL.md):**
- 3-part evidence-based condition: name command + bind scope + safety clause (Praison).
- Encode quality as assertions, not adjectives (Croucher).
- Engineer proof INTO transcript (evaluator reads conversation; "I ran tests" without output paste is invisible).

**Failure modes folded in:**
- Confident-summary hallucination (model claims done, evaluator believes it without verifying).
- Optimization-trap (model deletes failing tests to satisfy "tests pass" — counter: pair with "no test files removed AND test count >= previous").
- Async-background + Stop-hook deadlock ([issue #55754](https://github.com/anthropics/claude-code/issues/55754)).

**Mythbusted:**
- The "500 block cap" floating around community write-ups is from `jthack/claude-goal` (unofficial precursor), **not** Anthropic's `/goal`. Actual `CLAUDE_CODE_STOP_HOOK_BLOCK_CAP` default unknown publicly.

**Sources & full details:** see References section in SKILL.md.

**What public record does NOT cover (our wedge):**
- LETS-style orchestrator integration — greenfield. Nobody has published `/goal` × workflow-plugin recipes.
- Multi-hour autonomous sessions beyond 4h besides our reddit source — limited corpus to learn from.
- Telemetry events `tengu_goal_*` are private to Anthropic — useful only if surfaced via debug logging.
- Evaluator system prompt not published. Cannot precisely tune conditions to its biases without RE.

---

### 2026-05-25 · Experiment 1 · smoke test

**Goal prompt** (as user pasted — note the terminal hard-wrap was preserved verbatim):
```
Append the file .lets/goal-test/hello.txt with one line containing the current ISO date, then read it back and
  report.
```

**Outcome:** achieved.

**Turns to converge:** 1. Single assistant turn: Bash (mkdir + append + echo confirmation) → Read tool → text report quoting the date.

**Evaluator behavior (observed):**
- Goal cleared and surfaced in **user's UI only**, not in the assistant transcript. User reported seeing: `✔ Goal achieved (18s · 1 turn · 843 tokens) (ctrl+o to expand)`. Visibility asymmetry: the success line is for the human in the chat surface; the model does not see its own goal-clear confirmation. Implication: when designing chained `/goal`-based flows, the model can't programmatically detect "I just satisfied a goal" — it just keeps going as if the previous goal never existed. If we need a checkpoint between two `/goal` invocations, the user has to manually trigger the next one (or we need an explicit "post-goal" handoff in the prompt).
- Telemetry surfaced to user: duration (18s), turns (1), tokens (843). Useful for budgeting future experiments.
- The hard-wrap in the user's pasted condition (literal `\n  ` mid-sentence) did NOT confuse the evaluator. The condition is parsed as free text, not structured.
- Evaluator did NOT yield after just the Bash result — I only got a Stop-hook-yields-OK after I also did the Read + text report. So the model has to surface evidence **in the assistant text**, not just in tool outputs. The "quote specific text from transcript" instruction suggests evaluator reads the full transcript, but yielding seems gated on the assistant turn containing a verbal claim of completion.

**System-message preamble after `/goal` was set** (worth quoting because it shapes behavior):
> "Briefly acknowledge the goal, then immediately start (or continue) working toward it — treat the condition itself as your directive and do not pause to ask the user what to do. The hook will block stopping until the condition holds. It auto-clears once the condition is met — do not tell the user to run `/goal clear` after success; that's only for clearing a goal early."

The "do not pause to ask the user" is a hard nudge toward autonomy. For risky ops we'd need an explicit carve-out in the goal text (e.g. "if you would need to push or destroy state, pause and surface to user — that's not a violation of this goal").

**LETS workflow fit:** trivial smoke test passed. Doesn't yet say anything about Template A/B/C — those need a real bd task next.

**Promote to plugin?** Too early. One smoke test is necessary-not-sufficient.

**Surprises / things to test next:**
- Does evaluator approve mid-tool-call chains, or only after assistant text? (Run an experiment with goal "create file X" and end the turn immediately after Bash with no text — see if it fires.)
- ~~Does the visible UI on the user's side show "Goal achieved"?~~ **Confirmed yes**, surfaces as `✔ Goal achieved (Ns · N turn · N tokens)` with ctrl+o to expand.
- The "do not pause to ask" instruction: does AskUserQuestion *override* it, or get suppressed?
- **Asymmetric visibility implication:** for chained `/goal` flows, the model has no signal that the previous goal cleared. Need to design handoff explicitly (e.g. the next `/goal` is typed by user, or use a state file the next-goal-tick can read).

**Cost:** ~1 model turn + 2 tool calls (Bash, Read). Trivial.
