---
description: Full code review with dynamic agent selection (up to 12 specialized agents). Analyzes changes first, selects relevant experts. Also reviews implementation plans.
argument-hint: "[PR-url-or-number|--local|--staged|--last-commit|--branch|--plan|--file <path>] [--json] [--workflow]"
---

# Full Code Review

Comprehensive code review with dynamic agent selection based on change types. Up to 12 specialized agents, tiered severity scoring. Works with:
- GitHub PRs (posts comment to PR)
- Local changes (saves to file)
- Implementation plans (reviews `.lets/plans/` files)

> **IMPORTANT:** If the spec below invokes any deferred tool (e.g. `AskUserQuestion`), you MUST load and call it as specified. Never skip the call, never substitute a default answer of your own — the tool invocation is part of the contract. This is critical.

## Usage

```bash
/lets:review                     # Interactive - asks what to review
/lets:review <PR-url-or-number>  # GitHub PR
/lets:review --local             # Uncommitted changes
/lets:review --staged            # Staged changes only
/lets:review --last-commit       # Last commit
/lets:review --branch            # Full branch vs $LETS_MERGE_BRANCH (three-dot, like a PR)
/lets:review --plan              # Review latest plan in .lets/plans/
/lets:review --plan <path>       # Review specific plan file
/lets:review --file <path>       # Review an existing file (full content, not diff)
/lets:review --workflow          # Run via Dynamic Workflow (off-context fan-out); combinable with any code mode + --json
```

## Step 1: Determine Review Mode

**If argument provided:**
- PR URL/number -> GitHub PR mode
- `--local` / `--staged` / `--last-commit` / `--branch` -> Local mode
- `--plan` / `--plan <path>` -> **Plan review mode** (skip to Plan Review section below)
- `--file <path>` -> **File review mode** - reviews entire file content, not a diff

**If no argument**, use **AskUserQuestion**:

```
AskUserQuestion(
  questions=[{
    question: "What are we reviewing?",
    header: "Target",
    options: [
      { label: "Local changes", description: "Uncommitted changes in working tree" },
      { label: "Branch", description: "Full branch vs {LETS_MERGE_BRANCH} (three-dot, like a PR)" },
      { label: "Plan", description: "Review implementation plan from .lets/plans/" },
      { label: "Last commit", description: "Review the most recent commit" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Local changes** -> local mode with `git diff`
- **Branch** -> local mode with `git diff {LETS_MERGE_BRANCH}...HEAD` (run guards from Step 2 first)
- **Last commit** -> local mode with `git diff HEAD~1`
- **Plan** -> skip to Plan Review section
- **Other** (free text) -> treat as PR number or URL, use GitHub PR mode

Note: `--staged` is flag-only (not in the interactive menu). Use `/lets:review --staged` directly.

**If plan mode selected:** skip to **Plan Review** section below.

### JSON output flag

If `--json` is present alongside any mode:
- Save review output as structured JSON instead of markdown
- File: `.lets/reviews/{date}-{mode}.json` (e.g., `2026-02-26-PR-42.json`, `2026-02-26-local-review.json`, `2026-02-26-branch-review.json`)
- Skip markdown report generation (Step 8)
- Skip GitHub PR comment posting (Step 9) - JSON mode implies the caller handles output

### Workflow execution flag

If `--workflow` is present alongside any **code** mode (`--local` / `--staged` / `--last-commit` / `--branch` / `<PR>` / `--file`):
- Run the agent fan-out + aggregation inside a Dynamic Workflow instead of launching agents via the Task tool (Step 5/6).
- Combinable with `--json` (the workflow returns the aggregate; Claude still writes the JSON file in Step 8.5).
- NOT supported with `--plan` (the plan-review path is a separate follow-up) - if both are present, tell the user `--workflow` does not apply to plan review yet and run the standard plan-review path.

### Choosing the execution path (interactive)

When `--workflow` was NOT explicitly passed, decide the execution path as follows - **for code modes only** (never for `--plan`):

- If the `Workflow` tool is **not** available this session -> silently use the standard Task-based path. Do NOT show the option (no clutter for users without Dynamic Workflows).
- If the `Workflow` tool **is** available -> ask the user which path via `AskUserQuestion`:

```
AskUserQuestion(
  questions=[{
    question: "How should I run this review?",
    header: "Run mode",
    options: [
      { label: "Standard (Recommended)", description: "Agents via Task tool - full per-agent reports visible inline" },
      { label: "Workflow", description: "Dynamic Workflow - off-context fan-out, only the aggregate returns" }
    ],
    multiSelect: false
  }]
)
```

**Handle response:**
- **Standard** -> standard path (Step 5 as written).
- **Workflow** -> treat as if `--workflow` was set: go to `## Workflow Mode (--workflow)` after agent selection.
- **Other** (free text) -> if it names a path, honor it; otherwise default to Standard.

For the **bare no-argument invocation**, combine this with the Step 1 "What are we reviewing?" question in a single `AskUserQuestion` call (two questions) so the user answers target + run mode at once - but only include the run-mode question when the `Workflow` tool is available.

