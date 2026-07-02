---
description: Quick session status - what's done and what's planned (short summary, no file output)
---

# Task Status

Show task tracker state. Supports focused views via argument or interactive selection.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## Step 1: Determine View

**If argument is provided** (e.g., `/lets:status overview`), use that view directly.

**If no argument**, ask the user:

```
AskUserQuestion(
  questions=[{
    question: "What do you want to see?",
    header: "Status",
    options: [
      { label: "Overview", description: "Summary, label groups progress, top ready tasks" },
      { label: "Ready", description: "All tasks ready to work (no blockers)" },
      { label: "Labels", description: "Detailed progress by epic:* label group" },
      { label: "Blocked", description: "Blocked tasks and dependency graph" },
      { label: "Full", description: "Everything - summary, ready, blocked, deps, insights" }
    ],
    multiSelect: false
  }]
)
```

## Step 2: Run Commands for Selected View

### View: overview

Compact view. Used by `/lets:start`.

```lets-tracker
stats                              # project totals + per-epic:* label-group progress (beads-native dashboard)
label                              # the epic:* label set
ready limit=5
list-by-status status=in_progress
```

> Degrade: on a tracker that marks `stats`/`label` `absent` (e.g. `none`, or an MCP tracker without graph views), skip the Project-Health / Label-Group sections and tell the user those views are beads-native.

Output format:

```
## Project Health

{total} tasks  {wide progress bar 24 chars}  X% done
               {closed} closed · {wip} wip · {ready} ready · {blocked} blocked

### Label Groups
{label name, left-padded}  {progress bar 24 chars}  XX%  NN/MM
{label name, left-padded}  {progress bar 24 chars}  XX%  NN/MM
...

### In Progress
{list if any, otherwise skip section}

### If $LETS_PR_FLOW == github:

```bash
gh pr list --state open --author @me --json number,title,headRefName,url,reviewDecision --limit 5 2>/dev/null
```

Add to output after "### In Progress":

```
### Open PRs
#42 **{title}** ({branch}) - {reviewDecision or "pending"}
#38 **{title}** ({branch}) - {reviewDecision or "pending"}
(skip section entirely if no open PRs or $LETS_PR_FLOW != github)
```

### Top Ready
| Prio | Task |
|------|------|
| P2 | **Title** (`id`) |
...
(showing top 5, {N} more ready)
```

### View: ready

```lets-tracker
ready limit=0  # ALL ready tasks (0 = unlimited; a bare `ready` uses the tracker's default cap)
label          # the epic:* label set
```

Group ready tasks by `epic:*` label. Tasks without a label go under "Other".

Output format:

```
## Ready Tasks (no blockers)

### {label}
  P2  **Title** (`id`)
  P2  **Title** (`id`)
  P3  **Title** (`id`)

### {another label}
  P2  **Title** (`id`)
  P3  **Title** (`id`)

### Other
  P3  **Title** (`id`)

{N} ready tasks total
```

### View: labels

```lets-tracker
stats          # per-epic:* label-group progress (beads-native; on `stats`/`label` absent, render "label groups unavailable for tracker {name}")
label          # the epic:* label set
```

Output format:

```
## Label Groups

### {label}  {progress bar 24 chars}  XX%  NN/MM

| Prio | Task | Status |
|------|------|--------|
| P2 | **Title** (`id`) | ready / blocked / wip |
...

### {another label}  {progress bar 24 chars}  XX%  NN/MM
...

(repeat for each epic:* label, sorted by progress % descending)
```

### View: blocked

```lets-tracker
stats view=blocked   # dependency-graph tree - the ready/stats binding's dep-graph view (beads-native); on `absent`, render "dependency graph unavailable for tracker {name}"
```

Show dependency graph as ASCII tree. Group by root blocker - the task that ultimately blocks others.

Output format:

