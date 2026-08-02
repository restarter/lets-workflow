---
name: tracker-fake
version: 0.0.0
---

<!-- TEST FIXTURE, not a shipped adapter. It cannot live in plugins/lets/rules/:
     TestShippedTrackers_MatchOnDisk requires every adapter file there to appear in
     the shippedTrackers whitelist, so a fixture would fail it. It exists because the
     shipped set is beads (supports everything) + none (supports nothing), which
     leaves the PARTIAL adapter - some verbs absent, a renamed field, an optional
     status carried - exercised by nothing. Every contract assertion should hold here
     too; if one only passes on beads, it is pinning beads rather than the contract. -->

# Tracker adapter: fake (fixture x nothing)

A deliberately partial adapter. It supports the CORE verbs, marks two OPTIONAL verbs
absent, renames one field natively, and carries the optional `in_review` status -
the combination no shipped adapter has.

## Neutral statuses

`open`, `in_progress`, `closed`, `in_review`. No `blocked`.

`in_review` is carried, so a caller may advance a task there after opening a PR. The
terminal is process-gated: `close` from `in_progress` is not a legal edge, so the
binding advances to `in_review` and says so rather than failing.

## Capabilities + bindings

| verb | tier | supported | binding |
|------|------|-----------|---------|
| create         | CORE | yes | accepts: `title`, `description`, `priority`→`severity`. POST /tasks; returns `{id, url}` |
| show           | CORE | yes | returns: `id`, `title`, `status`, `url`, `description`. GET /tasks/<id>; status mapped to a NEUTRAL name |
| comment-add    | CORE | yes | POST /tasks/<id>/comments; empty body → HARD-FAIL, do not submit |
| set-status     | CORE | yes | PATCH /tasks/<id>; a status outside the four above is unsupported - skip and report nothing changed |
| close          | CORE | yes | PATCH /tasks/<id> to the agent-legal terminal; returns `in_review`, NOT `closed` - the real terminal is reachable only through a human QA step outside LETS |
| comment-list   | OPT  | yes | GET /tasks/<id>/comments |
| list-by-status | OPT  | yes | GET /tasks?status=<neutral> (status returned NEUTRAL) |
| search         | OPT  | no  | absent |
| ready/stats    | OPT  | no  | absent |
| label          | OPT  | no  | absent |
| assignee       | OPT  | yes | PATCH /tasks/<id> assignee |
| set-field      | OPT  | yes | accepts: `description`. PATCH /tasks/<id> |

## Degradation

`search`, `ready/stats` and `label` are absent: a caller continues and tells the user
the capability is unavailable - `detect-task`'s fallback drops to `list-by-status`
and confirms, `/lets:backlog` omits the totals line, `create-task` proposes a label
by hand. A CORE verb that fails at runtime HARD-FAILs loud; `close` returning
`in_review` is NOT a failure, it is the declared outcome, and the caller reports a
handoff rather than a close.
