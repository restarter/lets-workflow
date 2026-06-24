---
description: Structured planning - explore codebase, design architecture, evaluate options, produce detailed implementation plan
argument-hint: "[feature description] [--fast]"
---

# Plan

Turn a task or idea into a detailed implementation plan. Clarifies scope, explores codebase, discusses approaches with user, designs architecture for selected approaches, evaluates with experts, then writes a bite-sized plan.

**HARD-GATE: This command produces a plan, NOT code. No files are modified except .lets/plans/.**

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## --fast mode

`/lets:plan --fast` (combinable with a task-id or feature description, e.g. `/lets:plan lets-abc --fast`) skips the three subagent-dispatch phases - **Step 4** (explorer agents), **Step 6** (architect agents), **Step 7** (expert agents) - and replaces them with orchestrator-only equivalents (read files yourself, draft approaches inline, self-evaluate risks). Use it when the user explicitly wants a collaborative talk-through without subagent budget: "let's just plan it together, I'll review". Everything else - clarifying questions, interactive discussion, plan format, beads recording, the saved file - is identical to the full flow. The plan's shape doesn't change; only *how* it's built.

When `--fast` is **not** set, run the full flow exactly as written.

## Step 1: Capture the Goal

**Parse the argument:** strip a `--fast` token if present (sets fast mode); the rest is the feature goal.

**If a feature goal was provided:** use it.

**If no goal provided:** ask:

> "What are you trying to build or change?"

Wait for answer before proceeding.

## Step 2: Active Task Context

Use the **detect-task** skill to find the active task: `Skill(skill: "lets:detect-task")`.

If Step 1 captured an explicit `<task-id>` argument (e.g. an interactive `--flow plan` launch into a fresh worktree), resolve-and-claim it per the **detect-task** *explicit task-id argument* convention - pointer only, don't re-paraphrase.

If task found:
```bash
bd show <task-id>
```

Load: title, description, design notes, existing comments.

If no task found, warn:
> "No active task detected. Every session needs a task. Create one with `bd create` or pick from `bd ready`."

Do not block - continue if user acknowledges.

## Step 3: Clarifying Questions

**Ask questions BEFORE exploration.** Scope must be clear before launching expensive agents.

Based on (goal + task description), identify unknowns that would change the architecture.

Present ALL questions at once - max 5, min 2.

Categories to draw from:
- **Scope**: What's explicitly OUT of scope?
- **Behavior**: Edge cases, error handling, user-facing behavior
- **Constraints**: Performance, compatibility, must-not-break areas
- **Success criteria**: How will we know it's done?
- **Unknowns**: Technical unknowns that need spiking first

```
Before exploring the codebase, I need to clarify scope:

1. {question}
2. {question}
...

Answer all at once.
```

Wait for user answers before proceeding.

## Step 4: Codebase Exploration

Gather project context:

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
cat "$LETS_PROJECT_ROOT/CLAUDE.md" 2>/dev/null | head -200
```

### --fast: orchestrator-only exploration

**If `--fast` is set:** skip the explorer-dispatch phase below (Exploration Strategy, Show Exploration Plan, Launch Explorers). Instead, build the Codebase Map yourself:

- Read/Grep/Glob the files relevant to the feature goal + user clarifications, on demand - prioritize the entry points, the modules the feature touches, and the existing patterns it should follow.
- Iterate: read a file, learn something, decide what to read next. Stop when you can describe the relevant surface confidently.
- Synthesize the same `## Codebase Map` structure (sections by area), then go to the **Checkpoint: Exploration Review** below (same checkpoint, same options).
- Note any area you couldn't cover ("didn't read X - low confidence there") so the user can ask for more.

Then jump to **Checkpoint: Exploration Review**.

### Exploration Strategy

*(full mode only - skipped under `--fast`)*

Decide how many explorers to launch and what each should focus on.

**Inputs for decision:**

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
FILE_COUNT=$(find "$LETS_PROJECT_ROOT" -type f \
  -not -path '*/.git/*' \
  -not -path '*/node_modules/*' \
  -not -path '*/vendor/*' \
  -not -path '*/dist/*' \
  -not -path '*/.next/*' \
  -not -path '*/build/*' \
  -not -path '*/__pycache__/*' \
  | wc -l | tr -d ' ')
