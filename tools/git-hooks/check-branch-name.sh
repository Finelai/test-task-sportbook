#!/bin/sh
set -eu

if ! printf '%s\n' "$1" | LC_ALL=C grep -Eq '^(main|master|develop|(feature|feat|bugfix|fix|hotfix|release|chore)/[a-z0-9]+([.-][a-z0-9]+)*)$'; then
  printf 'Invalid branch name: %s\nUse type/short-description, for example feat/add-bets.\n' "$1" >&2
  exit 1
fi
