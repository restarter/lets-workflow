// ── META (required, pure literal) ──
export const meta = {
  name: 'lets-backlog',
  description: 'Fan out backlog-review agents over the project profile, then semantically cluster - all off-context',
  phases: [{ title: 'Ideate' }, { title: 'Cluster' }],
}

// ── ARGS (defensive parse - the runtime may deliver args as a JSON string) ──
const input = typeof args === 'string' ? JSON.parse(args) : (args || {})
const { profile, agents, projectRoot, claudeMd } = input

// ── SCHEMAS ──
// Backlog idea: category is backlog-semantic (missing/adjust/theme). adjust ideas embed the
// referenced task as **Title** (`task-id`) in description (Task References rule) - no separate field,
// so the pure logic below is category-agnostic.
const IDEA_SCHEMA = {
  type: 'object', additionalProperties: false,
  properties: {
    ideas: {
      type: 'array',
      items: {
        type: 'object', additionalProperties: false,
        properties: {
          title: { type: 'string' },
          category: { type: 'string', enum: ['missing', 'adjust', 'theme'] },
          impact: { type: 'string', enum: ['high', 'medium'] },
          description: { type: 'string' },
        },
        required: ['title', 'category', 'impact'],
      },
    },
  },
  required: ['ideas'],
}

// Cluster agent groups raw ideas into themes by member index; buildThemes computes attribution.
const CLUSTER_SCHEMA = {
  type: 'object', additionalProperties: false,
  properties: {
    themes: {
      type: 'array',
      items: {
        type: 'object', additionalProperties: false,
        properties: {
          title: { type: 'string' },
          category: { type: 'string', enum: ['missing', 'adjust', 'theme'] },
          impact: { type: 'string', enum: ['high', 'medium'] },
          description: { type: 'string' },
          members: { type: 'array', items: { type: 'integer' } }, // indices into the raw idea pool
        },
        required: ['title', 'category', 'members'],
      },
    },
  },
  required: ['themes'],
}

// ── PURE LOGIC (category-agnostic theme clustering; KEEP IN SYNC with
// backlog.md Phase 4 / Workflow Mode) ──
const IMPACT_RANK = { high: 0, medium: 1 }

// Turn the cluster agent's themes (member = pool index) into impact-sorted ideas with deterministic
// agents[] attribution. impact is high if ANY member is high. Any pool idea the agent failed to assign
// becomes its own theme (never drop ideas). Out-of-range / duplicate indices are ignored.
function buildThemes(themes, pool) {
  const assigned = new Set()
  const out = []
  for (const t of (Array.isArray(themes) ? themes : [])) {
    const members = (Array.isArray(t.members) ? t.members : [])
      .filter(i => Number.isInteger(i) && i >= 0 && i < pool.length)
    if (!members.length) continue
    const agentsArr = []
    let high = false
    for (const i of members) {
      assigned.add(i)
      const a = pool[i].agent
      if (a && !agentsArr.includes(a)) agentsArr.push(a)
      if (pool[i].impact === 'high') high = true
    }
    const first = pool[members[0]]
    out.push({
      title: t.title || first.title,
      category: t.category || first.category,
      impact: high ? 'high' : 'medium',
      description: t.description || first.description,
      agents: agentsArr,
    })
  }
  for (let i = 0; i < pool.length; i++) {
    if (assigned.has(i)) continue
    const it = pool[i]
    out.push({ title: it.title, category: it.category, impact: it.impact, description: it.description, agents: [it.agent] })
  }
  return out.sort((a, b) => (IMPACT_RANK[a.impact] ?? 9) - (IMPACT_RANK[b.impact] ?? 9))
}

// FALLBACK: dedupe by normalized title (merge agent attributions), then sort by impact (high first).
function clusterIdeas(items) {
  const byKey = new Map()
  for (const it of items) {
    const key = String(it.title || '').toLowerCase().trim()
    if (!key) continue
    const prev = byKey.get(key)
    if (prev) { if (!prev.agents.includes(it.agent)) prev.agents.push(it.agent) }
    else byKey.set(key, { ...it, agents: [it.agent] })
  }
  return [...byKey.values()].sort((a, b) => (IMPACT_RANK[a.impact] ?? 9) - (IMPACT_RANK[b.impact] ?? 9))
}

