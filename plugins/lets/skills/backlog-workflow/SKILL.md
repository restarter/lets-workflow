---
name: backlog-workflow
description: Dynamic Workflow asset for /lets:backlog review --workflow. Not auto-triggered - a workflow script invoked via scriptPath by /lets:backlog. Holds the off-context ideate -> cluster fan-out over the backlog.
user-invocable: false
---

# backlog-workflow (Dynamic Workflow asset)

A Dynamic Workflow asset (see `CLAUDE.md` -> "Dynamic Workflow Assets"; `review-workflow` is the reference example). `backlog.workflow.js` is executed by the `Workflow` tool, invoked from `/lets:backlog`'s Workflow Mode via:

```
Workflow({ scriptPath: "${CLAUDE_PLUGIN_ROOT}/skills/backlog-workflow/backlog.workflow.js", args })
```

`${CLAUDE_PLUGIN_ROOT}` is substituted at command-load time, so `backlog.md` carries the literal absolute path. Treat `backlog.workflow.js` as a template the command points at, not a script to reproduce inline.

## Why this is a transparent performance lever (not autonomous)

`/lets:backlog review --workflow` produces the SAME result as the standard Task path - only the per-agent backlog-idea dumps stay off-context. The explorer scout (Phase 1) and the signal-driven agent selection (Phase 2) stay IN-CONTEXT in `backlog.md`; the ideate fan-out (Phase 3) and the aggregate/cluster (Phase 4) move into the workflow (off-context). The interactive dialog (Phase 5) + capture (Phase 6) stay in-context after the aggregate returns. There is **no Web Research stage**: the standard backlog Phase 3 ideates over the project's own profile + CLAUDE.md without web search, so adding web here would diverge the two paths - `--workflow` must stay a pure context-savings lever, same findings. The cluster is a SEMANTIC dedupe + impact-sort (an off-context merge), not a quality gate.

## What it does (off-context)

1. **Ideate** - fan out the selected `lets:<name>` agents in parallel over the explorer's Project State Profile + CLAUDE.md (structured `IDEA_SCHEMA`: title, category=`missing|adjust|theme`, impact, description). `adjust` ideas embed the referenced task as **Title** (`task-id`) in description.
2. **Cluster** (off-context agent + deterministic attribution) - a neutral subagent (no `agentType`) groups raw ideas into themes, merging ideas that make the same point even when worded differently (`CLUSTER_SCHEMA`: themes carrying member indices). `buildThemes` computes each theme's `agents[]` + impact deterministically (high if any member is high) and guarantees no idea is dropped. Falls back to title-only dedup (`clusterIdeas`) if the cluster agent errors or returns nothing; skips the agent for <=1 idea.

## `args` contract (built by `/lets:backlog`, passed as a JSON value)

| key | type | meaning |
|---|---|---|
| `profile` | string | the explorer's Project State Profile (in-context Phase 1 output, ~500 words) |
| `backlogState` | string | the orchestrator's capped BACKLOG STATE block (Phase 1) - the agents' only task data; they have no tracker access of their own |
| `agents` | `[{name}]` | selected agent short-names (no `lets:` prefix; exclude `actor`) |
| `projectRoot` | string | absolute project root (agents must not read outside it) |
| `claudeMd` | string | CLAUDE.md context (first ~100 lines) |

## Returns

`{ ideas[], counts{agents, agents_responded, raw_ideas, ideas, clustering} }`. `ideas` are semantically clustered + impact-sorted, each with `agents[]` attribution - **`agents[]` length > 1 means multiple agents converged on that theme**, a first-class signal the command should lead with. `counts.clustering` is `semantic` (cluster agent ran) | `fallback_title` (title-only fallback) | `trivial` (<=1 idea). The schema carries `category` (`missing`/`adjust`/`theme`) + `impact` only - Phase 4's "Gaps" / "Quick Wins" buckets are derived by judgment at render time, not labelled in the schema (same as the standard path). Anti-silent-fail: `counts.agents_responded === 0` means every ideator errored - the command must surface "no ideas returned" and offer a re-run, never render a fabricated result.

## Constraints (Dynamic Workflow runtime)

- No filesystem - the script returns data; `/lets:backlog` renders + records.
- No sibling `import` - all logic stays inline.
- No `Date.now()` / `Math.random()` / `new Date()`.
- Top-level `await`/`return` are used, so the file is NOT Node-importable - no committed unit test; the pure logic (`buildThemes`, `clusterIdeas`) is category-agnostic and kept in sync with `backlog.md` Phase 4 prose by discipline + a live smoke test.