An explicit `--workflow` flag always wins over this prompt (no question asked).

## Step 2: Get Changes

### For GitHub PR:

```bash
gh pr view <PR> --json state,isDraft,author,title,body,additions,deletions,changedFiles,number,headRefName,headRefOid
```

`number` is the normalized PR number (Step 1 accepts a URL too) - use it wherever a number is needed. `headRefName` / `headRefOid` feed the SPEC resolution (Step 3) and the branch gate (Step 2.5).

**Skip if:** PR is closed, draft, trivial, or already reviewed.

```bash
gh pr diff <PR>
```

### For Local Changes:

```bash
# Uncommitted
git diff

# Staged only
git diff --staged

# Last commit
git diff HEAD~1

# Full branch vs merge branch (three-dot, merge-base diff)
git diff {LETS_MERGE_BRANCH}...HEAD
```

For `--branch` mode, run these guards first. **If any guard prints output, STOP the entire command, surface that message to the user, and skip remaining steps** — bash `exit` only terminates the spawned shell, not the orchestrator's command flow.

```bash
[ -z "{LETS_MERGE_BRANCH}" ] && echo "LETS_MERGE_BRANCH is not configured. Edit .lets/.env or run /lets:init." && exit
CURRENT=$(git branch --show-current)
[ "$CURRENT" = "{LETS_MERGE_BRANCH}" ] && echo "On {LETS_MERGE_BRANCH} - nothing to review against itself." && exit
git rev-parse --verify "{LETS_MERGE_BRANCH}" >/dev/null 2>&1 || { echo "Merge branch '{LETS_MERGE_BRANCH}' not found locally. Run: git fetch origin {LETS_MERGE_BRANCH}:{LETS_MERGE_BRANCH}"; exit; }
[ "$(git rev-list --count {LETS_MERGE_BRANCH}..HEAD)" = "0" ] && echo "Branch has no commits ahead of {LETS_MERGE_BRANCH}." && exit
```

If no changes found, inform user and exit.

### For File Review (--file):

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
cat "$LETS_PROJECT_ROOT/{path}"
```

Read the entire file. Use file content as "diff" for agents. No git diff needed - this reviews existing code, not changes.

If file not found, inform user and exit.

## Step 3: Gather Context

### For GitHub PR:

```bash
# Get CLAUDE.md files
cat CLAUDE.md 2>/dev/null | head -200

# Check for CLAUDE.md in modified directories
gh pr diff <PR> --name-only | xargs -I{} dirname {} | sort -u | xargs -I{} cat {}/CLAUDE.md 2>/dev/null

# Get PR summary
gh pr view <PR> --json title,body,commits
```

### For Local Changes:

```bash
# Get CLAUDE.md
cat CLAUDE.md 2>/dev/null | head -200

# Get changed file list
git diff --name-only  # (or --staged, HEAD~1, or {LETS_MERGE_BRANCH}...HEAD for --branch)
```

For `--branch` mode, also surface the commit list and stat so the reviewing agents see branch scope:

```bash
git log {LETS_MERGE_BRANCH}..HEAD --oneline     # two-dot: commits unique to HEAD
git diff {LETS_MERGE_BRANCH}...HEAD --stat      # three-dot: merge-base diff (PR-equivalent)
```

### Resolve the task SPEC (all modes)

Reviewing agents must know what the change is SUPPOSED to do. Without it, planned-but-not-yet-wired work reads as dead code and gets flagged as scope creep at BLOCKER severity - confidently wrong, which is worse than a miss.

**Resolve the task id:**

- **Local modes** (`--local` / `--staged` / `--last-commit` / `--branch`): `Skill(skill: "lets:detect-task")`.
- **PR mode**: in order - (1) the `.lets/sessions/.task-<slug>` file for `headRefName` (slug = `headRefName` with `/` replaced by `-`); (2) parse the id out of `headRefName` using detect-task's Step 1 rule for the ACTIVE tracker's id shape. The `.task` file wins because a branch name is frozen at creation while the file tracks the current task. Do NOT fall through to detect-task's `list-by-status in_progress` fallback: the reviewer usually sits on a different branch, and on a shared board that returns a colleague's task - injecting the WRONG spec is strictly worse than none.
- **`--file` mode**: no id, no spec. The file under review is usually unrelated to the active task; telling an agent it is "planned work" would be a new confident-wrongness vector.

**Validate before use.** The extracted id MUST match `^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`. A branch name on a fork PR is written by the PR author and git refs permit `` $ ( ) ` ; | & < > ``; the id crosses into a tracker verb that resolves to a shell command. No match → no spec. Never pass an unvalidated string to a verb or echo it into a bash block.

**Fetch the spec** (skip when no id resolved):

```lets-tracker
show task=<task-id>   # returns {id, title, status, url, description}
```

`{spec}` is the returned `description`, capped at ~150 lines AND ~8000 characters (the no-hard-wrap rule means one paragraph is one line, so a line cap alone is unbounded). It is repeated into every agent prompt and every skeptic prompt - do NOT also call `comment-list`.

