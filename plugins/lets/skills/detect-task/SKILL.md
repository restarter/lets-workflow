---
name: detect-task
description: Internal skill for commands. Detect the active task (any tracker adapter) from the task-state file / git branch name. Do not trigger on user conversation - only when commands need task detection.
user-invocable: false
---

# Detect Active Task

Parse the task-state file / current git branch to find the active task ID (tracker-adapter-aware). Used by commands that need to know which task is in progress.

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## Why This Exists

10+ commands need to detect the active task from branch name. This skill centralizes the logic so branch format changes are updated in one place.

## Detection Flow

### Step 1: Parse Branch Name

```bash
BRANCH=$(git branch --show-current)
```

Extract the task ID from the branch name. Formats:
- `feature/<task-id>-<slug>` - standard LETS branches (main repo)
- `worktree-<task-id>-<slug>` - worktree branches created via `/lets:worktree create` in new-branch mode (the LETS convention)
- `worktree-<custom-name>` - worktree branch without an embedded task ID; use fallback
- any other shape (e.g. `feature/foo`, `bugfix/bar`) - attached existing branch via `/lets:worktree create --attach`; no task ID in the name; use fallback

The `<task-id>` shape is TRACKER-DEPENDENT (the id sits immediately after `feature/` or `worktree-`, up to the first `-<slug>` boundary):
- beads: `<prefix>-<alphanum>[.<number>]` - e.g. `lets-abc`, `lets-abc.1`, `proj-xyz.42`
- a numeric-id tracker: a pure-numeric id - e.g. `48647`, so `feature/48647-<slug>`
- other adapters: the tracker's own id shape

Whatever this parse yields is a CANDIDATE, not an answer: it goes to **the id gate** below, like every other path. On the `branch=<ref>` variant the name being parsed is a pull request's head, which its author chose.

**Do NOT apply the beads `<prefix>-<alphanum>` regex on a non-beads project** - it false-positives on a slug word: `feature/48647-lifecycle-test` makes the beads regex capture `lifecycle-test`, NOT the numeric id `48647`. Match using the ACTIVE tracker's id shape (`{LETS_TRACKER}` from LETS Config). When the branch shape is ambiguous, the `.task-<slug>` file (Step 1.5, authoritative) and the Step 2 search-and-confirm fallback are safer than a branch-name guess.

### Step 1.5: Task-State File (fills the gap when the branch name carries no id)

The `.task-<branch-slug>` file (written by `take-task`) records the CURRENT task - authoritative over the branch name, because a worktree branch is frozen at create time and may host several tasks in sequence (the branch name is only the worktree's home task). Read it after the explicit-arg short-circuit, before the branch-name parse. `{LETS_MERGE_BRANCH}` is from LETS Config:

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current); BRANCH_SLUG=$(echo "$BRANCH" | tr '/' '-')
TASK_FILE="$LETS_PROJECT_ROOT/.lets/sessions/.task-${BRANCH_SLUG}"
CANDIDATE=$(sed -n 's/^task: //p' "$TASK_FILE" 2>/dev/null | head -1)
[ -n "$CANDIDATE" ] && printf 'candidate=%s on_merge_branch=%s\n' "$CANDIDATE" \
  "$([ "$BRANCH" = "{LETS_MERGE_BRANCH}" ] && echo yes || echo no)"
```

This prints a CANDIDATE, not an answer - it is a value read off disk, and nothing may use it before **the id gate** below. `on_merge_branch` only decides whether the liveness probe runs.

**`on_merge_branch=no`** - take this candidate to the gate and stop looking. The branch corroborates the file, and a just-closed id in a worktree is low-severity (the next claim overwrites it).

**`on_merge_branch=yes`** - the file alone cannot tell a live trunk claim from a stale `.task-main` left by a closed task or a main-mode session, so verify it. Run the candidate through **the id gate FIRST**: this probe is itself a tracker verb, so a raw candidate here would cross the boundary before anything checked it - which is exactly the hole the gate exists to close, and the reason the gate is not merely a final step.

```lets-tracker
show task=<TASK_ID from the gate>   # returns {id,title,status,url}; read status
```

`status` is `in_progress` -> use the id. Any other neutral status -> the claim is stale; fall through to the branch-name parse. `show` unsupported (`LETS_TRACKER=none`) or failing -> trust the file and say in one line that liveness could not be checked.

### The id gate (every candidate, exactly once, before any use)

Steps 1, 1.5, the explicit-argument path and a confirmed Step 2 hit all produce a CANDIDATE. None of them returns it. Each hands it here, and this block is the only place that turns a candidate into an id:

```bash
CANDIDATE="{the candidate from whichever step produced it}"
# One class, one place. A tracker verb resolves to a command the MODEL types - on beads a second
# shell hop, `bd show <id>` - so an id crossing that boundary is an unquoted value in someone
# else's shell. `.lets/` is shared by every worktree AND a tracked `.task-<slug>` file materialises
# on checkout, so a candidate read off disk is third-party input, not our own writing.
case "$CANDIDATE" in
  "") echo "TASK_ID=none" ;;
  -*)
    # A leading hyphen passes the class below - hyphen is a legal id character - and then becomes
    # an OPTION at the verb: `bd show --help` is not a lookup. Position matters, not just the
    # alphabet, and no tracker starts an id with a dash.
    echo "NOTE: candidate '${CANDIDATE}' starts with a hyphen - a tracker verb would read it as an option, not an id. Refusing it." >&2
    echo "TASK_ID=none" ;;
  *[!A-Za-z0-9._-]*)
    echo "NOTE: candidate '${CANDIDATE}' is outside the id character class - refusing it. The state file was edited or planted." >&2
    echo "TASK_ID=none" ;;
  *) echo "TASK_ID=$CANDIDATE" ;;
