#!/bin/sh
set -eu

if ! printf '%s\n' "$1" | LC_ALL=C grep -Eq '^(feat|fix|docs|style|refactor|perf|test|build|ci|chore|revert|codex)/[a-z0-9]+(-[a-z0-9]+)*$'; then
  printf 'Invalid branch name: %s\nUse type/short-description, for example feat/add-bets.\n' "$1" >&2
  exit 1
fi
