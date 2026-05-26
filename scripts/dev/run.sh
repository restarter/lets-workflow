#!/bin/bash
# scripts/dev/run.sh — Dev workflow for parallel worktree testing.
#
# Subcommands:
#   scripts/dev/run.sh build            Build cli/lets with dev-<branch>-<sha>[-dirty] version.
#   scripts/dev/run.sh info             Print dev state (binary, branch, plugin dir, conflicts).
#   scripts/dev/run.sh shell            Build + spawn $SHELL with PATH prepended.
#   scripts/dev/run.sh claude [args]    Build + exec `claude --plugin-dir <repo>/plugins/lets [args]`.
#   scripts/dev/run.sh tmux [names...]  Spawn tmux session with one Claude pane per worktree.
#
# Designed for TMUX-N-panes parallel testing: each worktree pane runs `make dev`
# (which calls this script's `claude` subcommand). Per-shell PATH prepend, no
# global install, no marketplace mutation. Production `lets` at ~/.local/bin
# stays untouched.

set -euo pipefail

REPO=$(git rev-parse --show-toplevel 2>/dev/null) || {
	echo "ERROR: not a git repository" >&2
	exit 2
}
CLI_DIR="$REPO/cli"
CLI_BIN="$CLI_DIR/lets"
PLUGIN_DIR="$REPO/plugins/lets"

# dev_version produces "dev-<branch>-<sha>[-dirty]". Slashes in branch names
# (e.g. feature/x) are replaced with hyphens because the value flows through
# Makefile's VERSION= which is touchy about special chars in ldflag values.
#
# Detached HEAD fallback: `git rev-parse --abbrev-ref HEAD` returns the
# literal "HEAD" when no branch is checked out (mid-bisect, mid-`git worktree
# add --detach`, manual sha checkout). symbolic-ref returns the branch ref
# OR fails when detached — use that to detect and label clearly.
dev_version() {
	local branch sha dirty=""
	branch=$(git -C "$REPO" symbolic-ref --short HEAD 2>/dev/null) || branch="detached"
	sha=$(git -C "$REPO" rev-parse --short=7 HEAD)
	if [ -n "$(git -C "$REPO" status --porcelain 2>/dev/null)" ]; then
		dirty="-dirty"
	fi
	printf 'dev-%s-%s%s' "${branch//\//-}" "$sha" "$dirty"
}

do_build() {
	local v
	v=$(dev_version)
	echo "→ building cli/lets ($v)" >&2
	make -C "$REPO" build VERSION="$v" >&2
}

do_info() {
	printf 'Repo:        %s\n' "$REPO"
	printf 'Branch:      %s\n' "$(git -C "$REPO" rev-parse --abbrev-ref HEAD)"
	printf 'HEAD:        %s\n' "$(git -C "$REPO" rev-parse --short=7 HEAD)"
	printf 'Dev version: %s\n' "$(dev_version)"
	printf 'Plugin dir:  %s\n' "$PLUGIN_DIR"
	if [ -x "$CLI_BIN" ]; then
		printf 'Binary:      %s (%s)\n' "$CLI_BIN" "$("$CLI_BIN" --version 2>&1)"
	else
		printf 'Binary:      not built (run: scripts/dev/run.sh build)\n'
	fi
	# Warn if a global `lets` would shadow the dev binary on PATH.
	local global
	global=$(command -v lets 2>/dev/null || true)
	if [ -n "$global" ] && [ "$global" != "$CLI_BIN" ]; then
		printf '⚠ Global:    %s (wins on PATH unless exported via dev shell|claude|tmux)\n' "$global"
	fi
}

do_shell() {
	do_build
	echo "→ dev subshell. PATH=$CLI_DIR:..., plugin=$PLUGIN_DIR" >&2
	echo "  launch claude with: claude --plugin-dir \"$PLUGIN_DIR\"" >&2
	PATH="$CLI_DIR:$PATH" "${SHELL:-/bin/bash}"
}

do_claude() {
	do_build
	if ! command -v claude >/dev/null 2>&1; then
		echo "ERROR: claude not found on PATH (install Claude Code first)" >&2
		exit 3
	fi
	echo "→ launching: claude --plugin-dir $PLUGIN_DIR" >&2
	PATH="$CLI_DIR:$PATH" exec claude --plugin-dir "$PLUGIN_DIR" "$@"
}

cmd=${1:-info}; [ $# -gt 0 ] && shift
case "$cmd" in
	build)  do_build ;;
	info)   do_info ;;
	shell)  do_shell ;;
	claude) do_claude "$@" ;;
	-h|--help|help)
		# Task 2 widens --help to include shell + claude. Task 3 extends to tmux.
		echo "Usage: scripts/dev/run.sh {build|info|shell|claude [args]}"
		;;
	*)
		echo "ERROR: unknown subcommand '$cmd'. Try: scripts/dev/run.sh --help" >&2
		exit 1
		;;
esac