esac
```

**Two rules, not one:** the leading character and the alphabet. The class alone is not enough, because `-` is a legal id character everywhere except position one, where it stops being a value and becomes a flag.

**The class is the tracker platform's contract on id shape**, not a beads detail: `[A-Za-z0-9._-]` covers beads (`lets-abc`, `lets-abc.1`) and a numeric-id tracker, and Step 1 is right that the id's *grammar* is adapter-specific while this is the outer bound every adapter must fit inside. An adapter whose ids need more must widen it here deliberately and say why in its `tracker-<name>.md`, because every character added is a character that reaches a shell the model types. Refusing is loud, so a too-narrow class shows up as a NOTE on every call rather than as silence.

**Why a gate and not a rule.** This used to be a sentence inside the `branch=<ref>` bullet telling every other path, and every consumer, to sanitize what it received. A rule addressed to N sites is followed by N-1 of them, and the one that did not was this skill's own Step 1.5 - the site the rule named by name. A single block with N references is a function call; N copies of a rule is a convention. `TestDetectTaskIdGate` pins the shape, so a path that grows its own `TASK_ID=` echo fails the build rather than the next reader's review.

**Precedence (full):** explicit task-id arg -> `.task-<slug>` `task:` (liveness-validated on the merge-branch via the neutral `show`) -> branch-name id -> a CONFIRMED `search` hit (never an unconfirmed one; skipped entirely when the caller passes `fallback=no` - see Optional arguments). On id-carrying branches `take-task` writes branch + file together so they agree; the file fills the id-less gaps (trunk / custom worktree / attach) and, in a multi-task worktree, reflects the current (switched) task the frozen branch name can't.

**Liveness scope (hot path).** The `show` probe runs ONLY on `{LETS_MERGE_BRANCH}`, the sole place a stale `.task-main` is indistinguishable from a live trunk claim. Off the merge-branch it does NOT run - detect-task is on the hot path of 10+ commands. This IS a cost change for non-beads adapters, which previously skipped the probe entirely: an MCP `show` is a network round-trip, and there is no portable `timeout` on macOS to bound it. We accept that on the merge-branch only, and unmitigated: the alternative is a non-beads project trusting a possibly-stale file forever. (An earlier draft promised an opt-out marker an adapter could set; no such marker was ever defined, and an escape hatch that does not exist is worse than none because it reads as coverage.) Trusting the file elsewhere is safe: `feature/<id>` corroborates via the branch, and a just-closed `task:` in a worktree is low-severity (the next claim overwrites).

### Step 2: Fallback (branch name carries no id)

**Callers listed as no-picker in Step 3 skip this step entirely** - return None instead. They cannot answer the confirmation this step ends in, so running it would spend a `search` (or `list-by-status`) round-trip on a result discarded by construction, on the hot path of 10+ commands.

The board cannot answer "which task is this branch about" - only "which tasks are in progress". On a shared board the first of those is a colleague's. So do not list; search, using what this session already knows.

Build a query from the branch slug with the shape words removed (`feature/`, `worktree-`, `bugfix/`). `feature/planfix-adapter-rules-sync` yields `planfix adapter rules sync`.

```lets-tracker
search query="<terms>"   # returns candidate tasks; NEVER auto-select one
```

Then judge, and say why: "this branch looks like **{title}** (`{id}`) - the branch slug matches its title". Ask the user to confirm before the id is used for anything.

- `search` absent, or it returns nothing -> fall back to `list-by-status status=in_progress` and present that list to choose from. Do NOT take the first.
- Exactly one candidate -> still confirm. One result is not evidence of ownership; it is one result.
- User declines or does not answer -> return None. None is a correct answer here.

**An id that came from this step is NEVER used silently.** That is the whole rule, and it is about provenance rather than identity: an id from the `.task` file or from the branch name is this branch's by construction, so it needs no confirmation; an id from the board is a guess, so it always does. No tracker needs a notion of "me" for this to hold, and on a branch LETS created this step never runs at all.

### Explicit task-id argument (resolve-and-claim)

An explicit argument is authoritative about WHICH task, never about the id's SHAPE - it is a candidate like any other and reaches the caller through **the id gate**.

When the **calling command was invoked with an explicit `<task-id>` argument** (e.g. a session spawned into a fresh worktree with `/lets:plan-workflow <id>`, where the branch is `worktree-<name>` and carries no id): treat that id as **authoritative** - do NOT parse the branch, skip Steps 1-2. The caller then ensures the task is claimed: if the tracker's `show` reports `<id>` not `in_progress`, invoke `Skill(skill: "lets:take-task", args: "<id>")` (in a worktree `take-task` claims + saves the per-branch session-ref without creating a branch). This is what makes an id-accepting command spawn-able into a fresh worktree.

**AUTO MODE carve-out (entry claim only).** This spawn-time claim runs the `set-status=in_progress` verb - a tracker state change AUTO MODE normally gates. But the claim that *starts* an autonomous spawned session is the authorized **entry action** and is exempt (an unattended session cannot ask, and the claim IS the entry). The exemption covers ONLY this entry claim - every later tracker state change (`close`, status flips, `bd dolt push` on beads) stays gated per AUTO MODE.

### Optional arguments: `branch=<ref>` / `fallback=no`

Passed as space-separated `key=value` via the `Skill` invocation's `args`. Both default OFF - a caller that passes neither gets exactly the flow above, unchanged.

- **`branch=<ref>`** - resolve for THAT ref instead of the checked-out one. It substitutes for `$(git branch --show-current)` in Step 1 and Step 1.5; nothing else changes. **Treat the value as UNTRUSTED**: the caller's ref may be a pull request's head, which its author names, and git permits `` $ ` ; | & ( ) `` in a ref. It reaches a filesystem path here, so derive the slug in-shell and pass only this block's OUTPUT downstream - never substitute the raw value into a path or into a tracker verb:

  ```bash
  LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
  SLUG=$(printf '%s' "<ref>" | tr '/' '-')
  case "$SLUG" in ""|*[!A-Za-z0-9._-]*) echo "UNSAFE REF - no task"; exit 0 ;; esac
  printf 'candidate=%s\n' "$(sed -n 's/^task: //p' "$LETS_PROJECT_ROOT/.lets/sessions/.task-$SLUG" 2>/dev/null | head -1)"
  ```

  The guard above is about the REF, which reaches a filesystem path - a different value and a different reason from the id gate. The id this block reads is a candidate like any other and goes to the gate; this path has no sanitizing of its own to remember.