```

Based on the **feature goal**, **user clarifications**, **project size**, and **task description**, identify distinct focus areas that need exploration. Each focus area becomes one explorer agent.

**Guidelines for choosing focus areas:**
- Each explorer should have a clear, non-overlapping focus
- Small isolated feature -> 1-2 explorers (e.g., "map existing auth code", "map test patterns")
- Cross-cutting feature -> 3-5 explorers (e.g., one per module/layer it touches)
- Large-scale refactor or unclear scope -> 5-10 explorers (one per area of concern)
- Subagents have a separate rate limit - prefer more exploration when uncertain

**Confirmation gate:** If planning to launch more than 10 explorers:

```
AskUserQuestion(
  questions=[{
    question: "Planning {N} explorers for this feature. That's a lot - confirm?",
    header: "Confirm",
    options: [
      { label: "Launch all", description: "{N} explorers, thorough coverage" },
      { label: "Reduce", description: "Suggest fewer, more focused explorers" }
    ],
    multiSelect: false
  }]
)
```

### Show Exploration Plan

Before launching, display the plan:

```
## Codebase Exploration

Project: {FILE_COUNT} source files
Feature: {feature goal, abbreviated}
Explorers: {N}

1. {focus area 1} - {what this explorer will map}
2. {focus area 2} - {what this explorer will map}
...

> Subagents have a separate rate limit - no cost to your conversation.

Launching...
```

### Launch Explorers

**CRITICAL: Launch ALL explorer agents in a SINGLE message with multiple Task tool calls.**

For each focus area, launch one explorer:

```
Task(
  subagent_type="lets:explorer",
  prompt="{FOCUS AREA} EXPLORATION. {One-line description of what to find.}

FEATURE GOAL: {feature goal from Step 1}
USER CLARIFICATIONS: {answers from Step 3}

TASK CONTEXT:
{task title and description from beads, if available}

YOUR FOCUS:
{specific bullets tailored to this focus area}

Return the structured exploration report as defined in your system prompt.
Focus ONLY on {focus area} - other explorers cover other areas."
)
```

**If any explorer fails or returns no useful data:**
- Note the gap explicitly in the Codebase Map ("No data for {area} - explorer failed")
- Ask user: "Explorer {N} couldn't map {area}. Continue with partial data, or should I explore {area} manually?"

### Synthesize Codebase Map

After all explorers return, synthesize a combined codebase map with one section per explorer:

```
## Codebase Map

