// ── META (required, pure literal) ──
export const meta = {
  name: 'lets-plan-workflow',
  description: 'Autonomous planning: explore, propose approaches, architect each, judge against a rubric, evaluate, synthesize a plan - all off-context',
  phases: [
    { title: 'Explore' }, { title: 'Approaches' }, { title: 'Architect' },
    { title: 'Judge' }, { title: 'Evaluate' }, { title: 'Plan' },
    { title: 'Plan Review' }, { title: 'Revise' }, { title: 'Plan Check' }, { title: 'Refine' },
  ],
}

// ── ARGS (defensive parse - the runtime may deliver args as a JSON string) ──
const input = typeof args === 'string' ? JSON.parse(args) : (args || {})
const { goal, rubric, taskContext, projectRoot, claudeMd } = input
// FAST = lean off-context budget (~7 agents): collapse the upstream panels to 1 agent each and
// skip the heavy Plan Review/Revise pass below (the quick Plan Check/Refine still runs).
// Distinct from native /lets:plan --fast (orchestrator-only, NO subagents, in-context).
// Default (no flag) = byte-identical to standard.
const FAST = !!input.fast
const MODE = FAST ? 'fast' : 'standard'  // single source for counts.mode on every return (don't duplicate the ternary)
const FOCUS = (input.focusAreas && input.focusAreas.length)
  ? (FAST ? [{ name: 'general', hint: input.focusAreas.map(f => `${f.name}: ${f.hint || ''}`).join('; ') || 'overall structure and integration points' }] : input.focusAreas)
  : [{ name: 'general', hint: 'overall structure and integration points relevant to the goal' }]
const JUDGES = (input.judges && input.judges.length)
  ? (FAST ? [input.judges[0]] : input.judges)
  : (FAST ? [{ name: 'pragmatist' }] : [{ name: 'pragmatist' }, { name: 'backend' }, { name: 'security' }])
const EXPERTS = (input.experts && input.experts.length)
  ? (FAST ? [input.experts[0]] : input.experts)
  : [{ name: 'pragmatist' }]
const PLAN_REVIEWERS = (input.planReviewers && input.planReviewers.length) ? input.planReviewers : [{ name: 'architect' }, { name: 'pragmatist' }]
const PLAN_CHECKER = input.planChecker || { name: 'pragmatist' }

// ── SCHEMAS ──
const EXPLORE_SCHEMA = {
  type: 'object', additionalProperties: false,
  properties: {
    area: { type: 'string' },
    findings: { type: 'array', items: { type: 'object', additionalProperties: false, properties: { file: { type: 'string' }, role: { type: 'string' } }, required: ['file', 'role'] } },
    integration_points: { type: 'array', items: { type: 'string' } },
    notes: { type: 'string' },
  },
  required: ['area', 'findings'],
}

const APPROACHES_SCHEMA = {
  type: 'object', additionalProperties: false,
  properties: {
    approaches: {
      type: 'array',
      items: {
        type: 'object', additionalProperties: false,
        properties: { id: { type: 'string' }, name: { type: 'string' }, summary: { type: 'string' }, tradeoffs: { type: 'string' }, files_touched: { type: 'array', items: { type: 'string' } } },
        required: ['id', 'name', 'summary'],
      },
    },
  },
  required: ['approaches'],
}

const ARCH_SCHEMA = {
  type: 'object', additionalProperties: false,
  properties: {
    summary: { type: 'string' },
    components: { type: 'array', items: { type: 'object', additionalProperties: false, properties: { path: { type: 'string' }, responsibility: { type: 'string' } }, required: ['path', 'responsibility'] } },
    files_create: { type: 'array', items: { type: 'string' } },
    files_modify: { type: 'array', items: { type: 'string' } },
    data_flow: { type: 'string' },
    pros: { type: 'array', items: { type: 'string' } },
    cons: { type: 'array', items: { type: 'string' } },
    risks: { type: 'array', items: { type: 'string' } },
  },
  // lets-i85v9: only summary is hard-required. files_create/files_modify stay optional - the model
  // omits "conditionally empty" required arrays (e.g. an approach that creates no files) and then
  // thrashes on tool-layer validation (38 rejected StructuredOutput calls in the diagnosed run).
  required: ['summary'],
}