**`{spec}` is EMPTY (not a sentinel string) whenever any of these hold** - each renderer prints its own "unavailable" text:
- no id resolved, or the id failed validation;
- `show` failed (binding unavailable / task not found);
- `show` succeeded but `description` is empty or absent. The neutral contract makes `description` OPTIONAL, and on `LETS_TRACKER=none` `show` is a documented no-op - both are "no spec", not an error.

**PR-mode fallback before giving up:** if `{spec}` is still empty, use the PR's own `title` + `body` (already fetched in Step 2) as `{spec}`, and note in the report that the spec came from the PR body rather than the tracker. Many projects put the spec there.

Record `spec_available` (true/false) and `spec_source` (`tracker` | `pr-body` | `null`) - Steps 8.5 and 9 both report them. Carry the resolved id forward; Step 10 reuses it.

## Step 4: Analyze Changes & Select Agents

**BEFORE launching agents, analyze the diff to determine which experts are needed.**

### 4.1 Detect Change Types

Scan the diff for file patterns:

| Pattern | Change Type | Relevant Agents |
|---------|-------------|-----------------|
| `Dockerfile`, `docker-compose`, `Makefile` | Infrastructure | devops, security, docs |
| `*.php` (Laravel/Yii) | Backend PHP | backend, security, architect |
| `*.py`, `*.go`, `*.java`, `*.rb` | Backend | backend, security, architect |
| `*.ts`, `*.tsx`, `*.vue`, `*.jsx` | Frontend | frontend, security, docs |
| `*.kt`, `*.swift` | Mobile | backend, security, architect |
| `config/*`, `.env*`, `*.yml` | Configuration | security, devops, docs |
| `migrations/*`, `*.sql` | Database | database, security, architect |
| `tests/*`, `*.test.*`, `*.spec.*` | Tests | qa, architect |
| `commands/*.md`, `agents/*.md`, `hooks/*.md` | Skill/Command | compliance, docs, pragmatist |
| `docs/*`, `*.md`, `CLAUDE.md` | Documentation | docs only |
| `package.json`, `composer.json` | Dependencies | security, devops |

### 4.2 Agent Catalog

| Agent (subagent_type) | When to Include | Skip If |
|----------------------|-----------------|---------|
| `lets:compliance` | ALWAYS | Never skip |
| `lets:backend` | Any code changes | Docs-only changes |
| `lets:security` | Code, config, deps | Docs-only, test-only |
| `lets:architect` | Code changes > 50 lines | Small fixes, docs |
| `lets:git-historian` | Changes to existing code | New files only |
| `lets:docs` | ALWAYS | Never skip |
| `lets:devops` | Docker, CI, Makefile, scripts | App code only |
| `lets:database` | Migrations, queries, ORM | No DB changes |
| `lets:frontend` | JS/TS/CSS changes | Backend only |
| `lets:qa` | Test file changes | No test changes |
| `lets:pragmatist` | Large changes (> 200 lines) | Small changes |
| `lets:actor` | Explicit user request only | Always (never auto-selected) |

**Actor note:** Actor is never auto-selected. When user explicitly requests it, invoke `Skill(skill: "lets:actor-fetch-personality", args: "<personality-source-from-user>")` to fetch the personality. Pass the returned `PERSONALITY:` block in the actor's Task prompt only.

### 4.2.1 File Mode Adjustments

When reviewing a single file (`--file` mode), adjust agent selection:

- **Skip git-historian** - no diff context, git blame adds noise for full-file review
- **Skip systemic pattern check instruction** - not comparing against a diff baseline, remove SYSTEMIC PATTERN CHECK from agent prompts
- **Adjust pragmatist threshold** - include for files >100 lines (not ">200 lines changed")
- **Display header** - show `Reviewing: {filename} ({N} lines)` instead of `Changes detected:`

### 4.3 Select Agents

Based on analysis, output:

```
## Agent Selection

Changes detected:
- [x] PHP backend code (15 files)
- [x] Database migrations (2 files)
- [ ] Frontend code
- [x] Docker config (1 file)
- [ ] Documentation

Selected agents (7 of 12):
1. compliance (always)
2. backend (PHP code + bug scanning)
3. security (PHP + DB + Docker)
4. architect (>50 lines changed)
5. database (migrations detected)
6. devops (Docker changes)
7. pragmatist (>200 lines total)

Skipped:
- frontend (no frontend changes)
- qa (no test files)
- git-historian (mostly new files)

> Subagents have a separate rate limit - no cost to your conversation.
```

**For `--file` mode**, adjust the display:

```
## Agent Selection

Reviewing: `{file path}` ({N} lines, {extension} file)

Selected agents ({M}):
1. compliance (always)
2. backend (PHP file)
3. security (source code)
4. architect (>50 lines)

Skipped for file mode:
- git-historian (no diff context)

> Subagents have a separate rate limit - no cost to your conversation.
```

**RULE: Default is INCLUDE. Only skip if clearly irrelevant.**

## Step 5: Launch Selected Agents (Parallel)

**If `--workflow` was parsed:** skip this step and Step 6 - go to the `## Workflow Mode (--workflow)` section below, then resume at Step 6.5. The rest of Step 5 is the standard Task-based path.

