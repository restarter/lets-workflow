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
- Specialized knowledge (security expert, architect)
- Protecting main context from large outputs
- Example: `architect`, `security-expert`, `qa-expert`

### Rules
- Code standards (always apply)
- Behavior rules (never commit without permission)
- Language preferences
- Example: `architecture.md`, `workflow.md`, `language.md`

## How They Work Together

```
User: /lets-review --local

Skill (lets-review) activates:
  1. Analyzes diff
  2. Selects relevant expert agents (3-10 from pool of 11)
  3. Launches agents in parallel via Task tool:
     - Task(subagent_type="architect", prompt="Review this diff...")
     - Task(subagent_type="security-expert", prompt="Review for vulnerabilities...")
     - Task(subagent_type="compliance-expert", prompt="Check project rules...")
  4. Filters results (confidence >= 80)
  5. Shows aggregated report

Rules apply throughout:
  - language.md: respond in user's language
  - workflow.md: never commit without permission
```

**Key insight: Skills orchestrate, agents execute, rules constrain.**
