# Expert agents

LETS ships 15 specialized agents. You don't have to pick them — the commands that use agents (`/lets:review`, `/lets:opinion`, `/lets:ask`, `/lets:plan`, `/lets:backlog`, `/lets:research`, `/lets:team`) analyze the situation and select only the ones that fit. The one place you name an expert yourself is `/lets:ask <expert> <question>`.

| Agent | Expertise | Example trigger |
|-------|-----------|-----------------|
| architect | System design, patterns, SOLID | Structural changes over ~50 lines |
| backend | APIs, business logic, error handling | Controllers, services, routes |
| frontend | UI components, state, accessibility | JS / TS / CSS changes |
| security | Vulnerabilities, auth, crypto | Auth code, tokens, encryption |
| database | Schema, migrations, query optimization | Migrations, ORM, raw queries |
| devops | Docker, CI/CD, deployment | Dockerfiles, CI configs, shell scripts |
| qa | Test strategy, coverage, assertions | Test files, testing patterns |
| compliance | Project standards, conventions | Always included in reviews |
| docs | Documentation sync, README accuracy | Always included in reviews |
| pragmatist | ROI analysis, overengineering detection | Large changes (over ~200 lines) |
| git-historian | Blame analysis, change patterns | Changes to existing code |
| explorer | Codebase mapping, pattern discovery | Used during `/lets:plan` |
| implementer | Full-stack implementation | Used by `/lets:execute` delegated runs and `/lets:team` |
| skeptic | Verifier: tries to refute one finding or claim against the code / sources | The `/lets:review` verify pass and the `/lets:research` cross-check; never picked as a reviewer |
| actor | Any personality from a URL or file | On explicit request |

## How agents work

**Dynamic selection.** Commands analyze the change (or the plan, or the question) and select only the relevant experts. The security agent won't review a docs-only PR; the database agent won't review CSS.

**Tiered scoring.** Findings are `[BLOCKER]` (must fix), `[SUGGESTION]` (should fix), or `[NIT]` (nice to have). Agents are tuned to skip the obvious and focus on what matters — no noise.

**Multiple modes.** Each agent behaves differently depending on context: *review* mode for code review, *opinion* mode for technical decisions, *plan* mode for evaluating an architecture, *brainstorm* mode for ideation, *ask* mode for a direct question.

**Read-only by default.** Agents analyze; they never modify code. The one exception is `implementer`, which has write access: `/lets:execute` hands it plan chunks you review and correct, and `/lets:team` runs it in parallel worktrees. Each agent writes its report to a file under `.lets/reports/` that the command reads in full, so a long report is never cut and a report that never arrives is shown as a gap, not as a clean result; the only file an analyst writes is that report.

**Agents respond in English.** Commands localize their output to your language (set by `LETS_LANGUAGE` — see [configuration.md](configuration.md)); the agents themselves always work in English.

**Which model they run on.** Agents use the model of your session - no agent is pinned to a named model, so a newer model in your session is a newer model everywhere. There are two exceptions. `explorer` is pinned *down* to Sonnet: mapping a codebase is mechanical work, and a cheaper model there costs nothing in quality. `implementer` in a delegated `/lets:execute` run takes the model you pick once at the start of the run (Opus by default, Fable for the hardest chunks, Sonnet or Haiku when the work is mechanical). The verifier (`skeptic`) deliberately stays on the session model - a weaker verifier tends to answer "not real" when it cannot follow a deep finding, and that would drop genuine `[BLOCKER]`s.

**Asking one expert.** `/lets:ask <expert> <question>` goes straight to the agent you name (`security`, `architect`, `docs`, `pragmatist`, ...); bare `/lets:ask` offers the four most relevant to the conversation.

## The actor agent

`actor` is a meta-agent: give it a personality — a URL or a local file — and it adopts that persona, then operates with LETS's structured output. Want a senior iOS developer's take on your Swift code? A UX designer reviewing your components? Point it at their personality and get their domain-specific analysis.

It's never auto-selected — it needs an explicit request and a personality source. You confirm each personality before it's loaded.

## See also

- **[code-review.md](code-review.md)** — agents in `/lets:review` and `/lets:github-pr`
- **[plan-execute.md](plan-execute.md)** — explorer and expert agents in `/lets:plan`, the `implementer` agent in delegated `/lets:execute` runs
- **[parallel-work.md](parallel-work.md)** — the `implementer` agent in `/lets:team`
- **[commands.md](commands.md)** — `/lets:opinion` and `/lets:ask` for decisions and questions