**CRITICAL:** Launch ALL selected agents in a SINGLE message with multiple Task tool calls.

For each selected agent, use the Task tool with:

- **subagent_type**: The agent identifier from the catalog (e.g., `lets:architect`)
- **prompt**: Provide review context (see below)
- Agents inherit the session model (no per-agent model pins)

### Task Prompt Template

Each agent receives this context in their task prompt. Agents define their own expertise, tiered scoring, output format, and mode-specific behavior in `agents/*.md` - do NOT duplicate those in the prompt.

```
ultrathink

PROJECT_ROOT: {LETS_PROJECT_ROOT from LETS Config}. Do NOT read or search files outside this directory.

MODE: review

Review the following code from your expert perspective.

SYSTEMIC PATTERN CHECK:
For each finding, grep the codebase to check if the same pattern exists in other files.
If the pattern is used in 2+ other places - it's a systemic issue, not a PR bug.
Still report it, but:
- Prefix with [SYSTEMIC]
- Note how many other files follow the same pattern
- Frame as "project-wide tech debt" not "bug in this PR"
- Downgrade tier by one level (e.g. [SUGGESTION] becomes [NIT])

CLAUDE.MD RULES:
{claude_md_content}

CHANGED FILES:
{list of modified files with stats, or single file path for --file mode}

CODE:
{diff_content, or full file content for --file mode}

```

## Workflow Mode (--workflow)

Runs when `--workflow` was parsed in Step 1 (code modes only). Replaces Step 5 (Task launch) and Step 6 (aggregation): both happen inside the Dynamic Workflow so the per-agent reports never enter this conversation - only the aggregated object returns.

### W1: Preflight

If the `Workflow` tool is NOT available this session, STOP the workflow path and tell the user:

> `--workflow` needs Claude Code >= 2.1.154 on a paid plan (Dynamic Workflows is a research preview). Re-run without `--workflow` to use the standard agent flow.

Do NOT silently fall back - the user opted in explicitly; surface the gap and let them choose.

### W2: Build args

Use the agents SELECTED in Step 4 (do not re-select). **Exclude `lets:actor`** from workflow mode - the skeleton does not inject the `PERSONALITY:` block, so an actor selection would lose its identity. If the user explicitly selected actor, tell them actor is not supported with `--workflow` yet and run the standard Task-based path instead.

Construct the `args` object:

```
{
  agents: [ { name: "compliance" }, { name: "security" }, ... ],  // selected agent short-names (no "lets:" prefix)
  mode: "PR-42" | "local-review" | "branch-review" | "file",
  projectRoot: "{LETS_PROJECT_ROOT from LETS Config}",
  claudeMd: "{CLAUDE.md content gathered in Step 3}",
  changedFiles: "{changed-file list with stats, or single path for --file}",
  code: "{full diff, or full file content for --file}",
  smallDiff: true | false,        // true when diff < 50 lines -> NIT findings are kept
  systemicCheck: true | false      // false for --file mode (no diff baseline)
}
```

Pass `args` as a real JSON value to the Workflow tool - NOT a JSON-encoded string. (The runtime may still deliver `args` to the script as a JSON string, so the skeleton defensively parses it at the top - see the first line of the script body.)

### W3: Invoke the workflow

Call:

```
Workflow({ scriptPath: "${CLAUDE_PLUGIN_ROOT}/skills/review-workflow/review.workflow.js", args: <W2 args> })
```

The skeleton is a committed asset in the `review-workflow` skill (`skills/review-workflow/review.workflow.js`) - NOT reproduced inline. `${CLAUDE_PLUGIN_ROOT}` is substituted at command-load time, so this markdown already carries the literal absolute path. The script is static; all per-review data travels in `args`. See `skills/review-workflow/SKILL.md` for the `args` contract and the stage flow (fan-out -> dedupe -> verify -> aggregate).

The skeleton's stages: **(1) Review** - fan out `lets:<name>` agents (structured `FINDING_SCHEMA`); **(2) Reduce** - split systemic (any tier) into its own section, NIT-filter the rest (unless small diff), dedupe keep-highest-tier, sort; **(3) Verify** - per BLOCKER/SUGGESTION, fan out `lets:skeptic` (2, or 3 for BLOCKER) with the asymmetric drop rule (Step 6.6); **(4) Aggregate** - verdict over the verified set, summary, `counts` incl. `refuted`. It returns `{ verdict, findings, systemic, summary, counts }`.

**The workflow runs in the BACKGROUND.** The tool returns immediately with a task ID / `runId` - NOT the aggregate. Do not try to read findings from the tool's immediate return. The fan-out continues in the background and a `<task-notification>` arrives when it completes; the orchestrator is re-invoked at that point. Optionally tell the user "Review fan-out running in the background - {N} agents" so the wait is visible (they can watch via `/workflows`).

### W4: Rejoin the standard flow (on workflow completion)

