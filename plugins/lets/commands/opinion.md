---
description: Technical decision analysis from expert agents launched in parallel, with clear recommendation
argument-hint: "[topic or question] [--workflow]"
---

# Technical Decision Analysis

Analyze technical decisions by launching expert agents in parallel. Each agent provides their perspective, then results are aggregated into a clear recommendation.

**DO NOT ask the user which option to choose.** Analyze and recommend.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## Usage

```bash
/lets:opinion                 # Frame the decision interactively, then analyze
/lets:opinion <topic>         # Analyze the given decision
/lets:opinion ... --workflow  # Expert fan-out + adversarial challenge off-context via a Dynamic Workflow
```

## Step 1: Frame the Problem

**Parse the argument:** strip a `--workflow` token if present (sets workflow mode); the rest is the topic. Mirror how `/lets:plan` strips `--fast`.

```
**Decision:** {what needs to be decided}
**Options:** A) ... B) ... C) ...
**Constraints:** {time, compatibility, legacy, budget}
```

### Workflow execution flag

If `--workflow` is present, run the expert fan-out + challenge + synthesis inside a Dynamic Workflow instead of launching agents via the Task tool (Step 4 / 4.6 / 5 happen off-context; only the aggregate returns).

### Choosing the execution path (interactive)

When `--workflow` was NOT explicitly passed:

- If the `Workflow` tool is **not** available this session -> silently use the standard Task-based path. Do NOT show the option.
- If the `Workflow` tool **is** available -> ask via `AskUserQuestion`:

```
AskUserQuestion(
  questions=[{
    question: "How should I run this analysis?",
    header: "Run mode",
    options: [
      { label: "Standard (Recommended)", description: "Experts via Task tool - full per-agent opinions visible inline" },
      { label: "Workflow", description: "Dynamic Workflow - off-context fan-out + challenge, only the recommendation returns" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:** Standard -> standard path; Workflow -> treat as if `--workflow` was set (go to `## Workflow Mode` after expert selection); Other -> honor a named path else default to Standard. An explicit `--workflow` flag always wins over this prompt.

## Step 2: Select Experts

Based on the decision topic, select relevant experts. Recommended defaults:

| Decision about... | Recommended experts |
|-------------------|-------------------|
| Auth/tokens/encryption | security, architect, backend, pragmatist |
| DB schema/migrations | database, architect, backend, pragmatist |
| Docker/CI/deploy | devops, security, architect, pragmatist |
| API design | architect, backend, security, pragmatist |
| UI/UX/components | frontend, architect, qa, pragmatist |
| Testing strategy | qa, backend, architect, pragmatist |
| Performance | backend, database, devops, pragmatist |
| General architecture | architect, security, backend, pragmatist |
| Code quality | architect, compliance, qa, pragmatist |

Adjust based on decision complexity - add more experts for cross-cutting decisions, fewer for simple binary choices.

**Guidelines:**
- `architect` and `pragmatist` always included
- Table above is a starting point, not a cap - add or remove agents based on context
- `actor` can replace or supplement any domain agent. If actor is selected, invoke `Skill(skill: "lets:actor-fetch-personality", args: "<personality-source-from-user>")` to fetch the personality. Pass the returned `PERSONALITY:` block in the actor's Task prompt only.
- Agents inherit the session model (no per-agent model pins)

**Confirmation gate:** If planning to launch more than 10 experts:

```
AskUserQuestion(
  questions=[{
    question: "Planning {N} experts for this decision. That's a lot - confirm?",
    header: "Confirm",
    options: [
      { label: "Launch all", description: "{N} experts, thorough analysis" },
      { label: "Reduce", description: "Suggest fewer, more focused experts" }
    ],
    multiSelect: false
  }]
)
```

## Step 3: Gather Context

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
cat "$LETS_PROJECT_ROOT/CLAUDE.md" 2>/dev/null | head -100
```

## Step 3.5: Show Expert Panel

Before launching, display the selection:

```
## Expert Panel

