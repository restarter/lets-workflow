// ── META (required, pure literal) ──
export const meta = {
  name: 'lets-opinion',
  description: 'Fan out expert opinions, conditionally challenge the leading option, synthesize a recommendation - all off-context',
  phases: [{ title: 'Opine' }, { title: 'Challenge' }],
}

// ── ARGS (defensive parse - the runtime may deliver args as a JSON string) ──
const input = typeof args === 'string' ? JSON.parse(args) : (args || {})
const { experts, decision, options, constraints, projectRoot, claudeMd } = input

// Option-id allowlist - constrains agent output (schema enum) AND validates a flip target so a
// hallucinated id can never become the recommendation.
const optionIdList = (options || []).map(o => o.id)
const optionIdSet = new Set(optionIdList)

// ── SCHEMAS ──
const OPINION_SCHEMA = {
  type: 'object',
  additionalProperties: false,
  properties: {
    recommended: { type: 'string', enum: optionIdList },
    scores: {
      type: 'array',
      items: {
        type: 'object',
        additionalProperties: false,
        properties: {
          option: { type: 'string', enum: optionIdList },
          score: { type: 'integer' },
          note: { type: 'string' },
        },
        required: ['option', 'score'],
      },
    },
    risks: { type: 'array', items: { type: 'string' } },
    confidence: { type: 'string', enum: ['high', 'medium', 'low'] },
    reasoning: { type: 'string' },
  },
  required: ['recommended', 'scores', 'confidence', 'reasoning'],
}

const CHALLENGE_SCHEMA = {
  type: 'object',
  additionalProperties: false,
  properties: {
    strongest_counter: { type: 'string' },
    severity: { type: 'string', enum: ['low', 'medium', 'high'] },
    would_change_pick: { type: 'boolean' },
    better_option: { type: ['string', 'null'], enum: [...optionIdList, null] },
  },
  required: ['strongest_counter', 'severity', 'would_change_pick'],
}

// ── PURE LOGIC (KEEP IN SYNC with opinion.md Step 4.6 / Step 5) ──

// Aggregate per option; leading = the option the most experts RECOMMEND. Returns ranked + consensus signals.
function tally(opinions, options) {
  const totals = new Map((options || []).map(o => [o.id, { id: o.id, total: 0, count: 0, recs: 0 }]))
  for (const op of opinions) {
    for (const s of (op.scores || [])) {
      const t = totals.get(s.option)
      if (t) { t.total += s.score; t.count++ }
    }
    const rt = totals.get(op.recommended)
    if (rt) rt.recs++
  }
  for (const v of totals.values()) v.avg = v.count ? v.total / v.count : 0
  // leading = the option most experts RECOMMEND (the single consensus axis that unanimous/weakConsensus
  // and the challenge prompt all speak to); average score is only a tiebreak, then coverage, then total.
  const ranked = [...totals.values()].sort((a, b) => b.recs - a.recs || b.avg - a.avg || b.count - a.count || b.total - a.total)
  // Zero opinions -> NO leading: the recommendation must surface as null, never a phantom first option.
  const leading = opinions.length > 0 && ranked[0] ? ranked[0].id : null
  const runnerUp = opinions.length > 0 && ranked[1] ? ranked[1].id : null
  const forLeading = leading == null ? 0 : ((totals.get(leading) || {}).recs || 0)
  const unanimous = leading != null && forLeading === opinions.length
  const anyLowConf = opinions.some(o => o.confidence === 'low')
  return { ranked, leading, runnerUp, unanimous, anyLowConf, forLeading, n: opinions.length }
}

// Strong consensus -> a challenge adds nothing. Skip it.
function weakConsensus(t) {
  if (t.leading == null) return false
  return !t.unanimous || t.anyLowConf
}

function sev(s) { return s === 'high' ? 3 : s === 'medium' ? 2 : 1 }

function strongestCounter(votes) {
  const ranked = [...votes].sort((a, b) => sev(b.severity) - sev(a.severity))
  return ranked[0] ? ranked[0].strongest_counter : null
}

// Asymmetric: only a strong adversarial signal overturns the leading option;
// weak critique never silently flips it (mirror of review's BLOCKER rule).
function decideRecommendation(t, votes, optionIds, challengeFailed) {
  // Challenge was dispatched (weak consensus) but every critic errored -> the leading option was NOT
  // stress-tested. Surface that distinctly; never report it as adversarially 'confirmed'. This is the
  // challenge-stage analog of review.workflow.js verify_failed (see opinion.md Step 4.6 / W4).
  if (challengeFailed) {
    return { option: t.leading, outcome: 'challenge_failed', caveat: 'Challenge ran but all critic agents errored - the leading option was NOT adversarially stress-tested.' }
  }
  if (votes.length === 0) {
    return { option: t.leading, outcome: 'confirmed', caveat: null } // challenge skipped (strong consensus)
  }
  const wouldChange = votes.filter(v => v.would_change_pick)
  const highChange = wouldChange.filter(v => v.severity === 'high')
  const majorityHighChange = highChange.length * 2 > votes.length
  if (majorityHighChange) {
    // Convergence is judged over the HIGH-severity would-change votes (the majority that opened the gate),
    // not all would-change votes - and only over real option ids (a hallucinated id can never win the flip).
    const betters = highChange.map(v => v.better_option).filter(b => b && optionIds.has(b))
    const counts = {}
    for (const b of betters) counts[b] = (counts[b] || 0) + 1
    const top = Object.entries(counts).sort((a, b) => b[1] - a[1])[0]
    if (top && top[1] * 2 > betters.length && top[0] !== t.leading) {
      return { option: top[0], outcome: 'flipped', caveat: strongestCounter(votes) }
    }
    return { option: t.leading, outcome: 'no_clear_winner', caveat: strongestCounter(votes) }
  }
  return { option: t.leading, outcome: 'confirmed', caveat: strongestCounter(votes) }
}

