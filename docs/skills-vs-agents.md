# Skills vs Agents vs Rules

## Quick Comparison

| Aspect | Skill | Agent | Rule |
|--------|-------|-------|------|
| **What** | Prompt template (slash command) | Autonomous worker | Always-on instruction |
| **When loaded** | On demand (`/skill-name`) | On delegation (Task tool) | Every conversation |
| **Who runs** | Main Claude session | Separate subprocess | Main Claude session |
| **Context** | Full conversation context | Only the prompt you give it | Full conversation context |
| **Can edit files** | Yes (main session) | Yes (if tools allowed) | N/A (just rules) |
| **Use case** | Workflows, checklists | Parallel work, specialization | Code standards, behavior |

## When to Use What

### Skills
- Interactive workflows (commit, review, session management)
- Checklists the user follows step by step
- Things that need conversation context
- Example: `/lets-commit`, `/lets-review`, `/lets-start`

### Agents
- Parallel independent work (multiple reviewers)
- Specialized knowledge (Android expert, security expert)
- Protecting main context from large outputs
- Example: `lets:android-reviewer`, `lets:security-reviewer`

### Rules
- Code standards (always apply)
- Behavior rules (never commit without permission)
- Language preferences
- Example: `android-standards.md`, `workflow.md`

## How They Work Together

```
User: /lets-review --local

Skill (lets-review) activates:
  1. Analyzes diff
  2. Selects relevant agents
  3. Launches agents in parallel via Task tool:
     - Task(subagent_type="lets:compliance-reviewer", prompt="...")
     - Task(subagent_type="lets:bug-scanner", prompt="...")
     - Task(subagent_type="lets:security-reviewer", prompt="...")
  4. Aggregates results
  5. Shows filtered report

Rules apply throughout:
  - language.md: respond in user's language
  - workflow.md: never commit without permission
```

## Key Insight

**Skills orchestrate, agents execute, rules constrain.**

A skill like `/lets-review` is the orchestrator - it decides which agents to launch and how to present results. The agents do the actual review work. Rules ensure everything follows project conventions.