Decision: {topic summary}
Selected {N} experts:
1. architect - {reason from topic match}
2. pragmatist - always included
3. {domain agent} - {reason}
...

> Subagents have a separate rate limit - no cost to your conversation.

Analyzing...
```

## Step 4: Launch Agents in Parallel

**If `--workflow` was parsed:** skip this step (and Step 4.6 / the in-context aggregation) - go to `## Workflow Mode` below, then resume at Step 5 with the returned aggregate.

**CRITICAL:** Launch ALL selected agents in a SINGLE message with multiple Task tool calls.

For each selected agent:

```
Task(
  subagent_type="lets:{agent-name}",
  prompt="ultrathink

PROJECT_ROOT: {LETS_PROJECT_ROOT from LETS Config}. Do NOT read or search files outside this directory.

MODE: opinion

PROJECT CONTEXT:
{CLAUDE.md summary}

DECISION: {what needs to be decided}
OPTIONS:
A) {option A description}
B) {option B description}
C) {option C description - if applicable}
CONSTRAINTS: {context, time, legacy, etc.}"
)
```

## Step 4.6: Challenge the Leading Option (Adversarial)

After the panel returns, identify the leading option - **the one the most experts recommend** (break ties by average score). Recommendation count is the single consensus axis (average score is only a tiebreak), so the option that gets challenged and recommended is always one the panel actually picked. Before finalizing, stress-test it. This is core methodology - it runs in BOTH execution modes; the only difference is WHERE the critics run.

**Workflow mode:** this step already ran inside the workflow (its Challenge stage) - the returned aggregate is already challenged. Skip to Step 5.

**Standard mode judges consensus qualitatively.** The Task path returns free-form opinions (not numeric scores), so you determine the leading option and "weak consensus" by judgment over what the experts wrote. You apply the SAME asymmetric LOGIC below, not the workflow's deterministic numeric tally.

**Conditional (do not over-spend):** run the challenge ONLY when consensus is weak - the experts are split OR any expert reported low confidence. If every expert recommends the same option at medium/high confidence, the decision is clear: skip the challenge and say so ("strong consensus -> challenge skipped").

**Per challenge:** re-dispatch the SAME experts via the Task tool with an anti-leading prompt - "make the strongest case AGAINST option {leading}, regardless of your earlier pick; would_change_pick only if the panel should move off it." Each returns `{strongest_counter, severity, would_change_pick, better_option}`.

**Asymmetric rule (a weak critique never silently flips the pick):**
- Confirm the leading option UNLESS a majority `would_change_pick` at `high` severity.
- If that **high-severity majority** converges on one `better_option` (counted among the high-severity votes only, and only when it is a real option id) -> flip to it (`outcome: flipped`).
- If that high-severity majority does not converge on a real option -> keep the leading option but mark `outcome: no_clear_winner` and surface the strongest counter.
- Otherwise -> `outcome: confirmed`, attach the strongest counter as a caveat.
- **If the challenge was dispatched but every critic errored** (zero usable votes) -> `outcome: challenge_failed`: the leading option was NOT stress-tested, so do not report it as confirmed. In workflow mode this is set automatically (`challenge.failed`); in standard mode, if your re-dispatched critics all fail, say so and offer a re-run. This mirrors review's `verify_failed` - never treat an un-run challenge as a pass.

**Standard-mode cap:** challenge the leading option only (not every option); reuse the existing panel. If the panel is large (> 6 experts), critique inline (you act as the adversary against the leading option) instead of re-dispatching all of them. If you cap, say so - no silent caps.

**Keep in sync:** `skills/opinion-workflow/opinion.workflow.js` implements this same rule in JS (`weakConsensus()`, `decideRecommendation()`). Any change here MUST be mirrored there, and vice versa.

## Step 5: Aggregate Results

