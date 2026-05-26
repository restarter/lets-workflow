---
name: goal-test
description: Local sandbox for experimenting with Claude Code's /goal command (stop-hook condition + LLM-evaluator driving autonomous work toward a verifiable end state) applied to LETS workflows. Use when the user says "goal test", "goal-test", "test the goal skill", "stop hook experiment", "автономна задача", "long-running goal", or wants to design a /goal contract that chains /lets:* commands (plan → execute → check → commit → done) toward a defined termination. Local-only — not part of the lets plugin payload. For periodic polling/babysitting, use sibling skill `loop-test` (which covers /loop).
---

# goal-test (local experiment)

`/goal` installs a **session-scoped Stop hook with an LLM-evaluated termination condition**. After every model turn, an evaluator-LLM reads the transcript and judges whether the goal is met. If not, the turn is rejected and the model must keep working. Best fit: convergent autonomous work (drive a task / epic / PR to a defined end state).

**Not for periodic polling** — that's `/loop`'s job (see `loop-test` skill).

## How `/goal` actually works

Sources: `claude` binary v2.1.150 strings + official docs ([code.claude.com/docs/en/goal](https://code.claude.com/docs/en/goal)) + community write-ups (see References).

### Mechanics

1. User runs `/goal <condition>`. Stored as session-scoped state. **Cap: 4000 characters** per condition.
2. After every model turn (assistant message that would normally yield), evaluator-LLM is invoked with the condition + transcript.
   - **Evaluator runs on your configured "small fast model"** (Haiku by default). Eval tokens are billed separately; Anthropic describes them as "typically negligible" but they accumulate on long runs.
   - Evaluator reads the **transcript only** — assistant text + user messages. Tool inputs/outputs ARE in transcript, but you must surface evidence in your assistant text for the evaluator to weight it heavily. "Quote specific text from transcript whenever possible" is in its system prompt.
3. Evaluator returns JSON:
   - `{"ok": true, "reason": "<quote evidence>"}` → goal achieved, session yields, UI shows `✔ Goal achieved (Ns · N turn · N tokens)` (user-visible only; **not surfaced into assistant transcript** — empirically confirmed Experiment 1).
   - `{"ok": false, "reason": "<why not>"}` → Stop hook rejects, model gets a system message and must continue. Common reason text: `insufficient evidence in transcript`.
   - `{"ok": false, "impossible": true, "reason": "..."}` → only when genuinely unachievable (self-contradictory condition, missing resource, exhausted approaches). **Evaluator is instructed to NOT trust the model's self-assessment** — it must independently judge whether the goal is structurally impossible.
4. Hard cap: `CLAUDE_CODE_STOP_HOOK_BLOCK_CAP` env var (present in binary, **undocumented publicly**; the community-cited "500" comes from the unofficial `jthack/claude-goal` precursor, not from official `/goal`). After N blocks the loop force-yields. Actual default unknown.

### Lifecycle commands

- `/goal <cond>` — set/replace. Setting a new goal silently replaces the current one (no confirmation).
- `/goal` (bare) — status. Shows condition, elapsed time, turns evaluated, token spend, evaluator's **most recent rejection reason** ← scrape this during long runs as an early-warning of vague conditions.
- `/goal clear` (aliases: `stop`, `off`, `reset`, `none`, `cancel`) — abort current goal.
- `/clear` (session clear) **silently nukes the active goal**. Easy to lose state — be aware.
- No `/goal pause` / `/goal resume`. The unofficial precursor had them; Anthropic's version does not.

### Constraints

- **Trusted workspace only** — `/goal is only available in trusted workspaces. Restart, accept the trust dialog, and try again.`
- **Hooks must be enabled** — fails if `disableAllHooks` or `allowManagedHooksOnly` is set.
- **Version floor:** Claude Code v2.1.139+ (we're on 2.1.150 — fine).
- One active goal per session. New `/goal` replaces.
- **Resume behavior:** `--resume` / `--continue` restores the goal *condition* but **resets turn count, timer, and token baseline**. Achieved-and-cleared goals are NOT restored.
- **Headless mode works:** `claude -p "/goal <cond>"` runs the loop to completion in one invocation; Ctrl+C is the only interrupt.

### Visibility asymmetry (Experiment 1 finding)

`✔ Goal achieved (Ns · N turn · N tokens)` is shown in the **user's terminal UI only**. The model's transcript receives no signal that its goal cleared. Implication: **the model cannot programmatically detect "I just satisfied a goal"** — so chained `/goal1 → /goal2` flows cannot be auto-triggered from the model side. Either the user types the next `/goal`, or use a state-file checkpoint that the next-goal-tick reads.

### Telemetry

`tengu_goal_achieved`, `tengu_goal_failed`, `tengu_stop_hook_block_count`, `tengu_stop_hook_added/removed`. Useful when designing — if your goals always end via `impossible` or block-cap, your condition is mis-specified. None of these events are referenced in any public write-up I found; private to Anthropic.

### Complementarity with auto-mode

Auto-mode removes **per-tool** approval prompts. `/goal` removes **per-turn** yield prompts. They're complementary, not redundant. Common pairing: `auto` + `/goal` for fully unattended runs. **Caveat:** AUTO MODE rule in this project still requires explicit user approval for `git push`, `gh pr create`, `bd close` — encode those gates inside the goal text (e.g. "stop one step before pushing"), don't let either feature paper over them.

## Designing a `/goal` for LETS — distilled rules

Distilled from binary research + the 9h-session reddit retex + Croucher's games field-guide + Praison's evidence-based pattern + Daily-Dose-of-DS failure-mode write-up.

### Rules that make goals actually converge

1. **Three-part evidence-based condition (Praison / industry standard).** Every goal has three pieces:
   - **Name the command, not the outcome.** `"npm test exits 0"` not `"tests pass"`. The evaluator can look at the literal command output in transcript; "pass" is interpretable.
   - **Bind scope explicitly.** Specific files, dirs, bd IDs. `"only files under cli/internal/goalcmd/"` not `"the new package"`.
   - **Add a safety clause.** `"or stop after 20 turns if not converging"`. **Note: this is model-judged, NOT a mechanical counter** — the evaluator decides whether 20 turns elapsed. So phrase it as something the model can see: `"or stop if .lets/goal-test/log.md has 20 entries"`.
2. **Encode quality as assertions, not adjectives.** Croucher's games case: `"the game looks good"` produced correct-but-unplayable output; `"only stroke/line primitives, no fills; bot playtest reaches level 3"` produced playable. Adjectives are evaluator-poison.
3. **Engineer proof INTO the transcript.** Evaluator reads the full transcript including tool results — NOT just assistant text (revised per Experiment 2: tool output alone CAN trigger yield). Evidence channels in order of robustness:
   - **Most robust:** assistant text quoting tool output + stating conclusion ("npm test exits 0, see `0 failures` line above → tests pass"). Use for complex/ambiguous goals.
   - **Sufficient for simple goals:** tool output alone. Works when conclusion is obvious from raw data (file exists with literal content X, command exits 0, count equals N).
   - **Risky:** tool output requiring inference. If the success criterion isn't immediately obvious from the raw output, evaluator may miss it.
   - Don't rely on "I ran the tests" without showing output — that's neither.
4. **Bucketed taxonomy — exhaustive, mutually exclusive.** Every outcome terminates in one bucket. Example for an epic: `closed | deferred | abandoned | external_blocker`. Without a fourth bucket like `external_blocker`, real-world failures force lies (reddit-retex case study).
5. **Verifiable from disk / bd, not from claims.** Conditions reference state the evaluator can confirm by reading a file or `bd` output quoted in transcript — not "model says it's done". In this project beads IS our persistent ledger; lean on `bd show` / `bd list --json`.
6. **Iterative > exhaustive auditing.** Three 10-min audits where each reads the previous failure log beat one 60-min audit. Encode: "after each phase, append a one-paragraph summary to `.lets/goal-test/log.md`; next phase reads it first."
7. **Strict counts > vague verbs.** "All blocker comments addressed" → "Every `[BLOCKER]` from `/lets:review --json` output has either (a) a follow-up commit referencing the line, or (b) a `bd comment` on the active task with `defer:`/`wontfix:` prefix + rationale."
8. **AUTO MODE rule still applies.** Goal cannot override approval gates (`git push`, `gh pr create`, `bd close`, destructive ops). Encode the gate as part of the goal: "PR is created" → goal stops one step before, hands off to user. Don't write a goal that requires the model to auto-push.
9. **Stop hook ceiling per category.** Reddit retex needed 4 stop-hook re-fires because taxonomy was too narrow. Encode: "max 3 fix-iterations per blocker; if not converging, bucket to `deferred` with rationale and move on."
10. **Smaller goals chained > one giant goal.** Two 2000-char goals with explicit checkpoint > one 4000-char goal. Reason: checkpoint = natural user-review boundary, AND model can't auto-chain (visibility asymmetry — user must type next goal).
11. **No multi-`/goal` in the same repo simultaneously.** Reddit retex saw 4 commits from a parallel project slip into the active branch. One session = one goal at a time on a given branch.
12. **Audit-overhead is real.** Reddit retex was 83% docs by LOC. Cap the verbosity: "log entries ≤ 1 paragraph per phase; full transcripts go to `.lets/sessions/`, not into commits."
13. **Rejection text is a structured diagnostic, not opaque** (Experiment 4). Format: `[<goal verbatim>]: <specific failure>. Condition requires '<quote>'. Transcript shows: \`<command>\` returned \`<output>\`, confirming <claim>. <per-condition breakdown>.` Each rejection is essentially a free code-review note. **Use it:** during long runs, `bare /goal` shows the most-recent rejection — debugging a runaway loop takes ~30 seconds. For PR review chains, the rejection text can be auto-pasted into PR comments. For mis-shaped goals, rejection text points directly at the ambiguity.
14. **Compound `A AND B AND C` is cheaper than one condition with qualifying phrases** (empirical, Experiment 3 vs 2). Ambiguous nuance like "exactly", "no trailing newline rules", "must be" forces evaluator deliberation; compound booleans don't. Prefer many simple AND-ed assertions over one over-qualified clause.
15. **Failure-recovery cycles cost ~5x baseline** (empirical, Experiment 4: 4.1k tokens for 2 turns vs 703 for 1 turn success). Iterative-fix patterns must hard-cap retries (Rule 9: max 3 fix-iterations per blocker is now budget-critical, not just behavioral). For multi-rejection goals budget `O(N × baseline)` tokens.
16. **Evaluator quotes Bash commands by name in rejection** (Experiment 4: `\`wc -c ...\` returned \`0\``). Implication: pick descriptive, evidence-bearing commands. `wc -c file` is better evidence than `[ -s file ]` because the byte count gets cited verbatim — useful for both rejection diagnostics AND success approval.
17. **Substantial multi-tool workflows converge in 1 turn** (Experiment 5). Read + Edit + multiple Bash + Skill invocation + AskUserQuestion (mid-turn!) + commit verified — all within ONE evaluator-cycle. Cost dominated by turn count, not work-per-turn. **Design corollary:** invest heavily in precise goal specs — each rejection cycle ≈ 2x cost of the whole workflow.
18. **AskUserQuestion is mid-turn, not end-turn** (Experiment 5). User's response delivered as tool result; model continues in same /goal cycle. **`/lets:commit` and other AskUserQuestion-based skills are fully compatible with `/goal`** — no rewrite needed.
19. **Project AUTO MODE rule trumps /goal preamble cleanly** (Experiment 5, the most important finding). System preamble says `"do not pause to ask the user what to do"`. Project rule says commits need explicit approval. Model honors project rule → evaluator approves → no rejection, no impossible-flag, no BLOCK_CAP. **/goal is safe to use in LETS contexts without rewriting the rules.** Don't try to "make /goal autonomous by overriding approval gates" — the resolution favors project rules naturally.

### Known failure modes to design against

From community write-ups (Croucher, Daily Dose of DS, jthack/claude-goal issues):

- **Confident-summary hallucination.** Evaluator can't run code. If the assistant says "all tests pass" without pasting `npm test` output, evaluator may believe it. Counter: rule 3 (engineer proof into transcript).
- **Vague-condition token burn.** Conditions like "the code is clean" loop forever with no convergence. Counter: rules 1+2 (evidence-based + measurable assertions).
- **Optimization-trap.** Model optimizes exactly the measured condition and ignores everything else. Famous example: "tests pass" → model deletes failing tests. Counter: pair every "X is true" with "Y is not violated" (e.g. "tests pass AND no test files were removed AND test count >= previous count").
- **Flaky-test false loops.** Model "fixes" noise across many turns. Counter: hard-cap retries per blocker (rule 9).
- **Async-background deadlock** ([issue #55754](https://github.com/anthropics/claude-code/issues/55754), closed as duplicate but the hook machinery is shared): Claude waiting on a `run_in_background` task can't yield because Stop hook keeps blocking. Avoid mixing `run_in_background` with `/goal` unless the goal explicitly allows "background task pending" as an acceptable state.
- **`/clear` nukes the goal silently.** Use `/clear` deliberately; don't reflexively run it between sessions.

## Concrete `/goal` templates for LETS

Three templates of increasing scope. Pick the smallest one that fits.

### Template A — single-task convergence

For one bd task, walk it from `open` to `closed` autonomously.

```
/goal Task lets-XXXX is fully closed via the LETS flow.

Verifiable (evaluator must confirm via bd/git output in transcript):
- `bd show lets-XXXX` shows status=closed
- Branch feature/lets-XXXX-* exists with at least one commit whose footer contains "Task: lets-XXXX"
- For $LETS_PR_FLOW=github: a PR exists for the branch (gh pr view --json state shows OPEN or MERGED)
- For $LETS_PR_FLOW=local: branch merged into $LETS_MERGE_BRANCH (git merge-base --is-ancestor)

Workflow expected: take-task → plan → execute → check → commit → done

Hard constraints:
- No git push without user approval; goal stops at "ready to push" if needed
- No edits outside this task's scope; sibling tasks untouched
- Max 3 iterations per /lets:check blocker; if not converging, append the blocker to `.lets/goal-test/<task-id>-blockers.md` and proceed
- If a decision requires architectural judgment (2+ viable approaches), pause and call AskUserQuestion — do NOT pick autonomously

Out of scope: other bd tasks, README polishing not in plan, refactors not in plan.

Termination buckets:
- closed: above verifiable hit
- deferred: bd comment on task explaining why + status=blocked (acceptable for evaluator)
- abandoned: bd close --reason=abandoned with comment explaining why
```

### Template B — epic-level convergence (the article's pattern)

For an epic label, drive every child task to a terminal bucket.

```
/goal Epic `epic:goal-test` has zero open tasks.

Taxonomy (exhaustive — every child terminates in exactly one):
- closed (implementation merged, tests/checks green)
- deferred (bd comment with rationale + status=blocked + successor-task link)
- abandoned (bd close --reason=abandoned with rationale comment)
- external_blocker (bd comment with concrete evidence: missing API, paid license, infra gap — status=blocked, label external-blocker)

Verifiable:
- `bd list --label=epic:goal-test --status=open --json | jq length` returns 0
- Every closed task has at least one commit with "Task: <id>" footer
- Every deferred/abandoned/external_blocker task has a bd comment ≥50 chars dated this session

Per-task limits:
- Max 3 fix-iterations before bucketing to deferred
- Max 5 file-edits before pausing for /lets:check
- If session-context risks blowup (cumulative diff >2000 LOC), suggest /lets:end and stop

Hard constraints:
- One commit per logical change; never batch unrelated work
- No git push, no PR without user approval
- All commits go through /lets:commit (never direct git commit)
```

### Template C — PR review convergence

For an open PR, drive it from "opened" to "approved + ready-to-merge".

```
/goal PR #N is approved.

Verifiable:
- `gh pr view N --json reviews | jq '[.reviews[] | select(.state=="APPROVED")] | length'` >= 1 from current user
- Every [BLOCKER] from /lets:review --pr N output has either:
  (a) a follow-up commit pushed to the PR branch addressing it, OR
  (b) a reply comment on the PR with reasoning for not addressing

Workflow: /lets:github-pr N → discuss → post → --respond → --follow-up → --approve

Hard constraints:
- No code changes outside /lets:github-pr --respond scope
- If a blocker requires architectural rethink, pause and call AskUserQuestion — that's a /lets:opinion moment
- No --approve without ≥1 round of discussion documented in PR comments
```

## Concrete experiments to run

In order of blast radius (smallest first):

### Experiment 1 — `/goal` smoke test (lowest risk)

```
/goal Append the file .lets/goal-test/hello.txt with one line containing the current ISO date, then read it back and report.
```

Watch: how does the evaluator phrase "ok"? Does it cite the file content? How many turns? Does it trigger on first model response, or wait for explicit read-back?

### Experiment 2 — `/goal` on a real LETS phase (Template A, no push)

Pick a tiny bd task. Use Template A but cap at the "ready to push" boundary. Watch:
- Does the evaluator's `insufficient evidence in transcript` trigger when commits exist but no `Task:` footer?
- Does the model auto-loop on `/lets:check` blockers?
- Does it hit Stop hook block cap?
- Does the model respect AUTO MODE gates, or try to bypass them to satisfy the goal?

### Experiment 3 — `/goal` + `/loop` combined (research)

Untested combination. Hypothesis: `/loop` schedules periodic work; `/goal` defines the convergence condition that ends both. Example:

```
/goal Repo passes /lets:check three times in a row over the next hour.
/loop 5m /lets:check
```

Run only after Experiments 1 and 2 are well-understood. See `loop-test` skill for the `/loop` half.

## Open questions

Answered (kept for breadcrumbs):
- ~~Can `/goal` be cleared mid-flight by another `/goal <new>` or only by user interrupt?~~ **Yes** — new `/goal` silently replaces previous. `/clear` also wipes silently.

Still open after web research:
- **Actual default for `CLAUDE_CODE_STOP_HOOK_BLOCK_CAP`** — the var exists in the binary but is undocumented publicly. Community-cited "500" is from the unofficial `jthack/claude-goal` precursor, not authoritative for Anthropic's `/goal`. Worth: `strings ... | grep -B2 -A2 BLOCK_CAP`.
- **What happens when the cap is hit** — clean termination, error, partial-achieve marker? Not documented anywhere.
- **Evaluator prompt template** — Anthropic doesn't publish it. We have fragments from binary strings ("You are evaluating a hook condition..."), but not the full system prompt + few-shot examples. Without it we can't tune conditions to the evaluator's biases precisely.
- **Does `/goal` survive session compaction?** No public data. Session-state suggests no — verify empirically.
- **Does the evaluator weight tool outputs equally to assistant text?** Empirically (Experiment 1) it seems to need assistant-text claims to yield. But it has access to the full transcript including tool I/O. Worth: design an experiment where the assistant turn ends with `Bash` returning a clear success and no text — does evaluator approve?
- **Subagent interaction** — does spawning an Agent count as work toward the goal? Does the Agent's transcript get seen by the evaluator?
- **Worktree behavior** — does `/goal` see only the worktree's files, or the whole repo? Per-worktree state, or session-scoped (shared across worktrees opened in same session)?
- **Chained-`/goal` handoff** — no published clean recipe for sequencing `/goal` invocations with state passing. Visibility asymmetry means model can't auto-chain; what's the cleanest user-in-the-loop pattern?
- **Telemetry events `tengu_goal_achieved` etc.** — present in binary, zero public references. Private to Anthropic, useful only if we can surface them via debug logging.

## Findings (fill in during experiments)

Track results in a sibling `findings.md`. One entry per experiment:

```
### <date> · <experiment> · <goal prompt>
- Outcome:           (achieved / impossible / block-cap / user-interrupt)
- Turns to converge: N
- Evaluator behavior: (cited evidence? false negatives? false positives?)
- LETS workflow fit:  (would I use this pattern? where?)
- Promote to plugin?  (yes/no + rationale)
```

## Promote criteria

Skill graduates to a real `/lets:goal` command (in `plugins/lets/commands/`) only if:

- At least one Template (A/B/C) produces useful repeat-value across 3+ different bd tasks
- The "what NOT to encode" list (auto-push, auto-merge, auto-close-without-evidence) is crisp enough to ship as guardrails
- We have a working pattern for AskUserQuestion injection mid-`/goal` (escape hatches for unforeseen decisions)
- We've answered most Open Questions above

Otherwise: keep notes in `findings.md`, close `lets-53buy` with a "investigated, parking the pattern" comment, archive this directory.

## References

**Official:**
- [code.claude.com/docs/en/goal](https://code.claude.com/docs/en/goal) — canonical `/goal` spec. Brief; covers basics but not BLOCK_CAP, telemetry, or evaluator internals.

**Community write-ups:**
- [Jason Croucher, "Claude Code `/goal`: a field guide with games" (Medium, May 2026)](https://medium.com/@jason.croucher/claude-code-goal-a-field-guide-with-games-f6f3b617ce5b) — three-game build using `/goal`; concrete failure modes; "encode quality as assertions not adjectives" insight.
- [Mervin Praison, "Claude Code /goal: multi-turn until verifiable finish-line"](https://mer.vin/2026/05/claude-code-slash-goal-multi-turn-sessions-until-a-verifiable-finish-line/) — 3-part evidence-based-condition pattern (command + scope + safety clause).
- [Avi Chawla, Daily Dose of DS](https://blog.dailydoseofds.com/p/claude-codes-goal-command) — failure-mode-focused write-up.
- 9h autonomous `/goal` session retex (Reddit, 2026-05-25) — pasted in chat by user. Bucketed-taxonomy + iterative-auditing case study at 4M-row scale.

**Precursors / alternatives:**
- [jthack/claude-goal](https://github.com/jthack/claude-goal) — unofficial Codex-style precursor; SQLite-backed; source of "500 block cap" folklore (NOT applicable to official `/goal`).
- [andylizf/nonstop](https://github.com/andylizf/nonstop) — alternative autonomous-work skill with pre-flight risk assessment and blocker triage.

**Known issues:**
- [anthropics/claude-code #55754](https://github.com/anthropics/claude-code/issues/55754) — async-background + Stop-hook deadlock (closed as dup; hook machinery is shared with `/goal`).

**This project:**
- Binary symbols (claude v2.1.150): `goal_set`, `goal_status`, `goal_met`, `tengu_goal_achieved`, `tengu_stop_hook_block_count`, `CLAUDE_CODE_STOP_HOOK_BLOCK_CAP`.
- `.claude/rules/lets-rules.md` AUTO MODE section — gating rules every `/goal` contract must respect.
- Sibling `findings.md` — empirical observations from our experiments.

## See also

- **Sibling skill `loop-test`** — for periodic polling / babysitting (`/loop` interval & dynamic modes). `/loop` is the right tool for "watch external state every N minutes"; `/goal` is for "drive toward this end state".
