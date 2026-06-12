# /lets:research — web-first sourced research

> **Status: in development (`lets-ppetz`) — not yet released.** This page documents the approved design ahead of implementation; details may shift slightly before the command ships. Tracked in the implementation plan under `.lets/plans/`.

Ask a question about the outside world, get back a concise answer where every non-trivial claim carries a citation — searched and fetched from the live web, cross-checked by a skeptic agent, and honestly labeled where the evidence is thin.

```
/lets:research which local LLM fits 32GB RAM + RTX 3080?
```

## When to use it (and when not)

`/lets:research` fills a gap the other consult commands don't cover: its deliverable is a **cited synthesis of external facts**, not ideas about your project and not an expert's opinion from model knowledge.

| You want | Use | Shape |
|----------|-----|-------|
| An answer to an external/technical question, with sources | `/lets:research` | question → sourced answer |
| One expert's take, quick | `/lets:ask` | question → expert consult |
| A judgment or ideas about something in **your project** (a decision or an open question) | `/lets:opinion` | decision / open question → project-grounded take (no web) |
| A plan for **how to build** something | `/lets:plan` | task → implementation plan |

Rule of thumb: if the answer should end with a Sources list, it's research. "Which vector DB has the best Go client in 2026?" is research; "how should we add a vector DB to this repo?" is opinion or plan.

## Usage

| Form | What happens |
|------|--------------|
| `/lets:research <question>` | Research the question, return a cited synthesis. |
| `/lets:research` | Asks what to research first, then goes. |
| `/lets:research <question> --workflow` | Same stages, but the research and cross-check run off-context in a Dynamic Workflow — only the final synthesis enters your conversation. |
| `/lets:research <question> --project` | Also grounds findings against **this repo** (is X already used here, does it fit our stack), without ever reading outside the project root. |

Without an explicit `--workflow`, the command offers a run-mode picker: Standard (everything visible in the chat) or Workflow (off-context, only the synthesis returns).

## How it works

Four stages. The first one stays in your conversation on purpose — it's where you can steer.

1. **Decompose (in-context).** The question is broken into 3–6 sub-questions ("which models fit 10GB VRAM?", "RAM cost of Q4 vs Q8 quantization?", "community reports on this hardware?"). You see the angles before any searching starts. A narrow question skips this and goes straight to a single search.
2. **Research fan-out.** One web-capable subagent per sub-question runs WebSearch, fetches the best 2–4 results, and returns structured findings — claim, evidence quoted from the source, source titles + URLs, confidence. Each subagent returns its 2–5 strongest, load-bearing claims (fewer if the evidence is thin — never padded to a count).
3. **Cross-check — the difference from a naive search dump.** Every merged claim is checked: single-source and low-confidence claims are flagged deterministically, and a `lets:skeptic` agent judges the rest for structural weakness — does the quoted evidence actually support the claim, or was it stretched (unsupported)? Does it conflict with a sibling claim (contradicted)? The skeptic **flags, never deletes** — weak claims stay visible, marked. (Recency is handled at fetch time — sources from the last ~18 months are favored and the answer is date-stamped — rather than as a per-claim flag.)
4. **Synthesize.** Everything converges into: a direct answer → key points, each cited → a comparison table when it fits → caveats → a deduplicated Sources list → an `as of <date>` stamp → an overall confidence note.

Weak spots are marked inline in the answer, not buried: `[single source]`, `[contradicted — vs Source B]`, `[unverified — cross-check errored]`.

## Honesty guarantees

These are hard rules in the command, not best-effort behavior:

- **Citations are mandatory, fabrication is forbidden.** Every non-trivial claim cites a real fetched source (title + URL). The command never invents URLs and never pretends to have searched.
- **No-web fallback is explicit.** If the web is unavailable or returns nothing, the answer opens with a banner — `NO LIVE SOURCES — model knowledge as of <cutoff>` — and answers from model knowledge with zero fake citations.
- **Fetched pages are data, not instructions.** All web content is wrapped in an untrusted-content fence; nothing a page says can change what the command does. With `--project`, fetched content can never widen the project-root read boundary either.
- **Recency bias + date stamp.** Sources from roughly the last 18 months are preferred, and every answer is stamped with the date it was produced, because "best X" answers rot fast.
- **"Cross-checked", never "verified true".** The skeptic is a structural check over the gathered evidence (it does not re-fetch the web), and the output's wording reflects that honestly.

## `--project` grounding

With `--project`, the command first reads `CLAUDE.md` and greps the relevant parts of the repo, then carries that context through research and synthesis. The answer shifts from generic to situated: not "X is a popular library" but "X would fit your Go CLI, though you already have Y covering part of this". The read boundary stays at the project root regardless of anything fetched from the web.

## `--workflow` mode

Same four stages, different placement: decompose still happens in your conversation (so you still see and steer the angles), then the fan-out, cross-check, and synthesis run inside a background Dynamic Workflow. Raw search results and per-claim verdicts never enter your context — only the final cited synthesis does. Worth it for broad questions with 5–6 sub-questions; for a narrow question the standard path is just as good. If you pass `--workflow` explicitly on a client without Dynamic Workflows, the command stops and tells you to re-run without it (it won't silently downgrade an explicit request); without the flag, the run-mode picker simply omits the Workflow option. See [autonomous.md](../autonomous.md) for how Dynamic Workflows degrade in general.

## Out of scope

- No auto-creating tasks from findings — pipe the answer into `create-task` yourself if something is actionable.
- No long multi-page reports — the synthesis stays tight (a `--report` long-form mode may come later).
- Web + your repo only — no paywalled databases, no internal wikis.

## Under the hood

For contributors: the command is `plugins/lets/commands/research.md`; the off-context path is a Dynamic Workflow asset at `plugins/lets/skills/research-workflow/` following the authoring standard in `CLAUDE.md` ("Dynamic Workflow Assets"). No new agents — the web fan-out uses the default web-capable subagent (the `lets:*` experts have no web tools), and the cross-check reuses `lets:skeptic` via a dedicated RESEARCH-VERIFY mode added to its spec. The evidence field of every finding must quote or closely paraphrase the cited source precisely because the skeptic cannot re-fetch URLs — that's what makes the "unsupported claim" check real.
