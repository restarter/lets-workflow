---
name: plan-workflow
description: PREVIEW Dynamic Workflow asset behind /lets:plan-workflow. Not auto-triggered - a workflow script invoked via scriptPath. The autonomous-planning chain, shipped for cross-project testing before it folds into native /lets:plan.
user-invocable: false
---

# plan-workflow (PREVIEW Dynamic Workflow asset)

**Preview / experimental.** `plan.workflow.js` is the autonomous-planning chain, invoked by `/lets:plan-workflow` via:

```
Workflow({ scriptPath: "${CLAUDE_PLUGIN_ROOT}/skills/plan-workflow/plan.workflow.js", args })
```

It runs the autonomous design without touching the heavily-used interactive `/lets:plan`. Once stable (after cross-project testing), the asset folds into native `/lets:plan` (as a `--workflow` mode or equivalent) and this standalone is retired (lets-jsw00).

## Why this is autonomous-only (not a re-skin of review/opinion)

`/lets:review --workflow` and `/lets:opinion --workflow` are performance levers: same result, off-context. Native `/lets:plan` is interactive (clarify -> explore -> approaches -> architecture -> eval -> discuss); every checkpoint forces intermediate results back into context, which kills the off-context win. So the only workflow that pays off is the **autonomous whole-command** form: the user front-loads a RUBRIC, judge agents make the informed picks against it, and the run returns a plan + decision log. Steer-by-rubric + approve-at-end.

## Stages (off-context, NO user gate between them)

1. **Explore** - fan out `lets:explorer` per focus area (`EXPLORE_SCHEMA`) -> codebase map.
2. **Approaches** - one `lets:architect` synthesizes 2-4 distinct approaches from the map + goal + rubric (`APPROACHES_SCHEMA`).
3. **Architect** - one `lets:architect` per approach -> full architecture (`ARCH_SCHEMA`).
4. **Judge** - a panel (default `pragmatist, backend, security` - never `architect`) scores each architecture against the rubric and picks a winner (`JUDGE_SCHEMA`); `aggregateJudges` tallies winner-votes (tiebreak: summed totals) over REAL approach ids only.
5. **Evaluate** - expert panel evaluates the winner for risks (`EVAL_SCHEMA`).
6. **Plan** - one `lets:architect` writes the bite-sized plan markdown, folding in eval findings (`PLAN_SCHEMA`).
7. **Plan Review** - fan out `lets:architect` + `lets:pragmatist` over the WRITTEN plan (mirror `/lets:review --plan`, `PLAN_REVIEW_SCHEMA`) -> verdict + findings.
8. **Revise** - one `lets:architect` applies the review findings -> revised plan (`REVISE_SCHEMA`); skipped if no findings; planMd never lost on agent error.
9. **Plan Check** - one `lets:pragmatist` runs a quick 5-lens sanity pass (mirror `/lets:check --plan`, `PLAN_CHECK_SCHEMA`) -> verdict + findings; catches regressions the revise introduced.
10. **Refine** - one `lets:architect` applies the check findings -> final plan; skipped if clean.

**Fast mode (`fast: true`) per-stage budget:** Explore = 1 explorer over a single merged focus area (instead of N). Approaches = 1 architect (best-first ordered against the rubric, since only the top one is architected). Architect = top-1 approach only (`approaches.slice(0,1)`). Judge = 1 judge (enum still constrained to the real architected id; if it errors with a single candidate, the lone approach is taken and `decision_log.forced = 'single-candidate'`). Evaluate = 1 expert. Plan = 1 architect. **Stages 7-8 (Plan Review -> Revise) are SKIPPED** - 0 agents; `refinement_log.review_skipped = true`. **Stages 9-10 (Plan Check -> Refine) RUN in both modes** - 1 checker, +1 refiner only on check findings. Net ~7 agents (+1 conditional) vs ~15-25. Default (no flag) is byte-identical to standard.