When the workflow's completion notification arrives, the orchestrator resumes with the workflow's returned aggregate object as the result - **this** is the only thing that enters context (the per-agent review reports AND the skeptic verdicts stayed in script variables). The findings are already verified (the script ran Step 6.6 as its Stage 3) and `counts.refuted` says how many were dropped/downgraded. If the workflow failed or returned nothing, surface the error and offer the standard `/lets:review` (Task-based) flow; do not silently drop the review. With the aggregate in hand:
- **Step 6.5** - grep-verify each `systemic[]` entry. If an agent was wrong (pattern only in this file), reclassify it into `findings` at its original tier and recompute the verdict from the new counts. Otherwise trust the script's `verdict`.
- **Step 6.6** - already done in-workflow (Stage 3); do NOT re-run skeptics. Surface `counts.refuted` as `refuted_count`; if `counts.verify_failed` > 0, warn that that many findings couldn't be verified (kept unverified).
- **Step 8** - save the markdown report (render from the returned object).
- **Step 8.5** - if `--json`, write `.lets/reviews/{date}-{mode}.json`. The workflow's `findings` and `verdict` map 1:1 onto the Step 8.5 shape - keep those field names exactly (`/lets:github-pr` reads only those two). The rest of the Step 8.5 wrapper is NOT in the return object and Claude must supply it: add top-level `date`, `mode`, and `findings_count`; and transform each `systemic[]` entry from the finding shape (`{title, file, line, tier, ...}`) into the Step 8.5 systemic shape `{title, count, description}` (use `systemic_count` as `count`). Do not write the raw return object verbatim.
- **Step 9 / Step 10** - output/post and link to task exactly as the standard flow.

## Step 6: Filter & Aggregate Results

**Workflow mode:** the script already filtered, deduped, separated systemic, sorted, and computed the verdict. Skip this step - the aggregate is in the Workflow return value. Continue at Step 6.5. The `skills/review-workflow/review.workflow.js` script mirrors the filter/dedupe rules below; **keep them in sync** (see the Keep-in-sync note under Step 7).

Wait for all agents, then:

1. **Separate systemic first:** Split `[SYSTEMIC]` findings into their own section BEFORE any tier filter. The systemic instruction downgrades tier by one, so a `[SUGGESTION]`-systemic arrives as `[NIT]`; filtering NITs first would drop it before the split can capture it.
2. **Filter:** From the regular (non-systemic) findings, keep [BLOCKER] and [SUGGESTION]. Include [NIT] only for small diffs (<50 lines)
3. **Dedupe:** Remove duplicate issues found by multiple agents. When deduplicating, keep the highest tier (BLOCKER > SUGGESTION > NIT)
4. **Prioritize:** Sort by tier ([BLOCKER] first, then [SUGGESTION], then [NIT])
5. **Count:** Tally issues by category

## Step 6.5: Verify Systemic Findings

For each `[SYSTEMIC]` finding from agents, quick-verify with grep:

```bash
# Example: agent flagged delete() without auth check
grep -r "delete(" --include="*.php" -l | head -10
```

- If confirmed systemic (2+ other files) - keep as systemic, note count
- If agent was wrong (only this file does it) - reclassify as regular finding, restore original tier

Systemic findings go into a separate section in the final report (see Step 9).

## Step 6.6: Verify Findings (Adversarial)

Cut false positives before reporting: each finding gets a refutation pass from the `lets:skeptic` agent. This is core review methodology - it runs in BOTH execution modes; the only difference is WHERE the skeptics run.

**Workflow mode:** this step already ran inside the workflow (its Stage 3) - the returned aggregate is already verified. Skip to Step 7.

**Scope:** verify `[BLOCKER]` and `[SUGGESTION]` findings only (skip `[NIT]`).

**Per finding:** launch `lets:skeptic` (via the Task tool in standard mode) N times - **N=2**, or **N=3** for a `[BLOCKER]`. Each skeptic gets the single finding + the code and returns `{real, confidence, reason}`.

**Asymmetric drop rule (do NOT suppress real bugs):**
- `[SUGGESTION]` -> drop on a simple majority `real=false`.
- `[BLOCKER]` -> drop ONLY on near-unanimous high-confidence refutation (all skeptics `real=false`, or a majority at `high` confidence); **downgrade** to `[SUGGESTION]` on a simple majority `real=false`; otherwise **keep** the `[BLOCKER]` (a confirmed or split BLOCKER stays a BLOCKER). Never silently drop a `[BLOCKER]` on a weak/low-confidence refute.

Survivors keep their (possibly downgraded) tier.

**Standard-mode cap** (bounds the in-context blow-up; workflow mode needs no cap - it verifies off-context): always verify `[BLOCKER]`s; verify at most the top-K=5 `[SUGGESTION]`s; if total findings > 10, verify inline (you act as the skeptic, re-checking each against the code) instead of spawning per-finding agents. If the cap truncates verification, say so in the output - no silent caps.

**Record `refuted_count`** (how many findings the verify pass dropped or downgraded) and surface it in Step 9 + Step 8.5. If any finding could NOT be verified (skeptics errored - `verify_failed` > 0), say so in the output: those findings are kept unverified, not silently treated as clean.

