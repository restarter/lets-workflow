# Protocol: agents - dispatch, reports, members

Loaded by: `/lets:ask`, `/lets:backlog`, `/lets:execute`, `/lets:opinion`, `/lets:plan`, `/lets:research`, `/lets:review`, `/lets:team` - Read before the first dispatch step. Core rules keep the always-on part: never `general-purpose` or another non-`lets:*` subagent type for expert work; directed search vs exploration.

| rule | detail |
|---|---|
| expert dispatch | launching expert agents for `/lets:review`, `/lets:github-pr`, `/lets:opinion`, `/lets:ask`, `/lets:plan`, `/lets:backlog`, `/lets:research` uses ONLY `lets:*` agents (`lets:architect`, `lets:security`, ...) |
| web data-gatherers (carve-out) | `/lets:research`'s per-sub-question web fetchers are data gatherers on the default web-capable subagent, NOT expert dispatch - `lets:*` agents have no web tools (no WebSearch / WebFetch). The `lets:*`-only rule covers research's `lets:skeptic` cross-check, not its web fetch |
| `lets:actor` | a meta-agent: needs an explicit user request + a personality source (URL or file path); never auto-selected; fetch the personality with the `actor-fetch-personality` skill before dispatch |
| reports travel by file | every agent dispatched through the Task / Agent tool gets a `REPORT_FILE` from the `agent-report` skill and writes its report there; the orchestrator reads every report in full and never treats a missing report as "no findings" - it is a loud gap in the output and in the saved artifact. `--workflow` paths are exempt (their agents return through StructuredOutput) |
| members | implementers and standing-team members are spawned, messaged and dismissed only through the `member-run` skill; `lets members` is the registry of who is live |
| tracker data | resolving tracker verbs is ORCHESTRATOR-ONLY - subagents never call tracker verbs (they do not receive the adapter file). A command that needs tracker data inside a subagent prompt pulls it itself and INJECTS it as fenced data. No exceptions, no carve-outs |
