// ── META (required, pure literal) ──
export const meta = {
  name: 'lets-explore',
  description: 'Web-research current standards, fan out brainstorm agents, semantically cluster - all off-context',
  phases: [{ title: 'Web Research' }, { title: 'Ideate' }, { title: 'Cluster' }],
}

// ── ARGS (defensive parse - the runtime may deliver args as a JSON string) ──
const input = typeof args === 'string' ? JSON.parse(args) : (args || {})
const { mode, topic, profile, agents, projectRoot, claudeMd, web: webEnabled } = input
// webEnabled: undefined/true => run the web-research stage (default), false => skip it (--no-web).
// mode is 'explore_idea' here; the asset is written mode-ready so a future brainstorm Review-backlog
// conversion can reuse it with mode 'review_backlog' (only the ideate prompt branches).

// ── SCHEMAS ──
const IDEA_SCHEMA = {
  type: 'object', additionalProperties: false,
  properties: {
    ideas: {
      type: 'array',
      items: {
        type: 'object', additionalProperties: false,
        properties: {
          title: { type: 'string' },
          // category covers both modes (explore: insight/question/approach; backlog: gap/quick_win) so the
          // asset is reusable; the explore prompt only ever emits the first three.
          category: { type: 'string', enum: ['insight', 'question', 'approach', 'gap', 'quick_win'] },
          impact: { type: 'string', enum: ['high', 'medium'] },
          description: { type: 'string' },
        },
        required: ['title', 'category', 'impact'],
      },
    },
  },
  required: ['ideas'],
}

// Web research brief (current community standards), gathered by an off-context subagent that CAN WebSearch.
const WEB_SCHEMA = {
  type: 'object', additionalProperties: false,
  properties: {
    used_web_search: { type: 'boolean' }, // false => the subagent had no web tool; we degrade gracefully
    summary: { type: 'string' },
    standards: { type: 'array', items: { type: 'string' } },
    tools: { type: 'array', items: { type: 'string' } },
    pitfalls: { type: 'array', items: { type: 'string' } },
    sources: {
      type: 'array',
      items: {
        type: 'object', additionalProperties: false,
        properties: { title: { type: 'string' }, url: { type: 'string' } },
        required: ['title', 'url'],
      },
    },
  },
  required: ['used_web_search', 'summary', 'sources'],
}

