# Protocol: roles - who does what, and where a decision goes

Loaded by: `/lets:start` (once the task is claimed, and in main mode) - Read "unless its text is in your current context". Every member's spawn brief (`member-run`: `/lets:team` members, `/lets:execute` implementers and checkers) and every `/lets:handoff` brief carry it whole: those agents never load the rules. Core rules keep the always-on part: the who-does-what line in `## Agents & Search`.

The division of labour in every LETS session - solo, orchestrator, worker, standing-team lead or member, an agent reached through `/lets:handoff`. Commands that dispatch these agents own their mechanics; this file is the only statement of who does what.

## Routes

| Route | Trigger | Goes to | Settled when |
|---|---|---|---|
| CLAIM | a decision or a claim ("safe", "green", "not affected") that meets the bar below | `lets:skeptic`, CLAIM mode - it tries to refute it | CONFIRMED with evidence; REFUTED -> withdrawn; UNVERIFIABLE -> reported as `[UNVERIFIED]` |
| PLAN-CHANGE | a design decision: a new approach, or a change to an approved plan's approach, a contract or an allowlist - not the packaging of approved content (writing out a plan's tasks, a chunk brief, an amendment that applies a decided fix) | `lets:architect` designs or reviews it; then CLAIM on the version that carries the decision | that version is CONFIRMED |
| FACT | anything about THIS repo - code, git, logs, docs | directed search (Grep / Glob / Read) when you know what and roughly where; `lets:explorer` to synthesize or compare, for "how does X work", for every affected place, or after 3+ read-then-decide rounds or 3+ files - in doubt, the explorer | the answer cites `file:line`, a sha or a log line - never memory |
| CHECK | mechanical verification: re-run a test, prove a failure pre-existed, a grep scan, a counter or anchor reconciliation | `lets:explorer`, the cheap mechanical checker | its evidence exists; anything non-mechanical it finds -> CLAIM |
| ACCEPT | an implementation is reported done | `lets:implementer` wrote it; its diff is read, CHECK runs, CLAIM runs when the change meets the bar | never merged on the implementer's report alone |
| INDEPENDENT | a second-model view on a plan or a branch | another agent through `/lets:handoff`, given none of this session's conclusions | each of its findings goes through CLAIM |
| OWNER | scope, priority, policy, anything external or irreversible | the user, in this session | the user's own words |
| CROSS-TASK | a decision another task, another session or a shared spec depends on | the orchestrator through `/lets:orc` | its answer informs; the user decides |

## The CLAIM bar

Route a decision or a claim this session makes to the skeptic when ANY of these holds, and never otherwise:

1. It changes what another component, command or session does or receives: a data shape, a CLI flag or `--json` field, an instruction in a command, skill, agent or rules file, a tracker adapter binding, spec wording. A reword that changes no instruction does not count.
2. It touches security, auth, secrets, permissions, or gate / fail-closed logic.
3. Undoing it costs more than reverting a commit: a migration, a published artifact, deleted data, a pushed tag.
4. It is written into an artifact another session reads: a plan, a hand-off brief, a tracker comment, a PR description, a message to the orchestrator.

A decision that meets none is stated with its reason and not routed - asking the skeptic about it is ceremony. Boundary: "the helper is used only by this test file, so the refactor is safe", said in this conversation, meets none; the same sentence written into a plan's Key Decisions meets 4.

The bar governs decisions a session makes. A command's own verification or acceptance pass - `/lets:review` VERIFY on every finding, `/lets:research` RESEARCH-VERIFY on its claims, `/lets:execute` accepting a chunk by its `Risk:` label - runs on its own terms.

## Who dispatches

A command or mode that promises no subagents - `/lets:check`, `/lets:plan --fast`, `/lets:plan --idea` - never dispatches: it names the route, marks the claim `[UNVERIFIED]`, and leaves the dispatch to the user. Otherwise the session dispatches `lets:skeptic`, `lets:explorer` and `lets:architect` itself - they are read-only - and shows the verdict. OWNER and CROSS-TASK never resolve without the user. The judging roles (skeptic, architect) run on the session's model - a cheaper one only when the owner picks it for the run, as `/lets:plan-workflow`'s model panel allows (with a warning); an implementer runs on the model the owner picks for the run.

A team member or an agent reached through `/lets:handoff` dispatches nothing itself: it names the route - to a teammate by `SendMessage` inside a team, otherwise in its report. An agent without `SendMessage` routes by ending its reply with `FOR <role>: <ROUTE> - <ask> - <evidence path>`; whoever spawned it forwards. Keep a reply to about 10 lines and put the detail in a file.

## Across teams and sessions

- Two standing-team leads talk directly through `/lets:orc` about a question that concerns only their two teams - a shared file, an interface between their tasks, a sequencing detail. Anything that changes scope, priority or another team's queue goes through the orchestrator, and the orchestrator is told the outcome of a direct exchange when that outcome is a decision.
- Overlapping work across teams or workers is resolved by rebase, never by coordination: no file freezes, no ownership negotiated per task, no file-list exchanges. The branch that merges second rebases on `origin/<merge>` and resolves the conflicts before its PR; a semantic hazard gets one line inside that rebase (e.g. "keep the other branch's restructuring, do not reintroduce text it removed").
