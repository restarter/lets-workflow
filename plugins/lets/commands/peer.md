---
description: Talk to a named LETS peer session (alias of /lets:orc with a target)
argument-hint: "<name> <ask|ping|read|tell|who> [text]"
---

# Peer

Alias of the `orc` skill with an explicit target.

1. Split the argument: `<name>` (a name with spaces is quoted), `<verb>`, then the rest as text. A `<name>` holding a quote, `$`, backtick or newline is refused in one line - Go validates the rest.
2. `Skill(skill: "lets:orc", args: "target=\"<name>\" verb=<verb> text=<rest>")` - without `footer=none`: this command adds nothing after the delegated run, so the orc run's single Close footer is the response's footer and this file emits none.
