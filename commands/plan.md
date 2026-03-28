---
description: Structured planning - explore codebase, design architecture, evaluate options, produce detailed implementation plan
argument-hint: "[feature description]"
---

# Plan

Turn a task or idea into a detailed implementation plan. Clarifies scope, explores codebase, discusses approaches with user, designs architecture for selected approaches, evaluates with experts, then writes a bite-sized plan.

**HARD-GATE: This command produces a plan, NOT code. No files are modified except .lets/plans/.**

## Step 1: Capture the Goal

**If argument provided:** use it as the feature goal.

**If no argument:** ask:

> "What are you trying to build or change?"

Wait for answer before proceeding.

## Step 2: Active Task Context

Use the **detect-task** skill to find the active task (read `skills/detect-task/SKILL.md` and follow its detection flow).

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
# ROOT = project-root from LETS Config
cat "$ROOT/CLAUDE.md" 2>/dev/null | head -200
```

### Exploration Strategy

Decide how many explorers to launch and what each should focus on.

**Inputs for decision:**

```bash
FILE_COUNT=$(find "$ROOT" -type f \
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
    header: "LETS",
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
    header: "LETS",
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
    header: "LETS",
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

Launch one architect agent per selected approach. Each gets a focused brief with user's decisions baked in.

### Architect Brief

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
    header: "LETS",
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
    header: "LETS",
    options: [
      { label: "Continue", description: "Looks good, proceed to expert evaluation" },
      { label: "Adjust", description: "I want to change something first" }
    ],
    multiSelect: false
  }]
)
```

## Step 7: Expert Evaluation

Evaluate the chosen architecture with domain experts.

### Suggest Experts

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
    header: "LETS",
    options: [
      { label: "Recommended", description: "{pragmatist + N domain experts based on feature}. Separate rate limit." },
      { label: "Pragmatist only", description: "Quick evaluation, just overengineering check" },
      { label: "Skip evaluation", description: "Architecture is solid, go straight to plan" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Recommended** -> dispatch recommended experts
- **Pragmatist only** -> dispatch only pragmatist
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
    header: "LETS",
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
    header: "LETS",
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
BRANCH=$(git branch --show-current)
SLUG="${BRANCH#feature/}"   # e.g., 0nf.10-improve-brainstorm
# ROOT = project-root from LETS Config
mkdir -p "$ROOT/.lets/plans"
```

Write plan to: `.lets/plans/{branch-slug}.md`

Example: branch `feature/0nf.10-improve-brainstorm` -> `.lets/plans/0nf.10-improve-brainstorm.md`

### Record in Beads

If active task found:
```bash
bd comments add <task-id> "## Plan: {feature name}

Approach: {chosen option name}
Tasks: {N} implementation tasks
Key files: {top 3-5 files}
Plan: .lets/plans/${SLUG}.md"
```

### Show Output

```
## Plan Ready: **{task title}** (`{task-id}`)

Saved: `.lets/plans/{branch-slug}.md`

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
┌─ LETS ──────────────────────────────────┐
│  Review plan? /lets:review --plan       │
│  Execute?     /lets:execute             │
│  New session? /lets:start               │
└─────────────────────────────────────────┘
```

## Rules

- **NEVER write code** outside the plan document in `.lets/plans/`
- **NEVER skip clarifying questions** (Step 3) - vague input produces vague plans
- **EVERY phase transition requires user approval** via AskUserQuestion
- **NEVER hardcode approach names** like "Minimal/Maximal/Pragmatic" - derive from exploration context
- **ALL parallel agents in a SINGLE message** - never sequential when parallel is possible
- **Exact file paths** in plan - verified against explorer findings
- **Complete code snippets** - no stubs, no "implement X here"
- **Plan is the artifact** - session ends when plan is saved
- Respond in user's language
