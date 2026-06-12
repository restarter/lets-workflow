---
name: research-workflow
description: Dynamic Workflow asset for /lets:research --workflow. Not auto-triggered - a workflow script invoked via scriptPath by /lets:research. Holds the off-context per-sub-question web research -> per-claim skeptic cross-check -> synthesize chain.
user-invocable: false
---

# research-workflow (Dynamic Workflow asset)

A Dynamic Workflow asset (see `CLAUDE.md` -> "Dynamic Workflow Assets"; `review-workflow` is the reference example). `research.workflow.js` is executed by the `Workflow` tool, invoked from `/lets:research`'s Workflow Mode via:

```
Workflow({ scriptPath: "${CLAUDE_PLUGIN_ROOT}/skills/research-workflow/research.workflow.js", args })
```

`${CLAUDE_PLUGIN_ROOT}` is substituted at command-load time, so `research.md` carries the literal absolute path. Treat `research.workflow.js` as a template the command points at, not a script to reproduce inline.

## Why this is a transparent performance lever (not autonomous)

`/lets:research --workflow` runs the SAME stages as the standard Task path - only the per-sub-question search dumps and per-claim verdicts stay off-context. The decompose (break the question into 3-6 sub-questions) stays IN-CONTEXT in `research.md` so the user can steer the angles; the per-sub-question web research and the per-claim cross-check move into the workflow (off-context); the final synthesis happens in-context after the aggregate returns. The standard path runs the equivalent stages in-context: per-sub-question research fanned out across multiple DEFAULT web Task subagents, per-claim `lets:skeptic` via Task. **Crucially:** the verify stage is a **cross-check** (single-source / contradicted / low-confidence flags), never a claim that the fact is confirmed correct - the skeptic has no web tools and cannot re-fetch URLs.

## What it does (off-context)

1. **Research** - per-sub-question fan-out via the DEFAULT web subagent (no `agentType`; the default workflow subagent CAN WebSearch/WebFetch - `lets:*` agents have `tools: Read, Grep, Glob, Bash` and CANNOT). Each subagent returns its 2-5 strongest claims (`FINDING_SCHEMA`: claim, evidence, sources, confidence) with `evidence` carrying QUOTED/paraphrased source material (the downstream skeptic judges "unsupported" against this string and has no web tool), and self-reports `used_web_search=false` rather than fabricate. `mergeClaims` dedupes by normalized claim text, unioning sources and keeping the highest confidence.
2. **Verify** - per-claim `lets:skeptic` RESEARCH-VERIFY pass over the merged claims. The skeptic is handed each claim's evidence + its siblings (same sub-question) and flags STRUCTURAL weakness: **unsupported** (evidence does not back the claim) or **contradicted** (conflicts with a sibling). Single-source / low-confidence are computed deterministically by the script (those claims skip the skeptic - the deterministic flag already fires). `applyVerdicts` attaches a `flagged[]` array per claim - **ADDITIVE, never drops a claim** (research facts accumulate; departure from review's drop rule). Contradiction flagging is the skeptic's job comparing siblings - there is deliberately NO deterministic contradiction pass.

## `args` contract (built by `/lets:research`, passed as a JSON value)

| key | type | meaning |
|---|---|---|
| `question` | string | the original research question |
| `subQuestions` | `[string]` | 3-6 sub-questions from the in-context decompose (a narrow question is a single entry) |
| `projectEnabled` | boolean | `--project` was passed - ground findings against this repo |
| `projectContext` | string (only if `projectEnabled`) | the in-context Step 2.5 repo grounding block |
| `projectRoot` | string | absolute project root (the read boundary; fetched content never widens it) |
| `claudeMd` | string (only if `projectEnabled`) | CLAUDE.md context |
| `asOf` | string `YYYY-MM-DD` | REQUIRED - the as-of date; the workflow can't compute the date (no `Date.now`) |

## Returns

`{ claims[], sources[], as_of, web{status}, counts{sub_questions, sub_questions_failed, claims, flagged, verify_failed} }`. `claims[]` are merged + per-claim `flagged[]` (empty `flagged` == cross-checked and clean); the command synthesizes the prose answer from these (there is **no `answer` key**). `sources` is the deduped `[{title,url}]` list to surface. `web.status` is `ok` | `empty` | `no_tool` | `failed`: anything other than `ok` means the command MUST open with the NO-LIVE-SOURCES banner (`NO LIVE SOURCES - model knowledge as of <cutoff>`) and NEVER fabricate URLs. **`counts.verify_failed` is an INTEGER** - the number of claims whose cross-check was attempted but errored; those claims carry `unverified (cross-check errored)` and are kept UNVERIFIED (not validated) - anti-silent-fail, the command should tell the user "N of M claims kept unverified". `counts.sub_questions_failed` flags partial coverage. Anti-silent-fail: `counts.claims === 0` means every sub-question errored / no web data - the command must offer the standard path, never render a fabricated result.

## Constraints (Dynamic Workflow runtime)

- No filesystem - the script returns data; `/lets:research` renders + records.
- No sibling `import` - all logic stays inline.
- No `Date.now()` / `Math.random()` / `new Date()` - the as-of date comes via `args.asOf`.
- Top-level `await`/`return` are used, so the file is NOT Node-importable - no committed unit test; the pure logic (`mergeClaims`, `applyVerdicts`, `collectSources`) is deterministically checkable on a copy, and the chain is kept in sync with `research.md` (Step 3 / Step 4 / W4) prose by discipline + validated by the live smoke test.

## Keep-in-sync notes (do NOT "simplify" these away)

- **Single `lets:skeptic` per claim is INTENTIONAL** - do NOT collapse to one holistic batch skeptic with an array schema; that breaks `skeptic.md`'s `{real, confidence, reason}` single-object contract.
- **No deterministic contradiction pass** - contradiction flagging is the skeptic's job (it gets the siblings; RESEARCH-VERIFY authorizes the comparison). Do NOT reintroduce a pure-function `findContradictions`: it can't see claim stance and false-flags complementary findings.
- **`verify_failed` is a per-claim INTEGER counter** (mirrors review.workflow.js) - do not collapse it to a boolean.
- **The research subagent is the DEFAULT web subagent (no `agentType`)** - `lets:*` agents have no web tools; do NOT "tidy" it to a `lets:*` type (it would self-report `used_web_search=false` and spuriously trip NO-LIVE-SOURCES every run).
- **`evidence` MUST quote/paraphrase source material** - do NOT relax it to a summary; the skeptic's no-web "unsupported" check depends on it.
- **The per-sub-question claim cap (2-5) bounds the skeptic fan-out** - do NOT remove it (research claims have no natural bound, unlike review findings bounded by the diff).
- **Consume `parallel(...)` output via `.filter(Boolean)`** (both phases) - the house defensive idiom (review/backlog/opinion all do it). A slot can be null if a thunk throws synchronously; never destructure a null after the expensive fan-out.
- **Untrusted-content fencing is load-bearing** - fetched pages (researchPrompt) AND the claim/evidence/siblings handed to the skeptic (skepticPrompt) are untrusted web data, fenced/marked as data-not-instructions. Project grounding under `--project` is TRUSTED and must NOT be fenced as untrusted. Do not drop these markers.