// Built at the Judge phase once the REAL approach ids exist, so `winner`/`scores[].approach`
// are enum-constrained to real ids (a judge can't name a phantom approach). Mirrors the
// option-id enum fix in opinion.workflow.js; aggregateJudges also defends, this is the schema layer.
function buildJudgeSchema(archIds) {
  return {
    type: 'object', additionalProperties: false,
    properties: {
      scores: { type: 'array', items: { type: 'object', additionalProperties: false, properties: { approach: { type: 'string', enum: archIds }, total: { type: 'integer' }, note: { type: 'string' } }, required: ['approach', 'total'] } },
      winner: { type: 'string', enum: archIds },
      rationale: { type: 'string' },
    },
    required: ['scores', 'winner', 'rationale'],
  }
}

const EVAL_SCHEMA = {
  type: 'object', additionalProperties: false,
  properties: {
    findings: { type: 'array', items: { type: 'object', additionalProperties: false, properties: { severity: { type: 'string', enum: ['blocker', 'suggestion', 'nit'] }, issue: { type: 'string' }, suggestion: { type: 'string' } }, required: ['severity', 'issue'] } },
    verdict: { type: 'string' },
  },
  required: ['findings'],
}

const PLAN_SCHEMA = {
  type: 'object', additionalProperties: false,
  properties: {
    plan_markdown: { type: 'string' },
    // The plan agent may override the judged winner when eval blockers warrant; it must self-report,
    // so the return reconciles `winner` (judged) with what the plan actually delivers (no silent override).
    delivered_approach: { type: 'string' },
    diverged_from_winner: { type: 'boolean' },
    divergence_reason: { type: ['string', 'null'] },
  },
  required: ['plan_markdown', 'diverged_from_winner'],
}

// Stages 7-10: review the WRITTEN plan (deep, multi-agent), quick-check it, revise between.
const PLAN_REVIEW_SCHEMA = {
  type: 'object', additionalProperties: false,
  properties: {
    verdict: { type: 'string', enum: ['APPROVED', 'NEEDS REVISION'] },
    findings: { type: 'array', items: { type: 'object', additionalProperties: false, properties: { task: { type: 'string' }, issue: { type: 'string' }, fix: { type: 'string' }, severity: { type: 'string', enum: ['blocker', 'suggestion', 'nit'] } }, required: ['issue'] } },
  },
  required: ['verdict', 'findings'],
}

const PLAN_CHECK_SCHEMA = {
  type: 'object', additionalProperties: false,
  properties: {
    verdict: { type: 'string', enum: ['GOOD', 'REVIEW', 'FIX'] },
    findings: { type: 'array', items: { type: 'object', additionalProperties: false, properties: { lens: { type: 'string' }, tier: { type: 'string', enum: ['BLOCKER', 'SUGGESTION'] }, issue: { type: 'string' }, fix: { type: 'string' } }, required: ['tier', 'issue'] } },
  },
  required: ['verdict', 'findings'],
}

const REVISE_SCHEMA = {
  type: 'object', additionalProperties: false,
  properties: { plan_markdown: { type: 'string' }, change_summary: { type: 'string' } },
  required: ['plan_markdown'],
}

// ── PURE LOGIC (KEEP IN SYNC with plan-workflow.md) ──

// Aggregate judge panel: winner = most winner-votes, tiebreak by summed per-approach totals.
// archIds is the set of REAL architected approach ids - a judge naming an unknown id can't win.
function aggregateJudges(judgeResults, archIds) {
  const totals = {}, votes = {}
  for (const id of archIds) { totals[id] = 0; votes[id] = 0 }
  const rationales = []
  for (const j of judgeResults) {
    for (const s of (j.scores || [])) { if (s.approach in totals) totals[s.approach] += s.total }
    if (j.winner in votes) votes[j.winner]++
    if (j.rationale) rationales.push(j.rationale)
  }
  const ranked = archIds.slice().sort((a, b) => votes[b] - votes[a] || totals[b] - totals[a])
  const winner = judgeResults.length > 0 && ranked.length > 0 ? ranked[0] : null
  return { winner, decision_log: { votes, totals, rationales, judges: judgeResults.length } }
}

// ── PROMPTS (built from args) ──
const HEAD = `PROJECT_ROOT: ${projectRoot}. Do NOT read or search files outside this directory.`
const RUBRIC_BLOCK = `RUBRIC (the steering criteria - weigh every judgment against these):\n${rubric || '(none provided)'}`
const TASK_BLOCK = taskContext ? `TASK CONTEXT:\n${taskContext}\n` : ''

function explorePrompt(f) {
  return `ultrathink

${HEAD}

MODE: plan (exploration)

GOAL: ${goal}
${TASK_BLOCK}
FOCUS AREA: ${f.name} - ${f.hint || ''}

Map this focus area as it relates to the goal: the relevant files and their role, integration points, and existing patterns to follow. Return structured findings. Do NOT propose solutions - just map what exists.`
}