// Cluster agent groups raw ideas into themes by member index; buildThemes computes attribution (see below).
const CLUSTER_SCHEMA = {
  type: 'object', additionalProperties: false,
  properties: {
    themes: {
      type: 'array',
      items: {
        type: 'object', additionalProperties: false,
        properties: {
          title: { type: 'string' },
          category: { type: 'string', enum: ['insight', 'question', 'approach', 'gap', 'quick_win'] },
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

// ── PURE LOGIC (KEEP IN SYNC with explore.md Phase 4 / Workflow Mode W4) ──
// Primary clustering is the off-context cluster AGENT (semantic grouping); buildThemes turns its
// {themes:[{members:[idx]}]} into ideas with deterministic agents[] + impact. clusterIdeas (title-only)
// is the graceful FALLBACK when the cluster agent errors or returns nothing.
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

// Format the web-research result into a compact brief injected into each ideator's prompt (null if no usable web data).
function formatWebBrief(w) {
  if (!w || !w.used_web_search) return null
  const parts = []
  if (w.summary) parts.push(String(w.summary).trim())
  const bullets = (label, arr) => { if (Array.isArray(arr) && arr.length) parts.push(`${label}:\n` + arr.map(s => `- ${s}`).join('\n')) }
  bullets('Current standards', w.standards)
  bullets('Tools / libraries (current)', w.tools)
  bullets('Community-flagged pitfalls', w.pitfalls)
  if (Array.isArray(w.sources) && w.sources.length) parts.push('Sources:\n' + w.sources.map(s => `- ${s.title}: ${s.url}`).join('\n'))
  return parts.length ? parts.join('\n\n') : null
}

// ── PROMPTS (built from args) ──
function ideatePrompt(webBrief) {
  // explore_idea ideation prompt (mirrors brainstorm.md Explore-idea AGENT_PROMPT_TEMPLATE, schema-ified).
  return `ultrathink

PROJECT_ROOT: ${projectRoot}. Do NOT read or search files outside this directory.

MODE: brainstorm (explore idea)

Explore a specific topic from YOUR area of expertise. You are NOT reviewing code or evaluating a decision - you are thinking through an idea and generating insights, questions, and angles from your domain.

TOPIC: ${topic}

PROJECT RULES (from CLAUDE.md):
${claudeMd}

TOPIC CONTEXT PROFILE (from the scout):
${profile}
${webBrief ? `\nCOMMUNITY STANDARDS (current, from live web research) - ground your ideas in this, and explicitly FLAG where current community practice differs from your prior assumptions; cite a source when it sharpens a point:\n${webBrief}\n` : ''}
Generate 2-4 ideas through your expertise lens: non-obvious insights, risks/opportunities (category "insight"), questions the user should answer before proceeding (category "question"), or concrete approaches/patterns from your domain (category "approach"). Be specific to THIS project, build on prior work in the profile and the community-standards brief, rank by impact (high/medium). Return structured output under "ideas"; do not fabricate if you have nothing useful - return fewer.`
}

// Web research prompt: a neutral subagent (no agentType) that CAN WebSearch/WebFetch gathers CURRENT community
// standards on the topic. It must self-report used_web_search=false rather than fabricate if it has no web tool.
function webResearchPrompt() {
  return `Use the WebSearch tool (and WebFetch on the most relevant results) to gather CURRENT (favor the last ~18 months) best practices and community standards for this topic: ${topic}

If you do NOT actually have a web-search tool available, set used_web_search=false and say so in summary - do NOT fabricate sources or pretend you searched.

Gather and return: a concise "summary" of the current community consensus; "standards" (the most-repeated current best-practice points); "tools" (popular libraries/tools + versions the community uses now); "pitfalls" (mistakes the community currently warns against); and "sources" = 3-8 real URLs you actually retrieved (title + url). Prefer authoritative/recent sources. Keep it tight - this brief is fed to several expert agents.`
}

// Cluster prompt: a neutral subagent (no agentType) groups raw ideas SEMANTICALLY - merging ideas that
// make the same point even when worded differently across agents/categories. It returns member indices;
// buildThemes does the attribution math so the agent can't drift the agents[] or impact.
function clusterPrompt(items) {
  const list = items.map((it, i) =>
    `[${i}] (${it.agent}/${it.category}/${it.impact}) ${it.title}\n    ${String(it.description || '').replace(/\s+/g, ' ').slice(0, 400)}`
  ).join('\n')
  return `You are clustering brainstorm ideas from multiple expert agents exploring this topic: ${topic}

Below are ${items.length} raw ideas, each tagged [index] (agent/category/impact). Group ideas that make the SAME core point into one theme - EVEN IF worded very differently or filed under slightly different categories (this convergence across agents is the signal that matters most). Keep genuinely distinct points separate; do NOT over-merge unrelated ideas.

For each theme return: a clear title; the dominant category (insight | question | approach); a 2-4 sentence merged description synthesizing the members (preserve any nuance or disagreement between them); and "members" = the [index] integers it covers. EVERY input index must appear in exactly one theme - a lone idea with no peers is a theme of one. Do not invent indices outside 0..${items.length - 1}.

IDEAS:
${list}

Return structured output under "themes".`
}

// ── ORCHESTRATION (generate-and-filter, off-context) ──
// Web Research: an off-context subagent (no agentType - the default workflow subagent CAN WebSearch/WebFetch,
// verified) gathers current community standards/best practices. Always-on unless args.web === false. Degrades
// gracefully: subagent reports no web tool -> 'no_tool', errors -> 'failed', empty -> 'empty'; ideate then just
// runs without the brief. The standard (non-workflow) path does the equivalent search in-context (see explore.md).
phase('Web Research')
let webBrief = null, webStatus = 'skipped', webSources = []
if (webEnabled !== false) {
  const w = await agent(webResearchPrompt(), { label: 'web', schema: WEB_SCHEMA }).catch(() => null)
  if (w && w.used_web_search) {
    webBrief = formatWebBrief(w)
    webStatus = webBrief ? 'ok' : 'empty'
    webSources = Array.isArray(w.sources) ? w.sources : []
  } else if (w) { webStatus = 'no_tool' } else { webStatus = 'failed' }
}

phase('Ideate')
const raw = await parallel((agents || []).map(a => () =>
  agent(ideatePrompt(webBrief), { agentType: `lets:${a.name}`, label: `ideate:${a.name}`, schema: IDEA_SCHEMA })
    .then(r => (r && r.ideas ? r.ideas.map(i => ({ ...i, agent: a.name })) : null))))

phase('Cluster')
const responded = raw.filter(Boolean)
const pool = responded.flat()

// Primary path: an off-context cluster agent groups the pool SEMANTICALLY (catches convergence that
// title-only dedup misses - the standard Task path already merges convergent ideas in-context, this
// keeps the two paths equivalent). buildThemes does deterministic attribution. Fall back to title-only
// clusterIdeas if the agent errors / returns nothing, and skip the agent entirely for <=1 idea.
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
  web: { status: webStatus, sources: webSources }, // sources[] = {title,url} for the command to surface
  counts: { agents: (agents || []).length, agents_responded: responded.length, raw_ideas: pool.length, ideas: clustered.length, clustering, web: webStatus },
}
