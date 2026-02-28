# AskUserQuestion Preview Feature

## What

`AskUserQuestion` tool supports `markdown` field on options. When any option has `markdown`, UI switches to split-screen: options list on the left, preview panel on the right. Selecting an option updates the preview.

## How to use

```json
{
  "question": "Which approach?",
  "header": "Design",
  "options": [
    {
      "label": "Option A",
      "description": "Short description",
      "markdown": "```\nASCII mockup or code here\nShown in preview panel\n```"
    },
    {
      "label": "Option B",
      "description": "Short description",
      "markdown": "different-preview-content"
    }
  ],
  "multiSelect": false
}
```

## Best use cases for LETS plugin

- **Architecture choices** - show file tree structures side by side
- **Code approach comparison** - show code snippets for each option
- **UI layout decisions** - ASCII mockups
- **Config format options** - show example YAML/JSON for each variant
- `/lets:opinion` - agents could present options with preview
- `/lets:brainstorm` - approach comparison during planning phases

## Constraints

- Only works with `multiSelect: false` (single select)
- Preview is monospace box
- Supports multi-line with newlines

## Discovered

2026-02-27 - first used during dolt-remote setup, file structure comparison.