// ── PROMPTS (built from args; KEEP IN SYNC with backlog.md Phase 3 prompt) ──
function backlogIdeatePrompt() {
  return `ultrathink

PROJECT_ROOT: ${projectRoot}. Do NOT read or search files outside this directory.

MODE: brainstorm (review backlog)

Review the project's BACKLOG of tasks from your area of expertise. Surface gaps in WHAT'S BEING TRACKED: missing themes, priority imbalances, areas with no tasks but clear need from your domain.

You are NOT reviewing code, counting duplications, finding stale files, or hunting bugs in source. The PROJECT STATE PROFILE below is your primary source. Use bd commands (bd show, bd comments) for task details. Code reads are allowed only as evidence for a backlog observation, never as primary investigation.

PROJECT RULES (from CLAUDE.md):
${claudeMd}

PROJECT STATE PROFILE:
${profile}

Generate 3-7 backlog-level ideas through your domain lens, each categorized:
- "missing": a task/theme that SHOULD exist but does not - include a suggested bd create brief in the description
- "adjust": an existing task needing re-prioritization, scope-change, split, or close - reference it as **Task Title** (\`task-id\`) in the description
- "theme": an observation about backlog distribution, gaps, or drift from project goals
Each idea must be backlog-actionable and specific to THIS project (not "improve testing" but "backlog has 0 tasks tracking auth flow regressions despite 4 recent auth changes"). Rank by impact (high/medium). Return structured output under "ideas"; do not fabricate - return fewer if you have nothing useful.`
}

// Cluster prompt: a neutral subagent (no agentType) groups raw backlog ideas SEMANTICALLY - merging
// ideas that make the same point even when worded differently across agents. Returns member indices;
// buildThemes does the attribution math so the agent cannot drift agents[] or impact.
function clusterPrompt(items) {
  const list = items.map((it, i) =>
    `[${i}] (${it.agent}/${it.category}/${it.impact}) ${it.title}\n    ${String(it.description || '').replace(/\s+/g, ' ').slice(0, 400)}`
  ).join('\n')
  return `You are clustering BACKLOG-REVIEW ideas from multiple expert agents reviewing this project's task backlog.

Below are ${items.length} raw ideas, each tagged [index] (agent/category/impact). Group ideas that make the SAME core point into one theme - EVEN IF worded very differently or filed under slightly different categories (this convergence across agents is the signal that matters most). Keep genuinely distinct points separate; do NOT over-merge unrelated ideas.

For each theme return: a clear title; the dominant category (missing | adjust | theme); a 2-4 sentence merged description synthesizing the members (preserve any task-id references and any nuance/disagreement between them); and "members" = the [index] integers it covers. EVERY input index must appear in exactly one theme - a lone idea with no peers is a theme of one. Do not invent indices outside 0..${items.length - 1}.

IDEAS:
${list}

Return structured output under "themes".`
}

// ── ORCHESTRATION (off-context fan-out -> semantic cluster) ──
phase('Ideate')
const raw = await parallel((agents || []).map(a => () =>
  agent(backlogIdeatePrompt(), { agentType: `lets:${a.name}`, label: `ideate:${a.name}`, schema: IDEA_SCHEMA })
    .then(r => (r && r.ideas ? r.ideas.map(i => ({ ...i, agent: a.name })) : null))))

phase('Cluster')
const responded = raw.filter(Boolean)
const pool = responded.flat()

// Primary: off-context cluster agent groups the pool SEMANTICALLY; buildThemes does deterministic
// attribution. Fall back to title-only clusterIdeas if the agent errors / returns nothing; skip the
// agent entirely for <=1 idea.
let clustered, clustering
if (pool.length > 1) {
  const themes = await agent(clusterPrompt(pool), { label: 'cluster', schema: CLUSTER_SCHEMA })
    .then(r => (r && Array.isArray(r.themes) && r.themes.length ? buildThemes(r.themes, pool) : null))
    .catch(() => null)
  if (themes && themes.length) { clustered = themes; clustering = 'semantic' }
  else { clustered = clusterIdeas(pool); clustering = 'fallback_title' }
} else {
  clustered = clusterIdeas(pool); clustering = 'trivial'
}

// counts.agents_responded === 0 -> every ideator errored; the command must NOT render a fake result.
return {
  ideas: clustered,
  counts: { agents: (agents || []).length, agents_responded: responded.length, raw_ideas: pool.length, ideas: clustered.length, clustering },
}