- **`fallback=no`** - skip Step 2 entirely and return None rather than a searched-and-confirmed id.

**Why `fallback=no` exists.** The reason is no longer "the fallback returns a colleague's task" - it now asks before it answers - but that a confirmation is a QUESTION, and a question is unacceptable mid-fan-out, in a `--json` run, or on a read-only orient surface. It also keeps provenance unambiguous at the call site *by construction*: fallback off + an id returned = the id came from the state file or the ref name, never from the board.

### Step 3: Confirming a searched id

A confirmed hit is a candidate too, and leaves through **the id gate** like the rest. Nothing here returns an id directly.

Step 2 always ends in a confirmation - not only when the result was ambiguous. What the confirmation looks like depends on the caller:
- **commit**: AskUserQuestion to pick task or "None"
- **done**: AskUserQuestion to pick task to close
- **review/check/opinion/ask**: skip the tracker comment if ambiguous. review/check additionally pass `fallback=no` when resolving the SPEC, so a searched id can never reach a reviewer prompt
- **note**: AskUserQuestion to pick task to add note to
- **any other caller** (orient, start, end, plan, execute, github-pr): **no picker** - these skip Step 2 outright and take None. They are read/orient surfaces; `orient` in particular declares that it does not select or claim.

**AUTO MODE consequence (a decision, not an accident):** an unattended `/lets:execute --auto` on an attached branch with no id in its name now returns None rather than adopting a board task, and stops saying the task could not be resolved. Adopting a stranger's task unattended is the worse failure.

## Output

ONE echoed line, produced by the id gate and by nothing else:

```
TASK_ID=<id>     # an active task was resolved
TASK_ID=none     # no task detected - the caller decides how to handle it
```

**The guarantee.** Every id this skill returns has cleared `[A-Za-z0-9._-]`. A candidate that has not is refused, loudly, and returned as `none`. This is a property of the skill, not an obligation on its callers: a consumer that passes `TASK_ID` into a tracker verb needs no check of its own, and `take-task` keeping the state file clean on the write side is a courtesy the read side does not depend on.

`none` is a real answer, not an error. It means "no task", and it means the same whether nothing was recorded or what was recorded could not be trusted - the stderr NOTE says which.

## Integration

Internal skill used by most commands that need task detection.
See: `grep -r "detect-task" commands/` for current usage.
