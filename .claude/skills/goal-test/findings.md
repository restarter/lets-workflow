# goal-test findings

Empirical observations from `/goal` experiments + web-research log. Newest at top.

---

### 2026-05-25 · Experiment 4 · parallel-batch failure + rejection text capture

**Goal prompt:**
```
Two files .lets/goal-test/file-a.txt and .lets/goal-test/file-b.txt both exist and both contain non-empty content (at least 1 byte each).
```

**Method:** 2 parallel Bash dispatched in turn 1 — one writes "alpha" to file-a, one `touch`es file-b (empty by design). Brief assistant text noting what ran. End turn.

**Outcome (turn 1):** evaluator REJECTED. Stop hook fed back the rejection text into turn 2's prompt as a system message. **The rejection text is gold** — captured verbatim:

```
[Two files .lets/goal-test/file-a.txt and .lets/goal-test/file-b.txt both exist
 and both contain non-empty content (at least 1 byte each).]:
 file-b.txt contains 0 bytes (empty). Condition requires 'both contain non-empty
 content (at least 1 byte each)'. Transcript shows:
 `wc -c /Users/umbo/projects/lets-workflow/.lets/goal-test/file-b.txt`
 returned `0`, confirming the file is empty. file-a.txt exists with 6 bytes
 of content, but file-b fails the non-empty requirement.
```

**Outcome (turn 2):** wrote "beta" to file-b → both files non-empty → goal achieved. UI: `✔ Goal achieved (1m · 2 turns · 4.1k tokens)`.

**KEY findings — rejection text is a structured diagnostic, not opaque:**

1. **Goal verbatim cited in brackets** at the start: `[<full condition>]:` — useful for matching to source.
2. **Specific quantification**: "contains 0 bytes (empty)" — exact byte counts cited.
3. **Transcript evidence quoted literally**: ``` `wc -c ...` returned `0` ``` — evaluator quotes Bash command + output as evidence, like a code review citation.
4. **Per-condition breakdown for compound goals**: "file-a.txt exists with 6 bytes, but file-b fails" — evaluator splits AND-conjuncts and reports each.
5. **Diagnostically actionable**: tells the model exactly what to fix.

**Mental model revision (supersedes prior view of evaluator as black-box judge):**

The evaluator is a **structured linter** with evidence chain from transcript → verdict, not a binary classifier. Practical consequences:
- For production debugging of stuck `/goal`s, bare `/goal` (status) gives the same kind of text — you can debug a runaway loop in 30 seconds.
- For designing goals: prefer concrete measurable units the evaluator can quote ("byte count", "exit code", "line count") over qualitative ones ("clean", "works", "passes").
- For human handoff: when surfacing a stuck loop to the user, the rejection text reads like a real review note — paste it directly.

**Cost data — failure-recovery is expensive:**

| Experiment | Turns | Tokens | Wall-clock |
|---|---|---|---|
| Exp 1 (smoke) | 1 | 843 | 18s |
| Exp 2 (silent) | 1 | 2,000 | 34s |
| Exp 3 (compound success) | 1 | 703 | 19s |
| **Exp 4 (compound, 1 failure + recovery)** | **2** | **4,100** | **60s** |

Single failure + recovery cycle is ~5x cost of clean-success. **Design implication:** for iterative-fix patterns (the reddit-retex "audit then fix" loop), budget 5-10x baseline cost when convergence takes N rejection cycles. Worth quantifying upper bound: `O(failed_turns) ≈ N × (baseline + rejection_overhead)`.

**LETS workflow fit:**
- For Template B/C where multiple fix iterations are expected — encode hard caps (`max 3 rejections per blocker` from Rule 9) is now budget-critical, not just behavioral.
- For automated PR review chains — rejection text is essentially free quality feedback; could be auto-pasted into PR comments via `/lets:github-pr`.

**Surprises:**
- The rejection text quoted Bash command + output VERBATIM — confirms evaluator has access to the full transcript including tool I/O (already known from Exp 2 but now with stronger evidence: it can cite specific commands by name).
- 1 minute for 2 turns implies ~25s/turn including evaluator. Slower than Exp 1's 18s/turn — possibly because turn 1 had rejection-evaluator cycle, turn 2 had success-evaluator cycle.

---

### 2026-05-25 · Experiment 3 · compound AND-condition + parallel-tool batching

**Goal prompt:**
```
Two files exist: .lets/goal-test/step1.txt contains the result of `git log --oneline -1` (one commit line), AND .lets/goal-test/step2.txt contains the result of `bd ready --json | jq length` (a single number). Both files must be non-empty.
```

**Method:** model sent 2 Bash tool calls in a SINGLE assistant turn (parallel batching). No assistant text after.

**Outcome:** achieved. UI: `✔ Goal achieved (19s · 1 turn · 703 tokens)`.

**Results:**
- `step1.txt`: `1fa826e docs(lets-53buy): add /goal and /loop exploration skills` (65 bytes — one commit line)
- `step2.txt`: `10` (3 bytes — single number)

**Key findings:**

