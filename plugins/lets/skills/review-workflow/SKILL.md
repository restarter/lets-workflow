---
name: review-workflow
description: Internal Dynamic Workflow asset for /lets:review --workflow. Not auto-triggered - it is a workflow script invoked via scriptPath, not a conversational skill. Holds the fan-out -> dedupe -> adversarial-verify -> aggregate chain that runs off-context.
user-invocable: false
---

# review-workflow (Dynamic Workflow asset)

This folder is the **reference example** of the Dynamic Workflow asset standard (see `CLAUDE.md` -> "Dynamic Workflow assets"). It is NOT a conversational skill: `review.workflow.js` is executed by the `Workflow` tool, invoked from `/lets:review`'s Workflow Mode via:

```
Workflow({ scriptPath: "${CLAUDE_PLUGIN_ROOT}/skills/review-workflow/review.workflow.js", args })
```

`${CLAUDE_PLUGIN_ROOT}` is substituted at command-load time, so `review.md` carries the literal absolute path. Treat `review.workflow.js` as a **template** the command points at, not a script to reproduce inline.

## What it does (off-context)

A multi-stage chain so per-agent reports never enter the conversation - only the final aggregate returns:

1. **Review** - fan out the selected `lets:<name>` review agents in parallel (structured `FINDING_SCHEMA` output).
2. **Reduce** - NIT-filter (unless small diff), split systemic, dedupe (keep highest tier), sort by tier.
3. **Verify** - for each BLOCKER/SUGGESTION, fan out `lets:skeptic` agents (2, or 3 for BLOCKER) to refute it. Apply the **asymmetric drop rule**: SUGGESTION drops on a simple majority `real=false`; BLOCKER drops only on near-unanimous high-confidence refute, otherwise downgrades. Count `refuted`.
4. **Aggregate** - verdict over the verified set, per-agent summary, counts.

## `args` contract (built by the command, passed as a JSON value)

| key | type | meaning |
|---|---|---|
| `agents` | `[{name}]` | selected review agents (short names, no `lets:` prefix; exclude `actor`) |
| `mode` | string | `PR-{n}` \| `local-review` \| `branch-review` \| `file` |
| `projectRoot` | string | absolute project root (agents must not read outside it) |
| `claudeMd` | string | CLAUDE.md context |
| `changedFiles` | string | changed-file list with stats (or single path for `--file`) |
| `code` | string | the diff (or full file content for `--file`) |
| `smallDiff` | bool | `true` keeps NIT findings (diff < 50 lines) |
| `systemicCheck` | bool | `false` for `--file` (no diff baseline) |

## Returns

`{ verdict, findings[], systemic[], summary{}, counts{blockers,suggestions,total,refuted,verify_failed} }` - the same shape `review.md` Step 8/8.5 persists. `verify_failed` > 0 means that many findings could not be verified (skeptics errored) and were kept unverified - surface it, never treat as "verified clean". `findings` + `verdict` are the `/lets:github-pr --json` contract; do not rename them.

## Constraints (Dynamic Workflow runtime)

- No filesystem - the script returns data; `/lets:review` persists files.
- No sibling `import` - all logic stays inline in `review.workflow.js`.
- No `Date.now()` / `Math.random()` / `new Date()`.
- Top-level `await`/`return` are used (the runtime wraps the body), so the file is NOT Node-importable - it has no clean unit test; the verdict/dedupe/verify logic is kept in sync with `review.md` prose by discipline and validated by the live smoke test.