function approachesPrompt(map) {
  return `ultrathink

${HEAD}

MODE: plan (approach synthesis)

GOAL: ${goal}
${RUBRIC_BLOCK}
${TASK_BLOCK}
CODEBASE MAP (from explorers):
${JSON.stringify(map, null, 2)}

Propose 2-4 CONCRETE, DISTINCT implementation approaches grounded in this codebase (not abstract labels). Give each a stable id (A, B, C, ...), a short name, a summary, the key tradeoff, and the files it would touch. Each approach must take a meaningfully different path.${FAST ? `\n\nORDER the approaches BEST-FIRST against the RUBRIC above (the strongest-fit approach as the FIRST array element). In fast mode ONLY the first approach is architected and judged, so its ordering is the actual selection - rank deliberately.` : ''}`
}

function archPrompt(a, map) {
  return `ultrathink

${HEAD}

MODE: plan (architecture for one approach)

GOAL: ${goal}
${RUBRIC_BLOCK}
${TASK_BLOCK}
APPROACH ${a.id} - ${a.name}: ${a.summary}
TRADEOFFS: ${a.tradeoffs || ''}

CODEBASE MAP:
${JSON.stringify(map, null, 2)}

Design THIS approach into a full architecture: components (exact file paths + responsibility), files to create, files to modify, data flow, pros, cons, risks. Follow existing patterns from the map. Be specific. Return files_create: [] (empty array) when the approach creates no new files - NEVER omit the key. Keep every string terse (no prose padding) so the JSON payload stays compact and is never truncated.`
}

function judgePrompt(archs, ids) {
  return `ultrathink

${HEAD}

MODE: plan (judge approaches against the rubric)

GOAL: ${goal}
${RUBRIC_BLOCK}
${TASK_BLOCK}
You are an impartial judge. You did NOT design these. Score EACH architecture (1-10) on how well it satisfies the RUBRIC above - not on personal preference. Then pick ONE winner (use its exact approach id from: ${ids.join(', ')}) and explain why against the rubric.

ARCHITECTURES:
${JSON.stringify(archs, null, 2)}`
}

function evalPrompt(winnerArch) {
  return `ultrathink

${HEAD}

MODE: plan (evaluate the chosen architecture)

GOAL: ${goal}
${RUBRIC_BLOCK}
${TASK_BLOCK}
Evaluate this CHOSEN architecture for real risks, gaps, and proportionality (overengineering / underspec) against the rubric. Report concrete findings with severity. Grep to verify claims about existing code.

CHOSEN ARCHITECTURE:
${JSON.stringify(winnerArch, null, 2)}`
}

function planPrompt(winnerArch, findings) {
  return `ultrathink

${HEAD}

MODE: plan (write the implementation plan)

GOAL: ${goal}
${RUBRIC_BLOCK}
${TASK_BLOCK}
The judged winner is approach ${winnerArch.approach} (${winnerArch.name}). Write a detailed, bite-sized implementation plan in MARKDOWN for it, folding in the evaluation findings below. Each task 15-60 min, exact file paths, complete (no TODO placeholders), a verification step per task, and a commit point per logical unit.

If the evaluation findings (especially blockers) show the chosen architecture is wrong, you MAY revise or abandon it - but you MUST report it: set delivered_approach (the approach id you actually implement, or "revised" for a hybrid/inversion), diverged_from_winner (true if the plan departs from the judged winner ${winnerArch.approach}), and divergence_reason (one sentence, or null). Return the plan as plan_markdown.

CHOSEN ARCHITECTURE:
${JSON.stringify(winnerArch, null, 2)}

EVALUATION FINDINGS TO ADDRESS:
${JSON.stringify(findings, null, 2)}`
}

function planReviewPrompt(planMd) {
  return `ultrathink

${HEAD}

MODE: plan (review the written plan)

GOAL: ${goal}
${RUBRIC_BLOCK}
${TASK_BLOCK}
Review this IMPLEMENTATION PLAN for quality, like /lets:review --plan. Check: every task is 15-60 min; code snippets are complete (no TODO/placeholder); file paths are exact and plausible; every task has a verification step; every logical unit has a commit point; ordering is logical; edge cases covered; the plan honors the RUBRIC. Grep the codebase to verify claims about existing code. Return verdict (APPROVED | NEEDS REVISION) and concrete findings (issue + fix + severity).

PLAN:
${planMd}`
}

