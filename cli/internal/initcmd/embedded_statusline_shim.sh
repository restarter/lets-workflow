#!/bin/sh
# LETS-branded statusline for Claude Code.
#
# Thin shim: pipes Claude Code's JSON stdin into `lets statusline`.
# All formatting/fetch logic lives in the Go binary (cli/internal/statusline/).
#
# Requires `lets` on $PATH. Falls back to a basic message if missing so
# Claude's statusline panel still renders something.

if command -v lets >/dev/null 2>&1; then
  cat | lets statusline
else
  printf "\033[1;38;2;255;215;0m\xf0\x9f\x8c\xb1 LETS Workflow\033[0m \033[38;2;153;122;0m\xc2\xbb\033[0m \033[38;2;255;100;100mlets binary not on \$PATH - run 'make install'\033[0m\n"
fi
