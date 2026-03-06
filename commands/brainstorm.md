---
description: Interactive planning - explore codebase, design architecture, evaluate options, produce detailed implementation plan
argument-hint: "[feature description]"
---

# Brainstorm

Turn a task or idea into a detailed implementation plan. Clarifies scope, explores codebase, discusses approaches with user, designs architecture for selected approaches, evaluates with experts, then writes a bite-sized plan.

**HARD-GATE: This command produces a plan, NOT code. No files are modified except .lets/plans/.**

## Step 1: Capture the Goal

**If argument provided:** use it as the feature goal.

**If no argument:** ask:

> "What are you trying to build or change?"

Wait for answer before proceeding.

## Step 2: Active Task Context

```bash
BRANCH=$(git branch --show-current)
# Parse task ID from branch: feature/<task-id>-<slug>

# Fallback:
bd list --status=in_progress
```

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

### Size Heuristic

Check project size before choosing exploration strategy:

```bash
find "$ROOT" -type f -not -path '*/.git/*' -not -path '*/node_modules/*' -not -path '*/vendor/*' | wc -l
```

| Project size | Strategy |
|-------------|----------|
| < 100 files | **Single agent** - one explorer with combined focus |
| >= 100 files | **Full** - 3 parallel explorer agents |

### Single Agent Exploration (< 100 files)

```
Task(
  subagent_type="lets:explorer",
  prompt="FULL EXPLORATION MODE. Map patterns, structure, and integration points for a new feature.

FEATURE GOAL: {feature goal from Step 1}
USER CLARIFICATIONS: {answers from Step 3}
TASK CONTEXT: {task title and description from beads, if available}

YOUR FOCUS:
- Find existing features SIMILAR to what we're building (reusable patterns)
- Map WHERE new code belongs (directory structure, naming conventions)
- Identify HOW new code connects (APIs, entry points, dependencies, tests)

Return a structured exploration report covering all three areas."
)
```

### Full Exploration (>= 100 files, 3 Agents Parallel)

**CRITICAL: Launch ALL 3 explorer agents in a SINGLE message with multiple Task tool calls.**

### Explorer 1: Pattern Scout

```
Task(
  subagent_type="lets:explorer",
  prompt="PATTERN SCOUT MODE. Find existing features SIMILAR to what we're building.

FEATURE GOAL: {feature goal from Step 1}

TASK CONTEXT:
{task title and description from beads, if available}

YOUR FOCUS:
- Find existing features or modules that solve a similar problem
- Trace through them end-to-end: entry point -> processing -> output
- Identify reusable patterns (if something is done 3+ places, it's the standard)
- Find reference implementations that architects should follow

Return the structured exploration report as defined in your system prompt.
Focus ONLY on similar existing patterns - leave structure mapping and integration to other explorers."
)
```

### Explorer 2: Structure Mapper

```
Task(
  subagent_type="lets:explorer",
  prompt="STRUCTURE MAPPER MODE. Map WHERE the new feature fits in the codebase.

FEATURE GOAL: {feature goal from Step 1}

TASK CONTEXT:
{task title and description from beads, if available}

YOUR FOCUS:
- Directory and module structure - where does new code belong?
- File naming conventions (what patterns do filenames follow?)
- Function/variable/type naming conventions
- Configuration and build patterns
- How are new modules typically added?

Return the structured exploration report as defined in your system prompt.
Focus ONLY on structure and conventions - leave patterns and integration to other explorers."
)
```

### Explorer 3: Integration Analyst

```
Task(
  subagent_type="lets:explorer",
  prompt="INTEGRATION ANALYST MODE. Map HOW new code connects to existing code.

FEATURE GOAL: {feature goal from Step 1}

TASK CONTEXT:
{task title and description from beads, if available}

YOUR FOCUS:
- Public APIs and interfaces that new code must work with
- Entry points where new code hooks into existing system
- Dependencies that the new feature will rely on
- Test infrastructure: framework, patterns, file locations, representative test example
- Potential breaking points and tight coupling risks

Return the structured exploration report as defined in your system prompt.
Focus ONLY on integration and testing - leave patterns and structure to other explorers."
)
```

**If any explorer fails or returns no useful data:**
- Note the gap explicitly in the Codebase Map ("No data for {area} - explorer failed")
- Ask user: "Explorer {N} couldn't map {area}. Continue with partial data, or should I explore {area} manually?"

After all return, synthesize a combined codebase map:

```
## Codebase Map

### Similar Patterns (from Pattern Scout)
{key findings - what exists, what to follow}

### Where It Fits (from Structure Mapper)
{directory, naming conventions, module location}

### How It Connects (from Integration Analyst)
{entry points, APIs, test patterns, risks}
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

### Checkpoint: Expert Selection

```
AskUserQuestion(
  questions=[{
    question: "Which experts should evaluate the architecture?",
    header: "LETS",
    options: [
      { label: "Recommended", description: "{pragmatist + N domain experts based on feature}" },
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
- **Skip evaluation** -> proceed directly to Step 8
- **Other** (free text) -> parse expert names from text, dispatch selected

### Dispatch Experts

**CRITICAL: Launch ALL selected agents in a SINGLE message.**

For each expert:

```
Task(
  subagent_type="lets:{agent-name}",
  prompt="ultrathink

RESPONSE LANGUAGE: {language from LETS Config, e.g. "English"}

OPINION MODE. Evaluate an architecture design for a feature.

FEATURE GOAL: {goal}

CHOSEN ARCHITECTURE:
{full architect output for chosen approach}

INSTRUCTIONS:
- Evaluate this architecture from your area of expertise
- Flag risks that others might miss
- Suggest specific improvements (not vague concerns)
- Be direct - no hedging, no 'it depends' without conclusion
{mandatory agent context - see table below}"
)
```

### Mandatory Agent Context

If a selected agent appears in this table, append the instruction to its prompt:

| Agent | Instruction |
|-------|-------------|
| `compliance` | "Only flag violations EXPLICITLY mentioned in CLAUDE.md. Quote the rule being violated." |
| `git-historian` | "Use git blame and git log to analyze historical context." |
| `docs` | "Check CLAUDE.md sync, docs/ sync, beads tracking, README/config docs." |
| `pragmatist` | "Assess if the solution is proportional to the problem. Flag overengineering." |

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
    question: "Proceed to plan generation?",
    header: "LETS",
    options: [
      { label: "Generate plan", description: "Architecture approved, write the implementation plan" },
      { label: "Adjust architecture", description: "Incorporate expert feedback first" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Generate plan** -> proceed to Step 8
- **Adjust architecture** -> discuss changes, update architecture, ask again
- **Other** (free text) -> treat as adjustment request

## Step 8: Plan Generation

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

## Step 9: Save & Output

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