**Keep in sync:** the `skills/review-workflow/review.workflow.js` script implements this same rule in JS (its `decide()`, the Verify stage). Any change here MUST be mirrored there.

## Step 7: Determine Verdict

**Note:** Systemic findings do NOT count toward the verdict - they're informational. The verdict is computed over the **verified** finding set (after Step 6.6 - refuted findings are already dropped/downgraded).

| Condition | Verdict |
|-----------|---------|
| 0 [BLOCKER]s, 0-2 [SUGGESTION]s | APPROVED |
| 0 [BLOCKER]s, 3+ [SUGGESTION]s | APPROVED WITH SUGGESTIONS |
| 1+ [BLOCKER]s | CHANGES REQUESTED |

> **Keep in sync (--workflow):** the `skills/review-workflow/review.workflow.js` script reimplements this table in JS (its `computeVerdict()`: `const verdict = blockers > 0 ? ... : suggestions >= 3 ? ...`). Any change to these thresholds MUST be mirrored there, and vice versa - otherwise the two paths return different verdicts for the same diff. (No unit test pins this - the workflow runtime blocks clean testing; the keep-in-sync discipline + the live smoke test are the guards.)

## Step 8: Save Review (BEFORE output)

**CRITICAL: Save first, then show results.**

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
mkdir -p "$LETS_PROJECT_ROOT/.lets/reviews"
```

Save to:
- PR mode: `.lets/reviews/{date}-PR-{number}.md`
- Local mode: `.lets/reviews/{date}-local-review.md`
- Branch mode: `.lets/reviews/{date}-branch-review.md` (own file - PR-equivalent diff deserves its own artifact)

Content: Full review report with all issues, verdict, and summary.

## Step 8.5: JSON Output

If `--json` flag was provided, save structured JSON and skip Steps 9-10.

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
mkdir -p "$LETS_PROJECT_ROOT/.lets/reviews"
```

Write to `.lets/reviews/{date}-{mode}.json`:

```json
{
  "date": "2026-02-26",
  "mode": "PR-42",
  "verdict": "CHANGES REQUESTED",
  "findings_count": 5,
  "refuted_count": 2,
  "verify_failed": 0,
  "findings": [
    {
      "id": 1,
      "title": "SQL injection in search query",
      "tier": "BLOCKER",
      "agent": "security",
      "file": "src/search.py",
      "line": 42,
      "description": "User input concatenated directly into SQL query",
      "suggestion": "Use parameterized queries"
    }
  ],
  "systemic": [
    {
      "title": "Missing input validation",
      "count": 5,
      "description": "Found in 5 files across the project"
    }
  ],
  "summary": {
    "compliance": "pass",
    "backend": "2 issues",
    "security": "1 issue",
    "architecture": "pass"
  }
}
```

`mode` values: `PR-{number}` | `local-review` | `branch-review` | (plan modes are handled by the Plan Review section, not this path).