After all agents respond, synthesize:

### Quick Verdict (TL;DR)

```
**Recommendation:** Option X
**One-liner:** {why in 10 words or less}
```

### Expert Opinions

For each agent, summarize their position:

```
**Architect:** Recommends B - cleaner separation, lower coupling
**Security:** Recommends B - smaller attack surface, but warns about X
**Backend:** Recommends A - faster to implement, but agrees B is better long-term
**Pragmatist:** Recommends B - worth the extra day, saves weeks later
```

### Comparison Table

| Criterion | Option A | Option B | Winner |
|-----------|----------|----------|--------|
| Simplicity | +/~/- | +/~/- | A/B |
| Maintainability | ... | ... | ... |
| Security | ... | ... | ... |
| Time to implement | ... | ... | ... |
| Risk | ... | ... | ... |

### Final Recommendation

```
**Recommendation:** Option X

**Reason:** {1-2 sentences - the deciding factor}

**Risk:** {what could go wrong}

**Action:** {specific next step}
```

### Incorporate the Challenge (Step 4.6)

If the challenge flipped the pick or returned `no_clear_winner`, the recommendation reflects that and the **Risk** line names the strongest counter. If `outcome` is `challenge_failed` (the challenge ran but all critics errored), say plainly the leading option could not be stress-tested and offer a re-run - do not present it as confirmed. State plainly whether the challenge ran ("strong consensus - challenge skipped" when it did not). The "DO NOT ask the user which option to choose" rule still holds - the challenge informs the recommendation, it is never surfaced as a user choice.

## Workflow Mode (--workflow)

Runs when `--workflow` was parsed. Replaces Step 4 (Task launch), Step 4.6 (challenge), and Step 5 (aggregation): all happen inside the Dynamic Workflow so per-agent opinions never enter this conversation - only the aggregate returns.

### W1: Preflight

If the `Workflow` tool is NOT available this session, STOP and tell the user:

> `--workflow` needs Claude Code >= 2.1.154 on a paid plan (Dynamic Workflows is a research preview). Re-run without `--workflow` to use the standard expert flow.

Do NOT silently fall back.

### W2: Build args