// ── PROMPTS (built from args) ──
function optionsBlock() {
  return (options || []).map(o => `- ${o.id}: ${o.text}`).join('\n')
}

function opinePrompt() {
  return `ultrathink

PROJECT_ROOT: ${projectRoot}. Do NOT read or search files outside this directory.

MODE: opinion

PROJECT CONTEXT:
${claudeMd}

DECISION: ${decision}
OPTIONS:
${optionsBlock()}
CONSTRAINTS: ${constraints || '(none stated)'}

Evaluate every option from your expert perspective. Return structured output: recommend ONE option (use its EXACT id from the list above), score EVERY option 1-5 (5=best) using its exact id - do not skip any and do not invent ids - each with a short note, list the key risks, your confidence, and your reasoning. Do not hedge - pick a recommendation.`
}

function challengePrompt(leading, leadingText) {
  return `ultrathink

PROJECT_ROOT: ${projectRoot}. Do NOT read or search files outside this directory.

MODE: opinion (adversarial challenge)

The expert panel currently leans toward option ${leading}: ${leadingText}

Make the STRONGEST possible case AGAINST option ${leading} - regardless of what you recommended earlier. What does it miss, when does it fail, what hidden cost or risk does it carry, and is another option actually better?

DECISION: ${decision}
OPTIONS:
${optionsBlock()}
CONSTRAINTS: ${constraints || '(none stated)'}

Return {strongest_counter, severity, would_change_pick, better_option}. would_change_pick=true ONLY if the counter is strong enough that the panel SHOULD move off ${leading}; better_option = the option id you'd move to (or null). Be a fair adversary - do not invent a dealbreaker where none exists; severity=low is a valid honest answer.`
}

// ── ORCHESTRATION (multi-stage off-context chain) ──
phase('Opine')
const rawOpinions = await parallel((experts || []).map(e => () =>
  agent(opinePrompt(), { agentType: `lets:${e.name}`, label: `opine:${e.name}`, schema: OPINION_SCHEMA })
    .then(r => (r ? { ...r, expert: e.name } : null))
))
const opinions = rawOpinions.filter(Boolean)

const t = tally(opinions, options)

// Stage 2: conditional adversarial challenge of the leading option.
phase('Challenge')
let votes = []
let challengeRan = false
if (weakConsensus(t)) {
  challengeRan = true
  const leadingText = (options.find(o => o.id === t.leading) || {}).text || ''
  const rawVotes = await parallel((experts || []).map(e => () =>
    agent(challengePrompt(t.leading, leadingText), { agentType: `lets:${e.name}`, label: `challenge:${e.name}`, schema: CHALLENGE_SCHEMA })
      .then(r => (r ? { ...r, expert: e.name } : null))
  ))
  votes = rawVotes.filter(Boolean)
}
// challenge dispatched but zero usable votes (all critics errored) != challenge skipped on strong consensus.
const challengeFailed = challengeRan && votes.length === 0

const rec = decideRecommendation(t, votes, optionIdSet, challengeFailed)

const ranked_options = t.ranked.map(r => ({
  option: r.id,
  text: (options.find(o => o.id === r.id) || {}).text || '',
  total_score: r.total,
  avg_score: Math.round(r.avg * 10) / 10,
  recommended_by: opinions.filter(o => o.recommended === r.id).map(o => o.expert),
}))

const positions = opinions.map(o => ({
  expert: o.expert,
  recommended: o.recommended,
  confidence: o.confidence,
  risks: o.risks || [],
  reasoning: o.reasoning,
}))

// counts.experts === 0 signals every opine agent errored -> consumer must NOT render a null recommendation.
return {
  recommendation: rec.option,
  outcome: rec.outcome,                 // confirmed | flipped | no_clear_winner | challenge_failed
  caveat: rec.caveat,
  ranked_options,
  positions,
  challenge: { ran: challengeRan, failed: challengeFailed, votes, skipped_reason: challengeRan ? null : 'strong consensus' },
  consensus: { unanimous: t.unanimous, recommended_leading: t.forLeading, n: t.n, any_low_confidence: t.anyLowConf },
  counts: { experts: opinions.length, challenged: votes.length, challenge_failed: challengeFailed ? 1 : 0 },
}
