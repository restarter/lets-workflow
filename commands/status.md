---
description: Show task overview and project status
---

# Task Status

Generate a full overview of the task tracker state.

## Instructions

Run all these commands and compile into a single report:

```bash
# 1. Summary statistics
bd status

# 2. Ready to work
bd ready --limit 20

# 3. In progress
bd list --status=in_progress

# 4. Blocked tasks
bd blocked

# 5. Epic progress
bd epic status

# 6. Priority breakdown
bd list --status=open --json | jq -r '.[].priority' | sort | uniq -c | sort -rn

# 7. Recently closed (last 7 days)
bd list --status=closed --limit 10
```

## Report Format

Present the report in this structure:

```markdown
# Task Status - {project name}
Generated: {current date}

## Summary
Total: X   Open: X   Closed: X
Epics: X   Tasks: X   Bugs: X
Ready: X   Blocked: X  WIP: X

## Priority Distribution
P0 Critical:  {count with bar}
P1 High:      {count with bar}
P2 Medium:    {count with bar}
P3 Low:       {count with bar}
P4 Backlog:   {count with bar}

## Ready to Work (no blockers)
{list from bd ready, mark epics with <- EPIC}

## In Progress
{list from bd list --status=in_progress, show assignee}

## Blocked
{list from bd blocked with blocking reasons}
Example:
1. [P2] proj-1om: Test Coverage
   blocked by: proj-ffj (Refactor CPA Core)

## Epics Progress
{from bd epic status, with visual progress bars}

## Dependency Graph
{show which tasks block others}
Example:
proj-ffj (Refactor)
+-- -> proj-1om (Tests)
+-- -> proj-az6 (Unified Selfhost)

## Recently Closed (7 days)
{list recently closed tasks with relative dates}

## Insights
{AI-generated observations, examples:}
- X tasks blocked by single epic - consider prioritizing
- No P0/P1 tasks - runway is clear
- Task X in progress > 3 days - may need attention
- Bottleneck: proj-ffj blocks 2 other tasks
- All epics at 0% - need to break down into subtasks
```

## Insights to Look For

Analyze the data and report:

1. **Bottlenecks** - tasks that block many others
2. **Stale work** - in_progress too long without updates
3. **Priority imbalance** - too many P1s or empty P1
4. **Empty epics** - epics without children tasks
5. **Dependency chains** - long chains that slow progress
6. **Quick wins** - P3/P4 tasks that are ready and small

## Output

Present the full report directly to the user. Use the box-drawing characters for visual appeal in terminal.

## Suggested Next Action

Based on current state, suggest next step with LETS box:

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

## Language

Report structure in English, but respond to user in their language (Ukrainian/Russian/English).
