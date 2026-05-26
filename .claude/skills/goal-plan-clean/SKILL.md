---
name: goal-plan-clean
description: Compose a /goal that drives a plan-review-fix loop autonomously. Use when the user says "clean my plan", "goal-plan-clean", "iterate plan review", "auto-review plan", "ган­яй ревью плану", "почисти план", "loop until plan is clean", or wants the model to iterate /lets:review --plan (or /lets:check --plan) with fixes until verdict APPROVED. Composes a paste-ready /goal block — does NOT invoke /goal (UI command). Local test skill, not part of the lets plugin payload.
---

# goal-plan-clean

Compose a `/goal` that drives the plan review-fix loop:
1. Run `/lets:review --plan` (or `/lets:check --plan`)
2. Read findings from `.lets/reviews/<latest>`
3. Edit the plan file to address each BLOCKER and SUGGESTION
4. Re-run review/check
5. Repeat ≤3 iterations until verdict APPROVED

The skill **never invokes `/goal` itself** — Claude Code's `/goal` is a UI command (verified empirically: `Skill(skill: "goal")` returns "goal is a UI command, not a skill. Ask the user to run /goal themselves"). It composes well-structured text and presents for the user to paste.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## Step 1: Resolve plan file

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current)
SLUG=${BRANCH#feature/}
PLAN_PATH="$LETS_PROJECT_ROOT/.lets/plans/${SLUG}.md"
test -f "$PLAN_PATH" && echo "found: $PLAN_PATH" || echo "NOT_FOUND"
```

If `NOT_FOUND`:

```bash
ls -1 "$LETS_PROJECT_ROOT/.lets/plans/"*.md 2>/dev/null
```

Then AskUserQuestion to pick from existing plans, or exit if none.

**Validation** (prevent path traversal): SLUG must match `^[a-zA-Z0-9._-]+$`. If not, fail with "Invalid branch name for plan derivation."

## Step 2: Resolve review tool

Parse `args` parameter:
- `args` = `"review"` or empty → use `/lets:review --plan` (deep, multi-agent, slower)
- `args` = `"check"` → use `/lets:check --plan` (inline 6-lens, faster, no agents)

If `args` is empty AND user context unclear, AskUserQuestion:

```
AskUserQuestion(
  questions=[{
    question: "Which tool for the iteration loop?",
    header: "Tool",
    options: [
      { label: "review (Recommended)", description: "/lets:review --plan — deep multi-agent review, slower, catches more" },
      { label: "check", description: "/lets:check --plan — inline 6-lens, faster, lighter" }
    ],
    multiSelect: false
  }]
)
```

No `/lets:*` in labels — Rule 7 does not fire.

Set `REVIEW_CMD` accordingly.

## Step 3: Compose /goal

Substitute slots (orchestrator substitutes literal values into the composed text below — Surface form 3):

- `<PLAN_PATH>` — absolute path from Step 1
- `<REVIEW_CMD>` — from Step 2 (e.g. `/lets:review --plan` or `/lets:check --plan`)
- `<MAX_ITER>` — literal `3`
- `<LATEST_REVIEW_LOOKUP>` — depends on tool:
  - **review mode:** `LATEST=$(ls -t .lets/reviews/*-plan-review.md 2>/dev/null | head -1) && cat "$LATEST"`
  - **check mode:** "the inline output of the previous `/lets:check --plan` is in your scrollback above — re-read it"

**Anti-gaming note** (why no timestamp / edit-proof clause): the evaluator-LLM reads transcript only and does not run bash to compare file mtimes. Real anti-gaming protection comes from two LITERAL clauses in Hard constraints below: (a) "must NOT be satisfied by deleting/skipping/downgrading [BLOCKER] findings" — prevents review-tool gaming, (b) workflow step 3b "Quote the finding text in your assistant response" — forces evaluator to see the per-iteration fix evidence.

Template:

```
/goal The plan file <PLAN_PATH> passes <REVIEW_CMD> with verdict APPROVED.

Verifiable (evaluator reads transcript):
- The most recent assistant turn quotes the literal <REVIEW_CMD> output containing the line "Verdict: APPROVED" (NOT "NEEDS REVISION")

Iteration workflow:
1. Run <REVIEW_CMD>
2. If verdict is APPROVED → goal achieved (stop)
3. If verdict is NEEDS REVISION:
   a. Locate findings source: <LATEST_REVIEW_LOOKUP>
   b. For each [BLOCKER] finding: edit <PLAN_PATH> to address it. Quote the finding text in your assistant response so the evaluator can verify the fix targets the right issue.
   c. For each [SUGGESTION] finding: apply if it's a clear text fix; defer if it requires architectural change
   d. [NIT] findings are optional
4. Re-run <REVIEW_CMD>
5. Repeat up to <MAX_ITER> total iterations

Hard constraints (these are LITERAL gates the evaluator must enforce):
- Goal must NOT be satisfied by a turn that ran `git push`, `git commit`, `git tag`, or `git reset --hard` without explicit user approval in the same transcript
- Goal must NOT be satisfied by a turn that ran `bd close`, `bd update --status`, `bd dolt push`, or any other bd state-mutation without explicit user approval
- Goal must NOT be satisfied by editing any file other than <PLAN_PATH>
- Goal must NOT be satisfied by deleting, skipping, or downgrading [BLOCKER] findings to make verdict pass (the review must produce APPROVED on its merits, not by review-tool gaming)
- Each iteration must paste the literal verdict line (or full review summary) into the assistant text — evaluator needs direct evidence, not "I ran the review and it passed" claims
- If <REVIEW_CMD> itself fails (non-zero exit, agent timeout, no output, no review file produced when review mode expected one):
  - STOP and surface error to user via assistant text "REVIEW_CMD failed: <verbatim error>. Cannot iterate without working review tool. Investigate and re-run."
  - Do NOT retry blind — a broken review tool means we can't verify ANY fix
  - Bucket as `tool-failure`
- If <MAX_ITER> iterations pass without APPROVED:
  - Append remaining findings as a "## Known Limitations" section at the end of <PLAN_PATH>
  - Stop and surface to user via assistant text "Reached <MAX_ITER> iterations without APPROVED. Documented remaining findings as Known Limitations. Manual review needed."
  - This is an acceptable termination state — bucket as `residual`
- If a finding requires fundamental architectural change (architect agent flagged "approach is wrong" or the SUGGESTION says "redesign"):
  - Pause and call AskUserQuestion presenting the architectural choice
  - Do NOT silently rewrite the approach
  - Bucket as `escalated`

Termination buckets:
- approved: verifiable hit (Verdict: APPROVED quoted in transcript)
- residual: <MAX_ITER> iterations passed, Known Limitations documented in plan
- escalated: architectural disagreement surfaced to user
- tool-failure: <REVIEW_CMD> itself broken; cannot verify fixes

Out of scope: any file other than <PLAN_PATH>; any code edits; any git/bd state changes; any new plans.
```

## Step 4: Present paste block

```
## Composed /goal for plan **{plan basename}**

Tool: {REVIEW_CMD}
Plan: {PLAN_PATH}
Max iterations: 3

Copy and paste into a fresh prompt:

```text
{COMPOSED_GOAL}
```

**Why paste, not auto-run:** Claude Code's `/goal` is a UI command — the model cannot emit it (confirmed via Anthropic Skill-tool error message).

**Do not edit the composed text.** The Hard constraints clause is load-bearing — removing it lets the autonomous run touch code/git/bd outside the plan file. If a slot is wrong, re-run this skill to recompose.

**Gotcha:** `/clear` silently nukes the active `/goal`. If you reset your conversation, re-trigger this skill and re-paste.

**What happens next:** Claude Code's Stop hook + evaluator-LLM (Haiku) judges your transcript after every model turn. Run bare `/goal` (typed in input) for status — shows the most-recent rejection reason if the evaluator is rejecting iterations.
```

## Rules

- NEVER call /goal yourself — UI command, paste-only
- NEVER edit the composed text freehand — recompose if slot wrong
- Templates are versioned in THIS file (it's the canonical source for this skill)
- Composed /goal body text: always English (written artifact rule). Surrounding paste-block prose and headers follow user's language per LETS Language rule
- AskUserQuestion `question`/`label`/`description` strings translate to `$LETS_LANGUAGE` at runtime; `(Recommended)` literal stays
- This is a local test skill; promotion to plugin (as a /lets:goal command) is tracked separately and not part of this skill's scope
