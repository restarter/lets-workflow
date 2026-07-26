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
2. **Reduce** - split systemic (any tier) into its own section, NIT-filter the rest (unless small diff), dedupe (keep highest tier), sort by tier.
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
| `spec` | string | task description from the tracker's `show` (or the PR body); **empty when unavailable** - never a sentinel string. Drives `specBlock` in the review prompt and the narrower `specBlockSkeptic` in the verify prompt. Treated as UNTRUSTED data (fenced, authority-bounded) and normalized in the script - the markdown cap is prose only, so the script re-does it: non-string → empty, whitespace-only → empty, a leading `UNAVAILABLE` → empty (belt-and-braces against a sentinel leaking through), 150 lines / 8000 chars with an explicit `[... spec truncated ...]` marker, and any `--- BEGIN/END SPEC` line inside the value is neutralized so the value cannot escape its own fence |
| `specTrusted` | bool | `false` when `spec` came from the PR body - i.e. written by the author of the code under review. Reviewers still get it (scope context); `specBlockSkeptic` is suppressed, because a skeptic's `real=false` is consumed deterministically by `decide()` and would delete a finding with no human in the loop |
| `prTree` | bool | does the working tree hold the reviewed code? `true` for all non-PR modes and for PR mode after a checkout; `false` adds a REVIEW TREE warning to both prompts. **Omitted on a `PR-*` mode is treated as `false`** - failing toward "the tree may be wrong" rather than silently trusting it |

## Returns

`{ verdict, findings[], systemic[], summary{}, counts{blockers,suggestions,total,refuted,verify_failed} }` - the same shape `review.md` Step 8/8.5 persists. `verify_failed` > 0 means that many findings could not be verified (skeptics errored) and were kept unverified - surface it, never treat as "verified clean". `findings` + `verdict` are the `/lets:github-pr --json` contract; do not rename them.

## Constraints (Dynamic Workflow runtime)

- No filesystem - the script returns data; `/lets:review` persists files.
- No sibling `import` - all logic stays inline in `review.workflow.js`.
- No `Date.now()` / `Math.random()` / `new Date()`.
- Top-level `await`/`return` are used (the runtime wraps the body), so the file is NOT Node-importable - it has no clean unit test; the verdict/dedupe/verify logic is kept in sync with `review.md` prose by discipline and validated by the live smoke test.
- **Syntax IS checkable** - but NOT with bare `node --check`: because line 2 is `export const meta`, it exits 0 on syntactically broken input (verified on node v22), including an unterminated template literal - the exact failure mode of the long backticked prompt strings. Copying to `.mjs` fails the other way (`Illegal return statement`). Wrap the body instead, mirroring the runtime:

  ```bash
  { echo 'async function __w(){'; sed 's/^export //' review.workflow.js; echo '}'; } | node --check /dev/stdin
  ```

  Verified to exit 0 on the real file and 1 on a copy with a broken template literal.