function planCheckPrompt(planMd) {
  return `ultrathink

${HEAD}

MODE: plan (quick sanity check)

GOAL: ${goal}
${RUBRIC_BLOCK}
${TASK_BLOCK}
Quick 5-lens sanity pass on this plan, like /lets:check --plan: [Feasibility] can it be built as described? [Completeness] requirements/edge cases covered? [Risk] dependencies/breaking changes? [Scope] proportional, not over/under-specified? [Clarity] can a dev follow without guessing? Report ONLY BLOCKER/SUGGESTION findings (skip nits). Return verdict (GOOD | REVIEW | FIX) + findings (lens + tier + issue + fix).

PLAN:
${planMd}`
}

function revisePrompt(planMd, findings) {
  return `ultrathink

${HEAD}

MODE: plan (apply findings to the plan)

GOAL: ${goal}
${RUBRIC_BLOCK}
${TASK_BLOCK}
Apply these findings to the plan. Preserve everything already correct - change ONLY what the findings require; do not drop tasks, code, or detail. Keep the bite-sized format (tasks 15-60 min, complete snippets, exact paths, per-task verify, commit points). Return the FULL revised plan as plan_markdown plus a one-line change_summary.

CURRENT PLAN:
${planMd}

FINDINGS TO ADDRESS:
${JSON.stringify(findings, null, 2)}`
}

// ── ORCHESTRATION (autonomous multi-stage off-context chain, no user gate between stages) ──
phase('Explore')
const exploreRaw = await parallel(FOCUS.map(f => () =>
  agent(explorePrompt(f), { agentType: 'lets:explorer', label: `explore:${f.name}`, schema: EXPLORE_SCHEMA })))
const map = exploreRaw.filter(Boolean)
if (map.length === 0) {
  return { error: 'exploration_failed', plan_markdown: null, decision_log: null, winner: null, approaches: [], eval_findings: [], counts: { mode: MODE, explorers: 0 } }
}

phase('Approaches')
const approachesRes = await agent(approachesPrompt(map), { agentType: 'lets:architect', label: 'approaches', schema: APPROACHES_SCHEMA })
const approaches = (approachesRes && approachesRes.approaches) ? approachesRes.approaches : []
if (approaches.length === 0) {
  return { error: 'no_approaches', plan_markdown: null, decision_log: null, winner: null, approaches: [], eval_findings: [], counts: { mode: MODE, explorers: map.length, approaches: 0 } }
}

phase('Architect')
// Fast: architect only the top approach (best-first per the fast approachesPrompt ordering) -> 1 architect, not N.
const archInput = FAST ? approaches.slice(0, 1) : approaches
const archRaw = await parallel(archInput.map(a => () =>
  agent(archPrompt(a, map), { agentType: 'lets:architect', label: `arch:${a.id}`, schema: ARCH_SCHEMA })
    .then(r => (r ? { ...r, approach: a.id, name: a.name } : null))))
const archs = archRaw.filter(Boolean)
if (archs.length === 0) {
  return { error: 'architecture_failed', plan_markdown: null, decision_log: null, winner: null, approaches, eval_findings: [], counts: { mode: MODE, explorers: map.length, approaches: approaches.length, architected: 0 } }
}
const archIds = archs.map(a => a.approach)

phase('Judge')
const JUDGE_SCHEMA = buildJudgeSchema(archIds)
const judgeRaw = await parallel(JUDGES.map(j => () =>
  agent(judgePrompt(archs, archIds), { agentType: `lets:${j.name}`, label: `judge:${j.name}`, schema: JUDGE_SCHEMA })))
const judgeResults = judgeRaw.filter(Boolean)
let { winner, decision_log } = aggregateJudges(judgeResults, archIds)
if (winner == null && FAST && archIds.length === 1) {
  // Fast single-candidate: the lone judge errored but there's only ONE architected approach,
  // so there is no choice to fabricate. Proceed with it and mark the fallback observably.
  winner = archIds[0]
  decision_log = { votes: { [winner]: 0 }, totals: { [winner]: 0 }, rationales: [], judges: 0, forced: 'single-candidate' }
}
if (winner == null) {
  // judges all errored AND >1 candidate -> do NOT silently pick; surface it (anti-silent-fail).
  return { error: 'judge_failed', plan_markdown: null, decision_log, winner: null, approaches, eval_findings: [], counts: { mode: MODE, explorers: map.length, approaches: approaches.length, architected: archs.length, judges: 0 } }
}
const winnerArch = archs.find(a => a.approach === winner)

phase('Evaluate')
const evalRaw = await parallel(EXPERTS.map(e => () =>
  agent(evalPrompt(winnerArch), { agentType: `lets:${e.name}`, label: `eval:${e.name}`, schema: EVAL_SCHEMA })))
