---
name: opinion-workflow
description: Internal Dynamic Workflow asset for /lets:opinion --workflow. Not auto-triggered - it is a workflow script invoked via scriptPath, not a conversational skill. Holds the fan-out -> tally -> conditional-challenge -> synthesize chain that runs off-context.
user-invocable: false
---

# opinion-workflow (Dynamic Workflow asset)

A Dynamic Workflow asset (see `CLAUDE.md` -> "Dynamic Workflow Assets"; `review-workflow` is the reference example). `opinion.workflow.js` is executed by the `Workflow` tool, invoked from `/lets:opinion`'s Workflow Mode via:

```
Workflow({ scriptPath: "${CLAUDE_PLUGIN_ROOT}/skills/opinion-workflow/opinion.workflow.js", args })
```

`${CLAUDE_PLUGIN_ROOT}` is substituted at command-load time, so `opinion.md` carries the literal absolute path. Treat `opinion.workflow.js` as a template the command points at, not a script to reproduce inline.

## What it does (off-context)

A multi-stage chain so per-agent opinions never enter the conversation - only the final aggregate returns:

1. **Opine** - fan out the selected `lets:<name>` experts in parallel (structured `OPINION_SCHEMA`: per-option scores 1-5, recommended option, risks, confidence, reasoning).
2. **Tally** (pure logic) - aggregate -> options ranked by **recommendation plurality** (average score is only a tiebreak), so `leading` = the most-recommended option (the same axis `unanimous`/`weakConsensus` use). Zero opinions -> no leading (`recommendation` surfaces as null, never a phantom first option). The option-id allowlist also feeds `OPINION_SCHEMA`/`CHALLENGE_SCHEMA` as a runtime enum, so agents can only emit valid ids.
3. **Challenge** (CONDITIONAL) - runs ONLY when `weakConsensus` (not unanimous, or any low-confidence vote). Re-dispatch the same experts to argue the strongest case AGAINST the leading option (`CHALLENGE_SCHEMA`). Strong consensus -> skipped.
4. **Synthesize** - `decideRecommendation` applies the asymmetric rule: a strong adversarial signal (majority `would_change_pick` at `high` severity converging on one better option - convergence judged over the **high-severity votes only**, against real option ids so a hallucinated id can't win) flips the pick; otherwise the leading option is confirmed with the strongest counter attached as a caveat. If the challenge was dispatched but every critic errored (zero usable votes), `outcome` is `challenge_failed` (the analog of review's `verify_failed`) - the leading option is surfaced as NOT stress-tested, never silently confirmed.

## `args` contract (built by the command, passed as a JSON value)

| key | type | meaning |
|---|---|---|
| `experts` | `[{name}]` | selected expert short-names (no `lets:` prefix; exclude `actor`) |
| `decision` | string | what is being decided |
| `options` | `[{id, text}]` | the options under consideration (keep `id` simple: `A`, `B`, ...) |
| `constraints` | string | time / compat / legacy / budget context |
| `projectRoot` | string | absolute project root (agents must not read outside it) |
| `claudeMd` | string | CLAUDE.md context |

## Returns

`{ recommendation, outcome, caveat, ranked_options[], positions[], challenge{ran,failed,votes,skipped_reason}, consensus{}, counts{experts,challenged,challenge_failed} }`. `outcome` is `confirmed | flipped | no_clear_winner | challenge_failed`. Claude renders the comparison table + final recommendation from this structured aggregate; the per-agent opinions and challenge votes stay off-context. Two anti-silent-fail guards (both mirror review's `verify_failed`): `counts.experts === 0` (or null `recommendation`) means every opine agent errored - the consumer must NOT render a recommendation, it surfaces "no usable opinions" and offers a re-run; `outcome === 'challenge_failed'` (`challenge.failed` / `counts.challenge_failed > 0`) means the challenge ran but all critics errored - the leading option was NOT stress-tested and must not be presented as confirmed.

## Constraints (Dynamic Workflow runtime)

- No filesystem - the script returns data; `/lets:opinion` renders + records.
- No sibling `import` - all logic stays inline in `opinion.workflow.js`.
- No `Date.now()` / `Math.random()` / `new Date()`.
- Top-level `await`/`return` are used (the runtime wraps the body), so the file is NOT Node-importable - it has no committed unit test; the `tally`/`weakConsensus`/`decideRecommendation` logic is kept in sync with `opinion.md` prose by discipline and validated by the live smoke test (mirroring `review-workflow`). The pure functions CAN be checked deterministically during development by pasting them into a standalone `node:test` file with fixed input vectors - that is a throwaway dev aid, not a repo test.
