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

do_tmux() {
	if ! command -v tmux >/dev/null 2>&1; then
		echo "ERROR: tmux not found on PATH (brew install tmux / apt install tmux)" >&2
		exit 3
	fi
	# NOTE: no top-level `do_build` here. Each pane runs `make dev` independently,
	# which resolves `git rev-parse --show-toplevel` to its OWN worktree and builds
	# `<that-worktree>/cli/lets`. A build of the orchestrator's repo (run from
	# main repo or a different worktree) would be unused by any pane.
	# Determine worktrees: explicit list via args, else auto-discover from .worktrees/.
	local worktrees=()
	if [ $# -gt 0 ]; then
		for name in "$@"; do
			local p="$REPO/.worktrees/$name"
			[ -d "$p" ] || { echo "ERROR: $p not a worktree directory" >&2; exit 4; }
			worktrees+=("$p")
		done
	else
		for p in "$REPO/.worktrees"/*/; do
			[ -d "$p" ] || continue
			worktrees+=("${p%/}")
		done
	fi
	local n=${#worktrees[@]}
	if [ "$n" -eq 0 ]; then
		echo "ERROR: no worktrees under $REPO/.worktrees/. Create one first: lets worktree create <name>" >&2
		exit 5
	fi
	if [ "$n" -gt 10 ]; then
		# TTY guard: in non-interactive contexts (CI, `tmux send-keys` from a wrapper,
		# piped invocation), `read` would block forever. Fail-fast with a clear hint
		# instead of hanging. User can pass explicit names to bypass the threshold.
		if [ ! -t 0 ]; then
			echo "ERROR: $n worktrees (> 10) and stdin is not a TTY." >&2
			echo "  Pass explicit names: scripts/dev/run.sh tmux <name1> <name2> ..." >&2
			echo "  Or run from an interactive shell." >&2
			exit 5
		fi
		printf '⚠ %d worktrees found. Open all panes? [y/N] ' "$n" >&2
		local reply
		read -r reply
		case "$reply" in
			y|Y|yes) ;;
			*) echo "aborted" >&2; exit 0 ;;
		esac
	fi
	local session="lets-dev-$$"
	echo "→ tmux session '$session' with $n pane(s)" >&2
	local first_wt="${worktrees[0]}"
	tmux new-session -d -s "$session" -c "$first_wt" "cd '$first_wt' && exec make dev"
	local i
	for ((i=1; i<n; i++)); do
		local wt="${worktrees[i]}"
		tmux split-window -t "$session" -c "$wt" "cd '$wt' && exec make dev"
		tmux select-layout -t "$session" tiled
	done
	echo "  attach: tmux attach -t '$session'" >&2
}

cmd=${1:-info}; [ $# -gt 0 ] && shift
case "$cmd" in
	build)  do_build ;;
	info)   do_info ;;
	shell)  do_shell ;;
	claude) do_claude "$@" ;;
	tmux)   do_tmux "$@" ;;
	-h|--help|help)
		echo "Usage: scripts/dev/run.sh {build|info|shell|claude [args]|tmux [names...]}"
		;;
	*)
		echo "ERROR: unknown subcommand '$cmd'. Try: scripts/dev/run.sh --help" >&2
		exit 1
		;;
esac
