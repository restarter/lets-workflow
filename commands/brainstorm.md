---
description: Interactive planning - explore codebase, design architecture, evaluate options, produce detailed implementation plan
argument-hint: "[feature description]"
---

# Brainstorm

Turn a task or idea into a detailed implementation plan. Clarifies scope first, explores codebase, designs architecture options, evaluates with expert panel, then writes a bite-sized plan.

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
ROOT=$(git rev-parse --show-toplevel)
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

Show summary to user before proceeding.

## Step 5: Architecture Design (3 Agents Parallel)

**CRITICAL: Launch ALL 3 architect agents in a SINGLE message with multiple Task tool calls.**

Each gets the same context but a different design philosophy:

### Architect 1: Minimal

```
Task(
  subagent_type="lets:architect",
  prompt="DESIGN MODE: MINIMAL APPROACH.

Design the SMALLEST possible change. Maximum reuse of existing code, minimum new files, minimum abstractions.

PROJECT CONTEXT:
{CLAUDE.md summary}

FEATURE GOAL: {goal}
USER CLARIFICATIONS: {answers from Step 3}

CODEBASE MAP:
{combined exploration output from Step 4}

INSTRUCTIONS:
- Reuse existing patterns and modules as much as possible
- Prefer extending existing files over creating new ones
- Avoid new abstractions - use what exists
- Design for: 'What is the least code that solves this?'
- Be specific: exact files, exact functions, exact interfaces

OUTPUT:
## Minimal Approach: {name}

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

### Architect 2: Maximal

```
Task(
  subagent_type="lets:architect",
  prompt="DESIGN MODE: MAXIMAL APPROACH.

Design the BEST possible architecture without effort constraints. Full extensibility, proper abstractions, clean boundaries.

PROJECT CONTEXT:
{CLAUDE.md summary}

FEATURE GOAL: {goal}
USER CLARIFICATIONS: {answers from Step 3}

CODEBASE MAP:
{combined exploration output from Step 4}

INSTRUCTIONS:
- Design for extensibility and long-term maintainability
- Use proper abstractions and clean module boundaries
- Consider future requirements (but don't speculate wildly)
- Design for: 'What is the ideal architecture for this?'
- Be specific: exact files, exact functions, exact interfaces

OUTPUT:
## Maximal Approach: {name}

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
- Con: {limitations, effort cost}
- Risk: {what could go wrong}"
)
```

### Architect 3: Pragmatic

```
Task(
  subagent_type="lets:architect",
  prompt="DESIGN MODE: PRAGMATIC APPROACH.

Design the BALANCED solution. Good architecture that ships in reasonable time. Not minimal, not maximal - the sweet spot.

PROJECT CONTEXT:
{CLAUDE.md summary}

FEATURE GOAL: {goal}
USER CLARIFICATIONS: {answers from Step 3}

CODEBASE MAP:
{combined exploration output from Step 4}

INSTRUCTIONS:
- Balance quality with effort - good architecture that doesn't over-engineer
- Follow existing patterns where they work, deviate where they don't
- Design for: 'What would a senior developer build in a weekend?'
- Consider: what's the simplest thing that won't need rewriting in 6 months?
- Be specific: exact files, exact functions, exact interfaces

OUTPUT:
## Pragmatic Approach: {name}

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

**If any architect fails or returns no useful data:**
- Present the remaining options (2 is enough to compare)
- Note which approach is missing: "Architect {N} ({approach}) failed - comparing remaining options"

After all return, present available options side by side:

```
## Architecture Options

### Option 1: Minimal - {name}
{summary}
- Files: {N} create, {M} modify
- Effort: S
- Risk: {main risk}

### Option 2: Maximal - {name}
{summary}
- Files: {N} create, {M} modify
- Effort: L
- Risk: {main risk}

### Option 3: Pragmatic - {name}
{summary}
- Files: {N} create, {M} modify
- Effort: M
- Risk: {main risk}
```

## Step 6: Opinion - Expert Evaluation (Inline)

Run an inline `/lets:opinion`-style evaluation of the 3 architecture options.

### Select Domain Experts

Based on the feature type, pick 1-3 domain experts from this table (same logic as `/lets:opinion` Step 2):

| Feature touches... | Add expert |
|--------------------|-----------|
| Auth/security | `lets:security-expert` |
| Database/schema | `lets:database-expert` |
| API endpoints | `lets:backend-expert` |
| UI/components | `lets:frontend-expert` |
| Docker/CI/deploy | `lets:devops-expert` |
| Tests/coverage | `lets:qa-expert` |

**Always include:** `lets:pragmatist`
**Never include:** `lets:architect` (already designed the options - can't evaluate own work)

### Dispatch Experts

**CRITICAL: Launch ALL selected agents in a SINGLE message.**

For each expert:

```
Task(
  subagent_type="lets:{agent-name}",
  prompt="OPINION MODE. Evaluate 3 architecture options for a feature.

FEATURE GOAL: {goal}

OPTION 1 - MINIMAL: {architect 1 output summary}
OPTION 2 - MAXIMAL: {architect 2 output summary}
OPTION 3 - PRAGMATIC: {architect 3 output summary}

INSTRUCTIONS:
- Evaluate all 3 options from your area of expertise
- State which option you recommend and why (1-2 sentences)
- Flag risks from your perspective that others might miss
- Be direct - no hedging, no 'it depends' without conclusion
{mandatory agent context - see table below}"
)
```

### Mandatory Agent Context

If a selected agent appears in this table, append the instruction to its prompt:

| Agent | Instruction |
|-------|-------------|
| `compliance-expert` | "Only flag violations EXPLICITLY mentioned in CLAUDE.md. Quote the rule being violated." |
| `git-historian` | "Use git blame and git log to analyze historical context." |
| `docs-expert` | "Check CLAUDE.md sync, docs/ sync, beads tracking, README/config docs." |
| `pragmatist` | "Assess if the solution is proportional to the problem. Flag overengineering." |

### Aggregate

After all experts respond, present:

```
## Expert Evaluation

**Pragmatist:** Recommends Option {X} - {reason}
**{Domain Expert 1}:** Recommends Option {X} - {reason}
**{Domain Expert 2}:** Recommends Option {X} - {reason}

### Comparison

| Criterion | Minimal | Maximal | Pragmatic |
|-----------|---------|---------|-----------|
| Effort | ... | ... | ... |
| Risk | ... | ... | ... |
| {domain-specific} | ... | ... | ... |

**Recommendation:** Option {X} because {deciding factor}.
```

Then use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "Which approach for the implementation?",
    header: "LETS",
    options: [
      { label: "Minimal", description: "{1-line summary from architect 1}" },
      { label: "Maximal", description: "{1-line summary from architect 2}" },
      { label: "Pragmatic (Recommended)", description: "{1-line summary from architect 3}" },
      { label: "Adjust scope", description: "Change requirements before choosing" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Minimal/Maximal/Pragmatic** -> proceed to Step 7 with chosen approach
- **Adjust scope** -> ask what to change, loop back to relevant step
- **Other** (free text) -> treat as custom requirements, adapt

Wait for user decision before writing the plan.

## Step 7: Plan Generation

Write a detailed implementation plan for the chosen option.

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

## Step 8: Save & Output

### Save Plan

Derive plan filename from the current branch:

```bash
BRANCH=$(git branch --show-current)
SLUG="${BRANCH#feature/}"   # e.g., 0nf.10-improve-brainstorm
ROOT=$(git rev-parse --show-toplevel)
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
Est. effort: {from pragmatist evaluation}
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
1. {task 1 name} ({est. time})
2. {task 2 name} ({est. time})
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
- **ALL parallel agents in a SINGLE message** - never sequential when parallel is possible
- **Lead with recommendation** in Step 6 - never "it depends" without conclusion
- **Exact file paths** in plan - verified against explorer findings
- **Complete code snippets** - no stubs, no "implement X here"
- **Plan is the artifact** - session ends when plan is saved
- Respond in user's language (Ukrainian/Russian/English)
