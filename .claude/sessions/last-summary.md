## Session Summary 2026-02-12 20:30

### Done
- Brainstormed Expert Agents Team concept (from 10 review agents to 10 universal experts)
- Defined agent roster: architect, security, backend, frontend, database, devops, qa, docs, compliance, git-historian
- Designed integration with lets-review, lets-opinion, and new lets-ask skill
- Designed auto model selection logic (haiku/sonnet/opus by complexity)
- Written and approved design document
- Created beads task with full description and progress comments

### In Progress
- lets-plugin-claude-fbp: Expert Agents Team - design done, implementation next

### Commits
- 1a6d8ab docs: Add Expert Agents Team design document

### Key Decisions
- Agent knows WHO (expertise in .md file), skill tells WHAT (prompt with context)
- /lets-ask = 1 expert (Slack ping), /lets-opinion = 3-5 experts (team meeting)
- No Edit/Write tools for agents - they analyze, don't modify
- Auto model selection: haiku for simple, sonnet for standard, opus for complex
- All agents default sonnet, skills override via Task tool model param

### Next Steps
- Create 10 agent .md files in agents/
- Create /lets-ask skill (new)
- Update /lets-review to use Task tool with agents
- Update /lets-opinion to launch agents in parallel
- Update CLAUDE.md with new capabilities

### Context for Next Session
- Branch: main
- Task: lets-plugin-claude-fbp "Expert Agents Team"
- Design doc: docs/plans/2026-02-12-expert-agents-team-design.md
- Plugin structure: agents/ dir at project root (plugin scope), skills in .claude/skills/
- Reference agents format: see reference/feature-dev/plugins/feature-dev/agents/
