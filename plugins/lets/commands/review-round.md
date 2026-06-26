---
description: Work through a RECEIVED round of review comments (inline markers, a review file, or PR threads) on a spec/doc/PR. Triage each item, record decisions to the beads task, keep the artifact FROZEN, then apply all edits in ONE final pass. Use when a reviewer returns N comments to incorporate.
argument-hint: "[path-or-review-source]"
---

# Review Round - Work Through a Received Review

The inverse of `/lets:review` (which GENERATES findings): this CONSUMES a received review and incorporates it.

> **Core discipline:** decisions go into the TASK as you go; the artifact stays **FROZEN**; all file edits happen in **ONE pass at the very end**. Never edit per-decision - cascading reframes change earlier items, and a half-edited spec mid-round is inconsistent.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## When to use
- A reviewer (team lead / Principal / external) returns your spec/doc/PR with many comments (`<!-- REVIEW ... -->`, `// REVIEW`, threaded PR comments, a separate review `.md`).
- A multi-item round where decisions interact / cascade.
- NOT for: a single comment (just fix it); generating a review (`/lets:review`); a single GitHub PR thread (`/lets:github-pr --respond`).

## Usage
```
/lets:review-round <path-or-source>     # annotated copy / review file / PR id
/lets:review-round                      # infer from context (last shared artifact)
```

## Step 0: Locate & index the review
See the whole shape BEFORE deciding anything.
```bash
grep -rn '<!-- REVIEW' <review-dir>/*.md                 # inline markers
for f in <review-dir>/*.md; do printf "%s %s\n" "$f" "$(grep -c '<!-- REVIEW' "$f")"; done
```
Record total + per-file breakdown. Note ALL sources - a round often spans an inline copy AND a PR thread.

## Step 1: Confirm the active task
Use the **detect-task** skill: `Skill(skill: "lets:detect-task")`. Every resolution lands on this task. If none, ask which task owns the artifact.

## Step 2: Cluster into themes + surface REFRAMES first
- Group the N comments into themes (by concern), not file order.
- Flag **reframe** comments - ones that question a PREMISE (topology, mechanism, scope), not a detail. **Decide these FIRST**: they cascade and can moot/alter the nits.
- Present the themed map to the user before triaging.

## Step 3: Verify falsifiable claims against code
A good reviewer makes concrete, checkable claims ("X uses 20 FQCNs", "file Y has Z"). **Verify them yourself** - don't accept "verified: ..." on faith, don't reject without checking. Grep/read the REAL code; record a verdict per claim. Cheapest, highest-signal step.

## Step 4: Triage theme-by-theme WITH the user
Per comment decide exactly one:
- **accept** - valid; note the concrete change for the final pass.
- **reject** - one-line reason (+ the verification that refutes a factual claim).
- **defer** - explicit TRIGGER + where it's recorded (deferral table / fwd-ref).
- **done** - already addressed; cite the location.

2+ viable approaches -> `AskUserQuestion` (header chip 4-12 chars naming the theme, `multiSelect: false`, recommended option first with `(Recommended)` in its label - per AskUserQuestion Conventions in the rules) or `/lets:opinion` for hard ones. The user is the architect; record their call - never auto-accept a premise change.

## Step 5: Record resolutions into the task (artifact stays FROZEN)
Do NOT touch spec/doc/PR files during triage. Append to the task. Templates:

**Decision log (per theme/batch):**
```lets-tracker
comment-add task=<task-id> body="## Decisions {date} - {theme} ({round})
- [accept] {ref} -> {concrete change for final pass}
- [reject] {ref} -> {reason / refuting evidence}
- [defer]  {ref} -> trigger: {x}; recorded in {deferral table}
- [done]   {ref} -> already at {location}"
```
**Verification log:**
```lets-tracker
comment-add task=<task-id> body="## Verified vs code {date}
- {claim} -> VALID/INVALID: {file:line / grep result}"
```
**Resume note (rounds usually span sessions / a compact):**
```lets-tracker
comment-add task=<task-id> body="## RESUME {date} - {round}
- where the review lives (paths + external sources) + index cmd
- PROCESS: decisions->task, artifact FROZEN, batch edits at end
- artifact FROZEN at {sha}; canonical path
- DECIDED so far (don't re-litigate) + verifications
- REMAINING themes + next step"
```

## Step 6: Final edit-pass - ONE batch, at the very end
Only after EVERY comment + reframe is decided:
- If the artifact has a canonical/normative part (a decisions log, a spec's normative register, an API contract), update it FIRST, then cascade dependent prose to match.
- Apply all accepted changes across ALL files in one coherent pass.
- Record deferrals (table + triggers).
- Consistency sweep: grep stale terms, check cross-ref IDs / phase numbers.
- `/lets:commit` - one focused commit, scoped to the artifact path only.

## Step 7: Respond to the reviewer
- Summarize accepted / rejected (reasons) / deferred (triggers) so each comment is traceable.
- **Audience boundary:** in an external/team channel (PR thread, issue tracker, chat) NEVER expose internal beads task IDs or absolute repo paths - reference the artifact by bare filename + in-doc IDs. Match the channel's language for the response; conversation in the user's language.
- For a GitHub PR, hand the per-comment summary to `/lets:github-pr --respond` to post replies inline.
- Re-share the updated artifact if delivered as an attachment.

## Rules
- **Decisions -> task; artifact FROZEN; one edit-pass at the end.** The #1 rule.
- **Verify falsifiable claims against code** before accept/reject.
- **Reframes before nits.**
- **Defer = explicit trigger + recorded location**, never "later" in chat.
- **The user decides**; you triage + recommend.
- Respond in the user's language; task content + written artifacts in English; team responses in the channel's language.

## Output
```
## Review Round: {round}
Source: {paths / PR}   Comments: {N} ({theme breakdown})
Decided: {a accept / r reject / d defer / w done}   Remaining: {...}
Artifact: FROZEN at {sha} (edits batched for final pass)

┌─ LETS ───────────────────────────┐
│  Next theme?  /lets:review-round │
│  Final pass?  /lets:commit       │
└──────────────────────────────────┘
```