```
## Dependency Graph

**Root Blocker Title** (`id`) [status]
 ├── **Blocked Task** (`id`) [blocked]
 │    └── **Transitively Blocked** (`id`) [blocked]
 └── **Another Blocked** (`id`) [blocked]

**Another Root** (`id`) [status]
 └── **Blocked Task** (`id`) [blocked]

{N} blocked tasks, {M} root blockers
```

If no blocked tasks: "No blocked tasks."

### View: full

Run all commands:

```lets-tracker
stats                                  # totals + per-epic:* label groups + priority histogram (beads-native dashboard)
ready limit=0                          # ALL ready tasks (0 = unlimited)
list-by-status status=in_progress
stats view=blocked                     # dependency-graph tree (beads-native)
label                                  # the epic:* label set
list-by-status status=closed limit=10  # recent activity
```

> Degrade: a tracker that marks `stats`/`blocked`/`label` `absent` renders only the `list-by-status`/`ready` sections + a "richer views are beads-native" note — never an empty/broken dashboard. (The priority histogram + per-epic groups are folded into the beads `stats` binding.)

Output format - full report:

```
# Task Status - {project name}

## Project Health

{total} tasks  {wide progress bar 24 chars}  X% done
               {closed} closed · {wip} wip · {ready} ready · {blocked} blocked

## Priority Distribution
P0 Critical:  {bar} N
P1 High:      {bar} N
P2 Medium:    {bar} N
P3 Low:       {bar} N
P4 Backlog:   {bar} N
(only show priorities that have tasks)

## Label Groups
{label name, left-padded}  {progress bar 24 chars}  XX%  NN/MM
{label name, left-padded}  {progress bar 24 chars}  XX%  NN/MM
...
(sorted by progress % descending)

## Ready to Work

### {label}
  P2  **Title** (`id`)
  P3  **Title** (`id`)

### {another label}
  P2  **Title** (`id`)
...

## In Progress
{list or "None"}

### If $LETS_PR_FLOW == github:

```bash
gh pr list --state open --json number,title,headRefName,author,url,reviewDecision --limit 20 2>/dev/null
```

Add to output after "## In Progress":

```
## Pull Requests
| PR | Branch | Review |
|----|--------|--------|
| #42 **{title}** | feature/... | APPROVED / CHANGES_REQUESTED / pending |
...
(skip section entirely if $LETS_PR_FLOW != github)
```

## Dependency Graph
{ASCII tree as in blocked view, or "No blocked tasks."}

## Recent Activity
{last 5-10 closed tasks with dates}
  2026-02-25  **Title** (`id`)
  2026-02-24  **Title** (`id`)
  2026-02-23  **Title** (`id`)
  ...

## Insights
{AI-generated observations:}
- Bottlenecks: tasks that block many others
- Stale work: in_progress too long
- Priority imbalance
- Quick wins: small ready P3/P4 tasks
- Epic progress comparison
```

## Visual Format Rules

- **Task references:** always `**Title** (id)` format. No bare IDs anywhere.
- Scan output before presenting - fix any bare IDs.
- **Progress bars:** `████████████░░░░░░░░░░░░` style, **24 chars** wide.
- **Alignment:** pad epic names to same width so progress bars align vertically.
- **Sorting:** epics by progress % descending (most complete first). Ready tasks by priority ascending (P0 first).
- Report structure in English, respond to user in their language.

## Rules

- Respond in user's language

## LETS Box

Based on current state:

**No WIP tasks:**
```
┌─ LETS ─────────────────────────┐
│  Start?  /lets:start           │
└────────────────────────────────┘
```

**Has WIP:**
```
┌─ LETS ─────────────────────────┐
│  Note?    /lets:note           │
│  Check?   /lets:check          │
└────────────────────────────────┘
```

**Uncommitted changes:**
```
┌─ LETS ─────────────────────────┐
│  Commit?  /lets:commit         │
└────────────────────────────────┘
```

**When called as `overview` from `/lets:start`** - no LETS box (start.md handles next steps).
