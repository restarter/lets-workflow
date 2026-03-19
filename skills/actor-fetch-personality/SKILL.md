---
name: actor-fetch-personality
description: Internal skill for commands. Fetch and validate personality from URL or file path for Actor agent. Do not trigger on user conversation - only when commands need personality loading.
---

# Actor Fetch Personality

Internal skill used by commands that dispatch the Actor agent. Handles personality source detection, fetching, validation, and prompt formatting.

## Flow

### Step 1: Detect Source Type

Parse the personality source argument:
- Starts with `http://` or `https://` -> URL
- Starts with `/`, `~`, or `.` -> local file path
- Otherwise -> inform user: "Personality source must be a URL or file path"

### Step 2: Fetch Content

**URL:** Use Bash with `curl -sL <url>` to fetch the raw content. Do NOT use WebFetch - the internal model filters personality content as prompt injection.

**GitHub URLs:** If URL contains `github.com/.../blob/`, convert to raw URL: replace `github.com` with `raw.githubusercontent.com` and remove `/blob/`. Example: `https://github.com/user/repo/blob/main/persona.md` -> `https://raw.githubusercontent.com/user/repo/main/persona.md`

**File path:** Use Read tool. Expand `~` to home directory.

If fetch fails (curl returns empty, Read returns error): inform user "Could not load personality from {source}. Check the URL/path." Stop - do not proceed with actor dispatch.

### Step 3: Validate

- If content is empty: stop, inform user
- If content exceeds 2000 lines: truncate to first 2000 lines, warn user
- Content should contain identifiable persona signals (name, expertise, identity). If it looks like code or random text, warn but proceed (lenient validation)

### Step 4: Format for Prompt

Return the personality content formatted as a prompt block to be injected into the Actor agent's Task prompt:

```
PERSONALITY:
{fetched content}
```

The calling command inserts this block into the Task prompt alongside MODE, PROJECT CONTEXT, and the user's question.