`refuted_count` is additive (consumers that don't know it ignore it, e.g. `/lets:github-pr` reads only `findings` + `verdict`): the number of findings the Step 6.6 verify pass dropped or downgraded. Omit or `0` when no verification ran.

After saving, inform user: "Review saved to: {path}"
Then STOP - skip Step 9 (Output) and Step 10 (Link to task).
The calling command handles output and task linking.

## Step 9: Output Results

### For GitHub PR Mode:

Post comment to PR:

```bash
gh pr comment <PR> --body "$(cat <<'EOF'
### Code Review

**Verdict:** {APPROVED | APPROVED WITH SUGGESTIONS | CHANGES REQUESTED}

{If `refuted_count` > 0, add a line: `_Adversarial verify dropped/downgraded {refuted_count} finding(s)._`}

Found {N} issues:

1. **[{TIER}] {issue title}**

   {file link with full SHA}

   {description and fix suggestion}

2. ...

{if systemic findings exist}
### Systemic Issues

These patterns exist across the project, not just in this PR:

1. **{issue title}** - found in {N} files
   {brief description and scope}

{end if}

---

**Review Summary:**
- Compliance: {pass/N issues}
- Backend/Bugs: {pass/N issues}
- Security: {pass/N issues}
- Architecture: {pass/N issues}
- Git History: {pass/N issues}
- Documentation: {pass/N issues}
- Infrastructure: {pass/N issues}
- Database: {pass/N issues}
- Frontend: {pass/N issues}
- Tests: {pass/N issues}
- Pragmatism: {pass/N issues}

Generated with Claude Code
EOF
)"
```

**Link format:** `https://github.com/owner/repo/blob/{full-sha}/path/file.ext#L10-L15`

### For Local Mode:

Display full report in console.

**Always end with:** `Saved to: .lets/reviews/{filename}`

## Step 10: Link Review to Active Task

Reuse the task id resolved in Step 3 ("Resolve the task SPEC") - do NOT call detect-task again. Skip the tracker comment when no id resolved, when validation rejected it, or when `show` failed for it: a `headRefName`-derived id may not exist on this board at all, and `comment-add` would HARD-FAIL at the end of an otherwise successful review.

```lets-tracker
comment-add task=<task-id> body="Code review ({PR #X | local}): {verdict}. {N} issues found."
```

---

## Plan Review

**This section runs when `--plan` is detected. Skips all code review steps above.**

### P1: Load Plan

Use the **detect-task** skill to find the active task: `Skill(skill: "lets:detect-task")`. Plan mode skips the code-review detect-task call below, so resolve the id here; the orchestrator substitutes it for `{task-id}`.

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
BRANCH=$(git branch --show-current)

# If a path was provided to --plan, use it directly and skip this derivation.

# Derive slug: trunk-mode uses task-id; otherwise the branch slug.
# {task-id} is substituted by the orchestrator from the detect-task result above.
if [ "$BRANCH" = "{LETS_MERGE_BRANCH}" ]; then
  SLUG="{task-id}"
else
  SLUG="${BRANCH#feature/}"
fi

# Guard: empty slug would collapse the glob to *.md -> global latest -> another worktree's plan
# (the bug this task fixes). Bail to the no-plan message instead.
if [ -z "$SLUG" ]; then
  PLAN=""
else
  # Latest plan for this slug - date-prefixed or legacy bare name. Slug-scoped (shared .lets
  # across worktrees via symlink, so global latest would grab another branch's plan - lets-fe788).
  PLAN=$(ls -t "$LETS_PROJECT_ROOT/.lets/plans/"*"${SLUG}"*.md 2>/dev/null | head -1)
  # Fallback: glob match by task-id (catches trunk-mode plans + naming drift)
  if [ -z "$PLAN" ] && [ -n "{task-id}" ]; then
    PLAN=$(ls -t "$LETS_PROJECT_ROOT/.lets/plans/"*"{task-id}"*.md 2>/dev/null | head -1)
  fi
fi

[ -n "$PLAN" ] && cat "$PLAN"
```

If no plan files found, inform user and exit:
> "No plans found in `.lets/plans/`. Run `/lets:plan` first."

Read the plan file and show title + task ID to user.

### P2: Gather Context

```bash
LETS_PROJECT_ROOT=$(git rev-parse --show-toplevel)
cat "$LETS_PROJECT_ROOT/CLAUDE.md" 2>/dev/null | head -200
```

Read the codebase files referenced in the plan's "Files" sections (Create/Modify targets) to verify paths exist and understand current state.

### P3: Analyze Plan & Select Agents

Scan the plan content for domain signals:

| Signal in Plan | Detected When | Select Agent |
|----------------|--------------|-------------|
| Backend/API | Mentions controllers, endpoints, services, business logic, PHP/Python/Go/Java | backend |
| Security | Mentions auth, tokens, encryption, secrets, validation, permissions | security |
| Database | Mentions migrations, schema, queries, ORM, indexes, tables | database |
| Frontend | Mentions components, CSS, JS/TS, UI, templates, views, React/Vue | frontend |
| Infrastructure | Mentions Docker, CI/CD, deploy, nginx, scripts, Makefile | devops |
| Testing | Mentions test files, coverage, assertions, TDD, test strategy | qa |

**Always include:** architect (plan structure) + pragmatist (feasibility)
**Add per signal:** each detected domain signal adds its agent
**Never include for plan review:** compliance, git-historian, docs, explorer, implementer, actor (unless user requests)

Show selection before launching:

```
## Agent Selection

Plan signals detected:
- [x] Backend (API endpoints in tasks 2, 4)
- [x] Database (migration in task 3)
- [ ] Frontend
- [ ] Security
...

Selected agents ({N}):
1. architect (always for plan review)
2. pragmatist (always for plan review)
3. backend (API signals detected)
4. database (migration signals detected)

> Subagents have a separate rate limit - no cost to your conversation.
```

### P4: Launch Plan Review Agents (Parallel)

**CRITICAL: Launch ALL selected agents in a SINGLE message.**

#### Architect (always included)

```
Task(
  subagent_type="lets:architect",
  prompt="ultrathink

PROJECT_ROOT: {LETS_PROJECT_ROOT from LETS Config}. Do NOT read or search files outside this directory.

MODE: plan

PLAN REVIEW MODE. Review this implementation plan for quality and completeness.

PROJECT CONTEXT:
{CLAUDE.md summary}

PLAN:
{full plan content}

REVIEW CRITERIA:
- Are all tasks 15-60 minutes of work? (not too big, not trivial)
- Are code snippets complete? (no // TODO, no // implement here, no placeholders)
- Are file paths exact and verified? (do referenced existing files actually exist?)
- Does every task have a verification step with expected output?
- Does every logical unit have a commit point?
- Are interfaces clearly defined? (function signatures, types, data flow)
- Is the task ordering logical? (dependencies respected, builds incrementally)
- Are edge cases addressed?
- Is the codebase map accurate? (do referenced patterns actually exist?)

OUTPUT FORMAT:
## Plan Review: Architecture

### Verdict: {APPROVED | NEEDS REVISION}

### Findings (only report actual issues)
1. **{issue}** [Task N]
   {what's wrong and how to fix it}

### Missing
{things the plan should cover but doesn't}

### Strengths
{1-2 things done well - keep feedback balanced}"
)
```

#### Pragmatist (always included)

```
Task(
  subagent_type="lets:pragmatist",
  prompt="ultrathink

PROJECT_ROOT: {LETS_PROJECT_ROOT from LETS Config}. Do NOT read or search files outside this directory.

MODE: plan

PLAN REVIEW MODE. Review this implementation plan for feasibility and proportionality.

PROJECT CONTEXT:
{CLAUDE.md summary}

PLAN:
{full plan content}

REVIEW CRITERIA:
- Is the overall approach proportional to the problem? (not overengineered?)
- Are there simpler alternatives for any task?
- Is the effort estimate realistic?
- Are there unnecessary abstractions or premature optimizations?
- Does the plan follow existing codebase patterns or reinvent the wheel?
- Is the scope well-defined? (clear what's in and out)
- Are there tasks that could be cut without losing core value?
- Will this plan survive contact with reality? (hidden dependencies, risky assumptions)

OUTPUT FORMAT:
## Plan Review: Pragmatism

### Verdict: {APPROVED | NEEDS REVISION}

### Findings (only report actual issues)
1. **{issue}** [Task N]
   {what's wrong and how to fix it}

### Scope Check
- Core value delivered: {yes/no, what}
- Unnecessary extras: {if any}
- Missing essentials: {if any}

### Bottom Line
{1-2 sentences: ship it or revise it, and why}"
)
```

#### Domain Experts (dynamically selected in P3)

For each additional agent selected in P3:

```
Task(
  subagent_type="lets:{agent-name}",
  prompt="ultrathink

PROJECT_ROOT: {LETS_PROJECT_ROOT from LETS Config}. Do NOT read or search files outside this directory.

MODE: plan

PLAN REVIEW MODE. Review this implementation plan from your domain expertise.

PROJECT CONTEXT:
{CLAUDE.md summary}

PLAN:
{full plan content}

REVIEW CRITERIA:
- Are the plan's assumptions correct in your domain?
- Are there risks or gaps the plan misses from your perspective?
- Are the proposed solutions following best practices in your area?
- Are there simpler alternatives for tasks in your domain?
- Grep the codebase to verify plan claims about existing code.

OUTPUT FORMAT:
## Plan Review: {Your Domain}

### Verdict: {APPROVED | NEEDS REVISION}

### Findings (only report actual issues)
1. **{issue}** [Task N]
   {what's wrong and how to fix it}

### Domain-Specific Risks
{risks from your area of expertise}

### Strengths
{1-2 things done well}"
)
```

### P5: Aggregate & Output

After all agents respond:

```
## Plan Review: **{plan title}** (`{task-id}`)

**Verdict:** {APPROVED if all approve | NEEDS REVISION if any agent flags revision}
**Agents:** {N} ({agent names})

### Architecture
{architect findings summary}

### Pragmatism
{pragmatist findings summary}

{for each additional domain agent that reported findings}
### {Agent Domain}
{agent findings summary}
{end for}

{if NEEDS REVISION}
### Action Items
1. {specific fix}
2. {specific fix}
...
{end if}

Saved to: .lets/reviews/{date}-plan-review.md
```

### P6: Save & Link

Save to `.lets/reviews/{date}-plan-review.md`

If active task found:
```lets-tracker
comment-add task=<task-id> body="Plan review: {verdict}. {N} issues found."
```

### Plan Review Output

**If approved:**
```
┌─ LETS ───────────────────────┐
│  Execute?      /lets:execute │
└──────────────────────────────┘
```

**If needs revision:** No box. List action items to fix in the plan first.

---

## Notes

- Use `gh` CLI for GitHub operations
- Always use full git SHA in links
- Filter by tier: keep [BLOCKER] and [SUGGESTION], skip [NIT] for large diffs
- For PR: post comment even if no issues (confirms review happened)
- For local: show full report in console + save to file
- Agents define their own expertise, tiered scoring, output format, and mode behavior in agents/*.md
- The review command only provides context (diff, CLAUDE.md) and orchestrates

## Workflow Integration

### Option A: Review before PR (recommended for significant changes)
```
Work -> /lets:review --local -> Fix issues -> /lets:commit -> Push -> PR
```

For multi-commit branches, add `/lets:review --branch` between `/lets:commit` and `Push` for a final PR-equivalent pass (three-dot diff against `$LETS_MERGE_BRANCH` — same shape GitHub would show). On a single-commit branch `--branch` ≡ `--last-commit`, so it's optional polish, not prescription.

### Option B: Review after PR
```
Work -> /lets:commit -> Push -> PR -> /lets:review <PR>
                                        |
                                   Comment on PR
```

### Quick check vs Full review
```
/lets:check  = Quick sanity check (~30 sec), before any commit
/lets:review = Full deep review (~2-3 min), before PR or on PR
```

## Rules

- Respond in user's language

## Output

**If no issues or approved:**
```
┌─ LETS ───────────────────────┐
│  Commit?  /lets:commit       │
│  Check?   /lets:check        │
└──────────────────────────────┘
```

**If changes requested:** No box. List issues to fix first.