Stages 7-10 are a single-pass **self-repair loop**: the plan reviews + fixes itself off-context before it ever reaches the user (who still approves at the end). `refinement_log` records the review/check verdicts and whether fixes landed.

## `args` contract

| key | type | meaning |
|---|---|---|
| `goal` | string | what to build/change |
| `rubric` | string | the steering criteria (replaces interactive picks) |
| `focusAreas` | `[{name, hint}]` | exploration areas (dispatcher-derived) |
| `judges` | `[{name}]` | judge panel (exclude `architect`) |
| `experts` | `[{name}]` | winner-evaluation panel |
| `taskContext` | string | active tracker task context (or empty) |
| `projectRoot` | string | absolute root (agents must not read outside it) |
| `claudeMd` | string | CLAUDE.md context |
| `fast` | boolean (optional) | lean budget — 1 agent/stage; merges focus areas to 1 explorer, architects only the top-ranked approach, 1 judge, 1 expert; SKIPS the heavy Plan Review/Revise pass, KEEPS the quick Plan Check/Refine. Default `false`. Distinct from native `/lets:plan --fast` (orchestrator-only, no subagents). |
| `planReviewers` | `[{name}]` (optional) | self-repair Plan Review panel; default `architect, pragmatist`. Ignored in fast mode (review pass skipped). |
| `planChecker` | `{name}` (optional) | Plan Check agent; default `pragmatist`. Runs in BOTH modes (fast keeps the quick check). |

## Returns

`{ plan_markdown, delivered_approach, diverged_from_winner, divergence_reason, review_findings[], check_findings[], refinement_log{}, decision_log, winner, winner_name, approaches[], eval_findings[], counts{} }`. `plan_markdown` is the FINAL self-repaired plan (after Plan Review -> Revise -> Plan Check -> Refine). `refinement_log` = `{review_verdict, review_findings, review_fixed, review_failed, review_skipped, check_verdict, check_findings, check_fixed, check_failed}` - the self-repair audit (if reviewers/checker all errored, the prior plan is kept and the `*_failed` flag says so; never silently skipped). In fast mode the heavy review pass is skipped, so `refinement_log` carries `review_skipped: true` with `review_verdict: null` and `review_failed: false` (a deliberate fast-skip is NOT a failure), while `check_verdict`/`check_findings`/`check_fixed`/`check_failed` carry real values in BOTH modes (Plan Check always runs). `counts.mode` is `'fast' | 'standard'` and lives INSIDE the `counts` object on EVERY return - both the success return and every typed early-error return (`exploration_failed`/`no_approaches`/`architecture_failed`/`judge_failed`) - so a failed fast run is distinguishable from a failed standard run. `decision_log.forced = 'single-candidate'` marks the fast judge-fallback. Anti-silent-fail: each upstream stage that wipes out (explorers / approaches / architects / judges all errored, or plan synth returns null) returns an `error` + null `plan_markdown` instead of fabricating - the dispatcher surfaces it and offers a re-run. `decision_log` = `{votes, totals, rationales, judges}`.

**Judge<->plan coherence:** the Evaluate stage can demolish the judged `winner` (it runs adversarially on the winning architecture); the Plan stage may then revise/abandon it. The plan agent self-reports `delivered_approach` + `diverged_from_winner` + `divergence_reason`, so `winner` (judged) and what the plan actually implements never silently disagree - the dispatcher must surface a divergence prominently. `JUDGE_SCHEMA` is built at runtime with `winner`/`scores[].approach` enum-constrained to the real architected ids (a judge cannot name a phantom approach).

## Constraints (Dynamic Workflow runtime)

- No filesystem - returns data; the command saves the plan.
- No sibling `import` - logic inline.
- No `Date.now()` / `Math.random()` / `new Date()`.
- Top-level `await`/`return` -> not Node-importable; validate via live smoke + a throwaway deterministic check of `aggregateJudges` during development.