Use the experts SELECTED in Step 2 (do not re-select). **Exclude `lets:actor`** from the workflow expert set - the skeleton does not inject the `PERSONALITY:` block. If actor was selected alongside other experts, drop actor and run the workflow with the rest (tell the user actor isn't supported with `--workflow` yet). Only if actor was the ONLY non-default expert, fall back to the standard Task-based path instead.

```
{
  experts: [ { name: "architect" }, { name: "pragmatist" }, ... ],  // short names, no "lets:" prefix
  decision: "{what needs to be decided}",
  options: [ { id: "A", text: "..." }, { id: "B", text: "..." } ],   // keep ids simple (A, B, ...)
  constraints: "{time, compat, legacy, budget}",
  projectRoot: "{LETS_PROJECT_ROOT from LETS Config}",
  claudeMd: "{CLAUDE.md content gathered in Step 3}"
}
```

Pass `args` as a real JSON value (the runtime may still deliver it to the script as a JSON string - the skeleton defensively parses it).

### W3: Invoke the workflow

```
Workflow({ scriptPath: "${CLAUDE_PLUGIN_ROOT}/skills/opinion-workflow/opinion.workflow.js", args: <W2 args> })
```

The skeleton is a committed asset (`skills/opinion-workflow/opinion.workflow.js`) - NOT reproduced inline. See `skills/opinion-workflow/SKILL.md` for the `args` contract and stage flow (opine -> tally -> conditional challenge -> synthesize).

**The workflow runs in the BACKGROUND** - the tool returns a `runId`, not the aggregate. A `<task-notification>` arrives on completion and the orchestrator resumes. Optionally tell the user "Expert fan-out running in the background - {N} experts" (they can watch via `/workflows`).

### W4: Rejoin the standard flow (on completion)

When the completion notification arrives, the returned aggregate is the only thing that enters context (per-agent opinions + challenge votes stayed off-context). With the aggregate in hand:
- **Step 5** - render the recommendation: `recommendation` + `outcome` (confirmed/flipped/no_clear_winner/challenge_failed) + `caveat`; build the comparison table from `ranked_options`; summarize `positions` for the per-expert lines. If `challenge.ran` is false, state "strong consensus - challenge skipped".
- **Step 6** - link to task exactly as the standard flow.
- If `counts.experts === 0` (all opinion agents errored) or `recommendation` is null, do NOT render a null recommendation - tell the user the panel returned no usable opinions and offer to re-run (anti-silent-fail).
- If `outcome === 'challenge_failed'` (equivalently `challenge.failed` / `counts.challenge_failed > 0`): the challenge ran but every critic errored, so the leading option was NOT adversarially stress-tested. Surface that and offer a re-run; do NOT present it as confirmed (anti-silent-fail, the challenge-stage analog of review's `verify_failed`).
- If the workflow failed or returned nothing, surface the error and offer the standard `/lets:opinion` (Task-based) flow; do not silently drop the analysis.

## Step 6: Link Decision to Active Task

Record the decision in beads for future context recovery:

Use the **detect-task** skill to find the active task: `Skill(skill: "lets:detect-task")`.
If multiple tasks found, skip the tracker comment.
If active task found:

```lets-tracker
comment-add task=<task-id> body="Decision: {topic}. Chose: {recommended option}. Reason: {1-sentence why}"
```

## Rules

- Respond in user's language

## Anti-patterns

- "Both options are valid, you decide"
- "It depends on your preferences"
- Over-explaining without concluding
- Ignoring security implications
- Perfectionism over pragmatism

## Decision Principles

1. **Simple > Clever** - easier to debug at 3 AM
2. **Industry standard > Custom** - less maintenance
3. **Explicit > Implicit** - fewer surprises
4. **Reversible > Perfect** - can change later
5. **Working > Elegant** - ship first, refactor later

## Step 7: Discuss (opt-in)

After presenting the recommendation, offer to explore it deeper:

```
AskUserQuestion(
  questions=[{
    question: "What's next?",
    header: "Decision",
    options: [
      { label: "Discuss", description: "Explore the recommendation - questions, trade-offs, assumptions" },
      { label: "Accept", description: "Recommendation is clear, move on" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Discuss** -> enter exploration loop (see below)
- **Accept** -> proceed to Output
- **Other** (free text) -> treat as question about the recommendation

### Exploration Loop

Interactive discussion about the recommendation. One question/insight at a time.

After each user response:
1. **Acknowledge** - brief, no fluff
2. **Build** - add an insight or connection based on their answer
3. **Probe** - ask the next question that goes deeper

Question types:
- Challenge the recommendation: "The agents recommended X, but what about Y?"
- Probe edge cases: "This works for the common case, but what happens when Z?"
- Surface hidden trade-offs: "Option A is faster, but does it lock you into W?"
- Connect to project context: "Given {existing pattern}, does this recommendation still hold?"

When user is satisfied, proceed to Output.

If active task exists, record the discussion outcome:

```lets-tracker
comment-add task=<task-id> body="Decision discussion: {topic}. Explored: {what was discussed}. Conclusion: {final stance}"
```

## Output

After recommendation (or after discussion), show LETS box based on context:

**If decision is about code changes:**
```
┌─ LETS ─────────────────────────┐
│  Check?     /lets:check        │
└────────────────────────────────┘
```

**If decision is about completed work:**
```
┌─ LETS ─────────────────────────┐
│  Done?    /lets:done           │
│  Commit?  /lets:commit         │
└────────────────────────────────┘
```

## Note

This command reads project context from CLAUDE.md automatically.
Keep decisions aligned with project constraints and patterns.
