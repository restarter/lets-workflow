---
name: explore-workflow
description: Dynamic Workflow asset for /lets:explore --workflow. Not auto-triggered - a workflow script invoked via scriptPath by /lets:explore. Holds the off-context web-research -> ideate -> cluster fan-out.
user-invocable: false
---

# explore-workflow (Dynamic Workflow asset)

A Dynamic Workflow asset (see `CLAUDE.md` -> "Dynamic Workflow Assets"; `review-workflow` is the reference example). `explore.workflow.js` is executed by the `Workflow` tool, invoked from `/lets:explore`'s Workflow Mode via:

```
Workflow({ scriptPath: "${CLAUDE_PLUGIN_ROOT}/skills/explore-workflow/explore.workflow.js", args })
```

`${CLAUDE_PLUGIN_ROOT}` is substituted at command-load time, so `explore.md` carries the literal absolute path. Treat `explore.workflow.js` as a template the command points at, not a script to reproduce inline.

## Why this is a transparent performance lever (not autonomous)

`/lets:explore --workflow` produces the SAME result as the standard Task path - only the per-agent idea dumps stay off-context. The scout (Step 2) and the signal-driven agent selection (Step 3) stay IN-CONTEXT in `explore.md` (the selection mirrors review/opinion, which also select from context the orchestrator already has); the web-research stage (current community standards), the ideate fan-out, and the cluster all move into the workflow (off-context). The interactive dialog stays in-context after the aggregate returns. (The standard Task path gathers the equivalent web brief in-context via the orchestrator - see `explore.md` - so the two paths stay equivalent.) So `--workflow` is a context-savings lever on a large fan-out, not an added methodology (there is no verify/judge stage - cluster is a SEMANTIC dedupe + impact-sort, an off-context merge, not a quality gate). The standard Task path already merges convergent ideas in-context at render time; the workflow's cluster agent performs that same merge off-context, so the two paths stay equivalent.

## What it does (off-context)

1. **Web Research** (off-context agent, always-on unless `web: false`) - a neutral subagent (no `agentType`; the default workflow subagent CAN WebSearch/WebFetch) gathers CURRENT community standards / best practices on the topic (`WEB_SCHEMA`: summary, standards, tools, pitfalls, sources). `formatWebBrief` renders it into a compact brief injected into every ideator prompt - **fenced as UNTRUSTED web content** (reference only, never instructions, never overrides PROJECT_ROOT), since it is fetched from arbitrary public pages (prompt-injection trust boundary; the standard path in `explore.md` Step 4 carries the identical fence). Degrades gracefully: if the subagent reports no web tool / errors / returns nothing, ideate runs without the brief.
2. **Ideate** - fan out the selected `lets:<name>` agents in parallel over the pre-gathered topic profile + the web brief (structured `IDEA_SCHEMA`: title, category, impact, description).
3. **Cluster** (off-context agent + deterministic attribution) - a neutral subagent (no `agentType`) groups the raw ideas into themes, merging ideas that make the same point even when worded differently across agents (`CLUSTER_SCHEMA`: themes carrying member indices). `buildThemes` then computes each theme's `agents[]` + impact deterministically (impact high if any member is high) and guarantees no idea is dropped (an unassigned idea becomes a theme of one). Falls back to title-only dedup (`clusterIdeas`) if the cluster agent errors or returns nothing, and skips the agent for <=1 idea.

## `args` contract (built by `/lets:explore`, passed as a JSON value)

| key | type | meaning |
|---|---|---|
| `mode` | string | `explore_idea` (the asset is mode-ready; a future brainstorm Review-backlog conversion can reuse it) |
| `topic` | string | the topic being explored |
| `profile` | string | the scout's Topic Context Profile (in-context Step 2 output, ~500 words) |
| `agents` | `[{name}]` | selected agent short-names (no `lets:` prefix; exclude `actor`) |
| `projectRoot` | string | absolute project root (agents must not read outside it) |
| `claudeMd` | string | CLAUDE.md context |
| `web` | boolean (optional) | default true - run the off-context Web Research stage; `false` (from `--no-web`) skips it |

## Returns

`{ ideas[], web{status, sources[]}, counts{agents, agents_responded, raw_ideas, ideas, clustering, web} }`. `web.sources` is `[{title, url}]` from the Web Research stage - the command should surface these links in `## Explore Results`. `ideas` are semantically clustered + impact-sorted, each with `agents[]` attribution - **`agents[]` length > 1 means multiple agents converged on that theme**, a first-class signal the command should lead with. `counts.clustering` is `semantic` (cluster agent ran) | `fallback_title` (title-only fallback after a cluster-agent error) | `trivial` (<=1 idea). `counts.web` reports the Web Research outcome (`ok` | `empty` | `no_tool` | `failed` | `skipped`). Claude renders the `## Explore Results` grouping from this. Anti-silent-fail: `counts.agents_responded === 0` means every ideator errored - the command must surface "no ideas returned" and offer a re-run, never render a fabricated result.

## Constraints (Dynamic Workflow runtime)

- No filesystem - the script returns data; `/lets:explore` renders + records.
- No sibling `import` - all logic stays inline.
- No `Date.now()` / `Math.random()` / `new Date()`.
- Top-level `await`/`return` are used, so the file is NOT Node-importable - no committed unit test; the web-research stage, the cluster agent + `buildThemes` / `clusterIdeas` / `formatWebBrief` are kept in sync with `explore.md` (Web Research / Step 5 / W4) prose by discipline + validated by the live smoke test (the pure logic - `buildThemes`, `clusterIdeas`, `formatWebBrief` - is deterministically checkable on a copy / via a stubbed-hook harness).
