# Security Policy

## Supported versions

Only the latest release receives fixes. There is no long-term-support branch — `lets` is a single-binary CLI; upgrade to the newest release:

```bash
curl -fsSL https://raw.githubusercontent.com/restarter/lets-workflow/main/scripts/install.sh | bash
```

## Reporting a vulnerability

**Please do not open a public issue for security problems.**

Use GitHub's private vulnerability reporting: open the repository's **Security** tab → **Report a vulnerability** ([how it works](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability)). That opens a private advisory thread visible only to you and the maintainers.

Include:

- a description of the vulnerability and its impact,
- steps to reproduce (a proof-of-concept if you have one),
- the affected version (`lets version`).

You'll get an acknowledgement within a few days. There's no bug-bounty program — this is a small open-source project — but valid reports are credited in the release notes unless you'd rather stay anonymous.

## Scope

In scope: the `lets` CLI (`cli/`), the install script (`scripts/install.sh`), the plugin's hooks (`plugins/lets/hooks/`), and anything that handles tokens/credentials or executes downloaded content.

Out of scope: issues in dependencies (report upstream — [beads](https://github.com/steveyegge/beads), [Dolt](https://github.com/dolthub/dolt), Claude Code), social engineering, and anything that requires an already-compromised machine.