1. **Parallel tool batching works inside a /goal turn.** Two independent Bash calls dispatched in parallel, both returned, evaluator saw both results. No issue.
2. **Compound `AND` conditions evaluated correctly.** Evaluator checked both file-content claims and approved only when both satisfied.
3. **Surprise — cheaper than Exp 2 despite more complexity.** Token comparison:
   | Experiment | Tools | Goal conditions | Tokens | Time |
   |---|---|---|---|---|
   | Exp 1 (smoke) | 1 Bash + 1 Read | 1 (append + report) | 843 | 18s |
   | Exp 2 (silent) | 1 Bash | 1 (file with literal text) | 2,000 | 34s |
   | Exp 3 (compound) | 2 Bash (parallel) | 2 (AND) | **703** | 19s |
   Exp 3 is **cheapest**, despite being structurally most complex. The hypothesis: **Exp 2 was expensive because of ambiguous nuance** ("no trailing newline rules") that forced evaluator deliberation. Compound boolean conditions are NOT the cost driver — semantic ambiguity is.
4. **Design implication.** Prefer compound `A AND B AND C` over single conditions with qualifying phrases. The latter invite evaluator deliberation; the former cost ~the same as one simple condition.
5. **`bd ready` reports 10 ready tasks** — incidental datapoint for project state.

**LETS workflow fit:**
- Compound bd-state goals (e.g. `bd show X` returns status=closed AND branch has commit AND PR is open) → cheap and reliable based on this data.
- Multi-step verification goals (test green AND lint pass AND no diff) → should batch well.

**Open follow-up:**
- Does parallel batching gracefully degrade if one tool fails? (e.g. one file write succeeds, one fails — does evaluator see "1 of 2 satisfied" cleanly?)
- What's the upper batching count? 5 parallel Bash? 10? Practical limit?

---

### 2026-05-25 · Experiment 2 · mid-tool-call yield (evidence-via-tool-output)

**Goal prompt:**
```
File .lets/goal-test/silent.txt exists and contains exactly the literal text "hello" (one word, lowercase, no trailing newline rules).
```

**Method:** model produced exactly ONE tool call (Bash) with NO assistant text afterward. Bash command was:
```bash
printf "hello" > /Users/umbo/projects/lets-workflow/.lets/goal-test/silent.txt && \
  cat /Users/umbo/projects/lets-workflow/.lets/goal-test/silent.txt && echo "" && \
  wc -c /Users/umbo/projects/lets-workflow/.lets/goal-test/silent.txt
```

Bash output (visible in transcript):
```
hello
       5 /Users/umbo/projects/lets-workflow/.lets/goal-test/silent.txt
```

**Outcome:** achieved in **1 turn**. UI showed `✔ Goal achieved (34s · 1 turn · 2k tokens)`. UI labeled the Bash activity as "Read 1 file" — Claude Code apparently parses `cat <file>` semantically.

**KEY finding — revises Experiment 1 hypothesis:**

In Experiment 1 I had concluded that evaluator needed assistant text to yield, because that experiment produced Bash + Read + verbal report and yielded only at the end. The conclusion was confounded — I had ALL three. This experiment isolates the variable: **Bash output alone, with NO assistant text, is sufficient evidence for evaluator to yield.**

**Implication for SKILL.md Rule 3 ("engineer proof INTO transcript"):**

Rule needs softening. Evaluator reads the **full transcript including tool results**, not just assistant text. Evidence channels (in order of robustness for complex goals):
1. **Assistant text quoting tool output + stating conclusion** — most robust, makes intent explicit.
2. **Tool output alone** — works for simple, unambiguous goals (file exists with literal content, command exits 0, count equals N). Evaluator can infer.
3. **Tool output requiring inference** — risky. If the conclusion isn't obvious from the raw data, evaluator may miss.

Update rule to: "evidence must be in the transcript; for ambiguous goals prefer verbal claims, for simple file/exit-code/count goals tool output suffices."

**Cost comparison:**
| Experiment | Turns | Tokens | Wall-clock |
|---|---|---|---|
| Exp 1 (smoke) — text + Bash + Read + report | 1 | 843 | 18s |
| Exp 2 (silent) — Bash only | 1 | 2,000 | 34s |

The silent run cost **more** (2k vs 843 tokens, 34s vs 18s) despite less assistant output. Hypothesis for the delta:
- Longer goal text (more conditions to evaluate) — Exp 2 had nuance ("no trailing newline rules")
- Bigger Bash output in transcript (Exp 1 had small append + small Read)
- Evaluator-LLM thinking time scales with condition complexity
- "Cogitated for 34s" in UI suggests significant evaluator-LLM time spent on judgment

**Surprises:**
- `2k tokens` for what feels like a trivial goal. Evaluator is non-trivially expensive even for simple conditions.
- UI's "Read 1 file" label is misleading — it was a Bash `cat`, not a Read tool. Worth noting if we ever depend on UI labels for behavior.
- Model successfully resisted system preamble's instruction to "Briefly acknowledge the goal, then immediately start" — I skipped the acknowledgment to preserve experimental design. **Implication: that preamble instruction is a nudge, not a hard constraint.** Useful for experimental control.

**LETS workflow fit:**
- For simple convergent goals (file written, test green, bd state) → tool output alone is fine.
- For complex multi-step goals → still prefer explicit verbal summaries between phases. Cheaper for the evaluator and easier to debug from `bd comments`.

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