{For each explorer that returned results:}
### {Focus Area} (from Explorer {N})
{key findings synthesized from this explorer's report}

{End for}
```

### Checkpoint: Exploration Review

Show the codebase map to user, then ask:

```
AskUserQuestion(
  questions=[{
    question: "Codebase map ready. How to proceed?",
    header: "Codebase",
    options: [
      { label: "Continue", description: "Looks good, let's discuss approaches" },
      { label: "Questions", description: "I have questions about the findings" },
      { label: "Re-explore", description: "Missing context, explore a specific area" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Continue** -> proceed to Step 5
- **Questions** -> answer user's questions about findings, then ask checkpoint again
- **Re-explore** -> ask what area to explore, launch targeted explorer, update codebase map, ask checkpoint again
- **Other** (free text) -> treat as question or correction, address it, ask checkpoint again

## Step 5: Approach Discussion

Based on (codebase map + goal + user clarifications), propose concrete implementation approaches.

### Formulate Approaches

Analyze exploration findings and propose 2-4 approaches. Each approach must be:
- **Concrete** - derived from what explorers found in THIS codebase, not abstract labels
- **Distinct** - each takes a meaningfully different path (not just "do more" / "do less")
- **Named** - short descriptive name based on the key differentiator

**Bad approach names:** "Minimal", "Maximal", "Pragmatic" (generic, not project-specific)
**Good approach names:** "Extend existing parser", "New module with shared helpers", "Plugin architecture"

### Present to User

```
## Possible Approaches

Based on exploration, I see {N} approaches:

### A: {name}
{2-3 sentences: what it does, key trade-off}

### B: {name}
{2-3 sentences: what it does, key trade-off}

### C: {name} (if applicable)
{2-3 sentences: what it does, key trade-off}
```

Then ask which to develop into full architecture designs:

```
AskUserQuestion(
  questions=[{
    question: "Which approaches should I develop into full architecture designs?",
    header: "Approach",
    options: [
      { label: "A: {name}", description: "{1-line summary}" },
      { label: "B: {name}", description: "{1-line summary}" },
      { label: "C: {name}", description: "{1-line summary}" (if applicable) },
      { label: "Discuss first", description: "I have questions or want to adjust" }
    ],
    multiSelect: true
  }]
)
```

**Handle response:**
- **1+ approaches selected** -> proceed to Step 6 with selected approaches
- **Discuss first** -> open discussion, user asks questions or suggests modifications, then present updated approaches and ask again
- **Other** (free text) -> treat as new approach idea or modification, incorporate and re-present

## Step 6: Architecture Design

### --fast: orchestrator-only design

**If `--fast` is set:** skip the architect dispatch (Architect Brief, parallel launch). Instead, draft the architecture yourself for each approach the user selected in Step 5 - using the codebase map, the user's clarifications, and your own analysis. Produce the same `## {Approach Name}` shape (Summary / Components / Files / Data Flow / Trade-offs) for each. Present the design(s) to the user, then go to **Checkpoint: Architecture Review** below (same checkpoint, same options).

Then jump to **Checkpoint: Architecture Review** (use the multi-approach or single-approach variant as applicable).

### Architect Brief

*(full mode only - skipped under `--fast`)*

Launch one architect agent per selected approach. Each gets a focused brief with user's decisions baked in.

For each selected approach:

```
Task(
  subagent_type="lets:architect",
  prompt="ultrathink

DESIGN MODE: {approach name}.

Design a detailed architecture for this specific approach.

PROJECT CONTEXT:
{CLAUDE.md summary}

FEATURE GOAL: {goal}
USER CLARIFICATIONS: {answers from Step 3}
APPROACH TO DEVELOP: {approach description from Step 5, including any user input from discussion}

CODEBASE MAP:
{combined exploration output from Step 4}

INSTRUCTIONS:
- Develop THIS specific approach into a full architecture
- Be specific: exact files, exact functions, exact interfaces
- Follow existing codebase patterns found by explorers
- Include user's decisions and preferences from the discussion

OUTPUT:
## {Approach Name}

### Summary
{2-3 sentences - what and why}

### Components
{each component: file path, responsibility, key interface}

### Files
- Create: {list with purpose}
- Modify: {list with what changes}

### Data Flow
{entry -> processing -> output}

### Trade-offs
- Pro: {advantages}
- Con: {limitations}
- Risk: {what could go wrong}"
)
```

**CRITICAL: Launch ALL architect agents in a SINGLE message with multiple Task tool calls.**

**If any architect fails:** Note which approach failed. If 2+ remain, continue. If only 1 remains, present it and ask user if they want to re-run the failed one or proceed.

### Checkpoint: Architecture Review

After all architects return, present results.

**If multiple approaches were developed:**

```
## Architecture Designs

### {Approach A name}
{summary}
- Files: {N} create, {M} modify
- Key trade-off: {main pro vs con}

### {Approach B name}
{summary}
- Files: {N} create, {M} modify
- Key trade-off: {main pro vs con}
```

```
AskUserQuestion(
  questions=[{
    question: "Which architecture to proceed with?",
    header: "Architecture",
    options: [
      { label: "{Approach A name}", description: "{1-line summary}" },
      { label: "{Approach B name}", description: "{1-line summary}" },
      { label: "Combine", description: "Mix ideas from multiple approaches" },
      { label: "Adjust", description: "Change requirements and re-design" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Approach selected** -> proceed to Step 7 with chosen architecture
- **Combine** -> discuss which parts to take from each, create merged brief, re-run single architect
- **Adjust** -> discuss what to change, loop back to relevant step
- **Other** (free text) -> treat as modification request, adapt

**If only 1 approach was developed:**

Show the architecture design and ask:

```
AskUserQuestion(
  questions=[{
    question: "Architecture design ready. Proceed?",
    header: "Design",
    options: [
      { label: "Continue", description: "Looks good, proceed to expert evaluation" },
      { label: "Adjust", description: "I want to change something first" }
    ],
    multiSelect: false
  }]
)
```

## Step 7: Expert Evaluation

### --fast / Self-evaluation: orchestrator-only evaluation

**If `--fast` is set, OR the user picks "Self-evaluation" at the Expert Selection checkpoint below:** do an orchestrator-only evaluation - no expert subagents. (`--fast` skips the Suggest Experts / Expert Selection / Dispatch steps entirely; the gate option lands here as the chosen answer.) Self-evaluate the chosen architecture from project context: name the real risks, the proportionality concerns (overengineering / underspec), and the trade-offs. Present them in the same `## Expert Evaluation` shape (a short list of findings + a Risks & Suggestions list), then add:

> *This evaluation skipped the expert agents. For a second opinion before executing, run `/lets:opinion` or `/lets:review --plan` after the plan is saved.*

Then go to the **Checkpoint: Evaluation Results** below (same checkpoint, same options).

### Suggest Experts

*(full mode only - skipped under `--fast`)*

Evaluate the chosen architecture with domain experts.

Based on what the feature touches, suggest relevant experts:

| Feature touches... | Suggest expert |
|--------------------|---------------|
| Auth/security | `lets:security` |
| Database/schema | `lets:database` |
| API endpoints | `lets:backend` |
| UI/components | `lets:frontend` |
| Docker/CI/deploy | `lets:devops` |
| Tests/coverage | `lets:qa` |

**Always suggest:** `lets:pragmatist`
**Never suggest:** `lets:architect` (designed the option - can't evaluate own work)
**On request:** `lets:actor` (external personality - user must provide source. Use **actor-fetch-personality** skill)

### Checkpoint: Expert Selection

```
AskUserQuestion(
  questions=[{
    question: "Which experts should evaluate the architecture?",
    header: "Experts",
    options: [
      { label: "Full panel (Recommended)", description: "Pragmatist + N domain experts based on feature. Separate rate limit." },
      { label: "Pragmatist only", description: "Quick evaluation, just overengineering check" },
      { label: "Self-evaluation", description: "No agents - orchestrator critiques its own architecture (risks, overengineering, trade-offs)" },
      { label: "Skip evaluation", description: "Architecture is solid, go straight to plan" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Full panel** -> dispatch recommended experts
- **Pragmatist only** -> dispatch only pragmatist
- **Self-evaluation** -> no subagents: run the orchestrator-only evaluation from "### --fast / Self-evaluation: orchestrator-only evaluation" above (same `## Expert Evaluation` shape + the second-opinion note), then go to **Checkpoint: Evaluation Results**. Unlike **Skip**, this still produces an evaluation - it just spends no agent budget.
- **Skip evaluation** -> proceed directly to Step 9 (Plan Generation)
- **Other** (free text) -> parse expert names from text, dispatch selected

### Dispatch Experts

**CRITICAL: Launch ALL selected agents in a SINGLE message.**

For each expert:

```
Task(
  subagent_type="lets:{agent-name}",
  prompt="ultrathink

MODE: plan

Evaluate an architecture design for a feature.

FEATURE GOAL: {goal}

CHOSEN ARCHITECTURE:
{full architect output for chosen approach}

{If actor agent: include PERSONALITY block from actor-fetch-personality skill}
PERSONALITY:
{fetched personality content - only for lets:actor, omit for other agents}"
)
```

### Checkpoint: Evaluation Results

After all experts respond, present findings:

```
## Expert Evaluation

**Pragmatist:** {verdict} - {key point}
**{Expert 1}:** {verdict} - {key point}
**{Expert 2}:** {verdict} - {key point}

### Risks & Suggestions
{consolidated list of actionable items from all experts}
```

Then:

```
AskUserQuestion(
  questions=[{
    question: "How to proceed?",
    header: "Findings",
    options: [
      { label: "Discuss", description: "Explore trade-offs, challenge assumptions, probe deeper" },
      { label: "Generate plan", description: "Architecture approved, write the implementation plan" },
      { label: "Adjust architecture", description: "Incorporate expert feedback first" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Discuss** -> proceed to Step 8 (Interactive Exploration)
- **Generate plan** -> skip to Step 9 (Plan Generation)
- **Adjust architecture** -> discuss changes, update architecture, ask again
- **Other** (free text) -> treat as adjustment request

## Step 8: Interactive Exploration

Deep-dive discussion to stress-test the architecture before committing to a plan. Ask probing questions, share insights, challenge assumptions.

**This step is opt-in** - user chooses "Discuss" after evaluation. For simple tasks users skip straight to plan generation.

### How It Works

This is a **conversation, not a checklist**. One question or insight at a time, building on user's responses.

### Start with Insights

Open with 2-3 insights derived from exploration + evaluation. Format:

```
## Let's explore this deeper

**Insight:** {non-obvious implication from the architecture or expert feedback}

**Insight:** {connection the user might not have considered - "X already almost exists because Y"}

**Question:** {probing question that challenges an assumption or surfaces a trade-off}
```

### Conversation Loop

After each user response:

1. **Acknowledge** - brief, no fluff ("Good point." / "That changes things.")
2. **Build** - add an insight or connection based on their answer
3. **Probe** - ask the next question that goes deeper or shifts angle

### Question Types to Draw From

- **Assumption challenges:** "You're assuming X - what if Y instead?"
- **Edge case probes:** "What happens when Z?"
- **Trade-off surfacing:** "This gives you A but costs B - is that worth it?"
- **Scope questions:** "Do you actually need X, or is Y enough for now?"
- **Integration risks:** "How does this interact with {existing thing from exploration}?"
- **Future-proofing:** "If requirements change to include X, does this design bend or break?"
- **User perspective:** "From the end user's view, what changes?"

### What Makes Good Insights

- Connect dots between exploration findings and user's goals
- Surface things that "almost exist" in the codebase
- Flag where the architecture diverges from existing patterns (and whether that's intentional)
- Note when expert feedback contradicts each other
- Highlight decisions that are easy to change now but expensive later

### Exit Condition

After each exchange, gauge whether to continue or wrap up. Signals to wrap up:
- User gives short answers (topic exhausted)
- Key questions have been covered
- User explicitly says "enough" / "let's move on"

When ready to wrap up:

```
AskUserQuestion(
  questions=[{
    question: "Ready to generate the plan?",
    header: "Plan ready",
    options: [
      { label: "Generate plan", description: "Discussion complete, write the implementation plan" },
      { label: "Keep exploring", description: "More to discuss" },
      { label: "Adjust architecture", description: "Change the approach based on what we discussed" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Generate plan** -> proceed to Step 9
- **Keep exploring** -> continue conversation loop
- **Adjust architecture** -> go back to Step 6 with updated context from discussion
- **Other** (free text) -> treat as continuation of discussion

### Record Key Findings

After exploration ends, if active task exists:

```bash
# tracker: comment-add binding (beads); non-beads resolves via the adapter (lets-rules "Tracker Adapters")
bd comments add <task-id> "Exploration insights:
- {key insight 1}
- {key insight 2}
- {decision or trade-off confirmed}"
```

## Step 9: Plan Generation

Write a detailed implementation plan for the chosen architecture.

### Plan Format

```markdown
# {Feature Name} - Implementation Plan

**Task:** {task-id}
**Date:** {YYYY-MM-DD}
**Goal:** {one sentence}
**Approach:** {chosen option name and summary}

---

## Context

### Codebase Map
{condensed from Step 4 - only parts relevant to chosen approach}

### Key Decisions
- {decision 1}: {rationale}
- {decision 2}: {rationale}

### Non-Goals (out of scope)
- {thing 1}
- {thing 2}

---

## Implementation

### Task 1: {Component/Step Name}

**Files:**
- Create: `exact/path/to/file.ext` - {purpose}
- Modify: `exact/path/to/existing.ext` - {what changes}

**Step 1.1: {action}**

{complete code snippet - no placeholders, no "add here" comments}

**Step 1.2: {action}**

...

**Verify:**
Run: `{verification command}`
Expected: {what output looks like}

**Commit:**
git add {specific files}
git commit -m "{type}: {message}"

---

### Task 2: {Next Component}

...

---

## Verification

When all tasks complete:
- [ ] {check 1 with specific command}
- [ ] {check 2 with specific command}

## Success Criteria
- [ ] {measurable outcome 1}
- [ ] {measurable outcome 2}
```

### Plan Quality Gates

Before saving, verify the plan passes these gates:
- Every task is 15-60 minutes of work
- Every code snippet is complete (no `// TODO` or `// implement here`)
- Every task that touches existing code names exact files and functions
- Every task that creates new code shows the interface (signatures, types)
- Every task has a verification step with expected output
- Every logical unit has a commit point
- File paths are exact and verified against explorer findings

## Step 10: Save & Output

### Save Plan

Derive plan filename from the current branch:

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current)
if [ "$BRANCH" = "{LETS_MERGE_BRANCH}" ]; then
  # Trunk-mode: branch name has no task scope, derive slug from task-id (from detect-task in Step 2)
  SLUG="{TASK_ID}"
else
  SLUG="${BRANCH#feature/}"   # e.g., 0nf.10-improve-statusline
fi
STAMP=$(date +%Y-%m-%d-%H%M)   # same convention as .lets/sessions/ - keeps plan history, no overwrite
mkdir -p "$LETS_PROJECT_ROOT/.lets/plans"
PLAN_FILE="$LETS_PROJECT_ROOT/.lets/plans/${STAMP}-${SLUG}.md"
echo "$PLAN_FILE"   # capture the exact dated path - this is where you Write the plan
```

Write plan to: `$PLAN_FILE` (i.e. `.lets/plans/${STAMP}-${SLUG}.md`)

Example: branch `feature/0nf.10-improve-statusline` -> `.lets/plans/2026-06-06-1846-0nf.10-improve-statusline.md`
Trunk-mode example: branch `main`, task `lets-abc` -> `.lets/plans/2026-06-06-1846-lets-abc.md`

### Record in Beads

If active task found:
```bash
# tracker: comment-add binding (beads); non-beads resolves via the adapter (lets-rules "Tracker Adapters")
bd comments add <task-id> "## Plan: {feature name}

Approach: {chosen option name}
Tasks: {N} implementation tasks
Key files: {top 3-5 files}
Plan: .lets/plans/${STAMP}-${SLUG}.md"
```

### Show Output

```
## Plan Ready: **{task title}** (`{task-id}`)

Saved: `.lets/plans/${STAMP}-${SLUG}.md`
Built: {full flow (explorer + architect + expert agents) | fast mode (orchestrator-only - no subagents)}

### Approach
{chosen option - 2 sentences}

### Tasks
1. {task 1 name}
2. {task 2 name}
...

### Key Decisions
- {decision 1}
- {decision 2}

Start a new session to execute the plan with clean context.
```

```
┌─ LETS ─────────────────────────────┐
│  Check plan?   /lets:check --plan  │
│  Review plan?  /lets:review --plan │
│  Execute?      /lets:execute       │
│  New session?  /lets:start         │
└────────────────────────────────────┘
```

(In fast mode, nudge `/lets:check --plan` or `/lets:review --plan` first - the plan didn't get agent review.)

## Rules

- **NEVER write code** outside the plan document in `.lets/plans/`
- **NEVER skip clarifying questions** (Step 3) - vague input produces vague plans, in fast mode too
- **EVERY phase transition requires user approval** via AskUserQuestion
- **NEVER hardcode approach names** like "Minimal/Maximal/Pragmatic" - derive from exploration context
- **ALL parallel agents in a SINGLE message** - never sequential when parallel is possible
- **`--fast` skips Steps 4/6/7 subagent dispatch only** - it does NOT skip the clarifying questions, the checkpoints, the discussion, or any user approval gate. Output plan format is identical; record which mode was used in the Plan Ready output.
- **Exact file paths** in plan - verified against explorer findings (or orchestrator's own reads in fast mode)
- **Complete code snippets** - no stubs, no "implement X here"
- **Plan is the artifact** - session ends when plan is saved
- Respond in user's language