const evalFindings = evalRaw.filter(Boolean).flatMap(r => r.findings || [])

phase('Plan')
const planRes = await agent(planPrompt(winnerArch, evalFindings), { agentType: 'lets:architect', label: 'plan', schema: PLAN_SCHEMA })
let planMd = planRes ? planRes.plan_markdown : null
if (planMd == null) {
  // Plan synthesis itself wiped out (agent error / null plan_markdown). Surface a typed error like the
  // four upstream stages instead of falling through to the success return with a null plan (anti-silent-fail).
  return { error: 'plan_synthesis_failed', plan_markdown: null, decision_log, winner, approaches, eval_findings: evalFindings, counts: { mode: MODE, explorers: map.length, approaches: approaches.length, architected: archs.length, judges: judgeResults.length } }
}

// Stage 7-8: review the WRITTEN plan (mirror /lets:review --plan) + revise. Never lose planMd on agent error.
// Fast mode SKIPS this heavy pass only; the quick Plan Check/Refine below runs in BOTH modes.
phase('Plan Review')
let reviewFindings = [], reviewVerdict = null, reviewFailed = false, reviewFixed = false, reviewSkipped = false
if (!FAST && planMd) {
  const prRaw = await parallel(PLAN_REVIEWERS.map(r => () =>
    agent(planReviewPrompt(planMd), { agentType: `lets:${r.name}`, label: `planreview:${r.name}`, schema: PLAN_REVIEW_SCHEMA })
      .then(x => (x ? { ...x, agent: r.name } : null))))
  const prRes = prRaw.filter(Boolean)
  reviewFailed = prRes.length === 0
  reviewFindings = prRes.flatMap(r => (r.findings || []).map(f => ({ ...f, agent: r.agent })))
  reviewVerdict = prRes.length === 0 ? null : (prRes.some(r => r.verdict === 'NEEDS REVISION') ? 'NEEDS REVISION' : 'APPROVED')
  if (reviewFindings.length > 0) {
    phase('Revise')
    const rev = await agent(revisePrompt(planMd, reviewFindings), { agentType: 'lets:architect', label: 'revise:review', schema: REVISE_SCHEMA })
    if (rev && rev.plan_markdown) { planMd = rev.plan_markdown; reviewFixed = true }
  }
} else if (FAST && planMd) {
  reviewSkipped = true  // deliberate fast-mode skip (NOT a silent skip / agent error)
}

// Stage 9-10: quick 5-lens check (mirror /lets:check --plan) + refine. Catches regressions the revise introduced.
phase('Plan Check')
let checkFindings = [], checkVerdict = null, checkFailed = false, checkFixed = false
if (planMd) {
  const chk = await agent(planCheckPrompt(planMd), { agentType: `lets:${PLAN_CHECKER.name}`, label: 'plancheck', schema: PLAN_CHECK_SCHEMA })
  checkFailed = !chk
  if (chk) { checkFindings = chk.findings || []; checkVerdict = chk.verdict }
  if (checkFindings.length > 0) {
    phase('Refine')
    const ref = await agent(revisePrompt(planMd, checkFindings), { agentType: 'lets:architect', label: 'refine:check', schema: REVISE_SCHEMA })
    if (ref && ref.plan_markdown) { planMd = ref.plan_markdown; checkFixed = true }
  }
}

return {
  plan_markdown: planMd,
  delivered_approach: planRes ? (planRes.delivered_approach || winner) : null,
  diverged_from_winner: planRes ? !!planRes.diverged_from_winner : null,
  divergence_reason: planRes ? (planRes.divergence_reason || null) : null,
  review_findings: reviewFindings,
  check_findings: checkFindings,
  // Self-repair audit: review pass skipped in fast (review_skipped says why); check pass runs in BOTH modes (real verdict).
  refinement_log: { review_verdict: reviewVerdict, review_findings: reviewFindings.length, review_fixed: reviewFixed, review_failed: reviewFailed, review_skipped: reviewSkipped, check_verdict: checkVerdict, check_findings: checkFindings.length, check_fixed: checkFixed, check_failed: checkFailed },
  decision_log,
  winner,
  winner_name: winnerArch ? winnerArch.name : null,
  approaches: approaches.map(a => ({ id: a.id, name: a.name, summary: a.summary })),
  eval_findings: evalFindings,
  counts: { mode: MODE, explorers: map.length, approaches: approaches.length, architected: archs.length, judges: judgeResults.length, experts: evalRaw.filter(Boolean).length, eval_findings: evalFindings.length, review_findings: reviewFindings.length, check_findings: checkFindings.length },
}
