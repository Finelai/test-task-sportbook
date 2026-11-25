#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
check() {
  expected=$1
  branch=$2
  if sh "$repo_root/tools/git-hooks/check-branch-name.sh" "$branch" >/dev/null 2>&1; then actual=pass; else actual=fail; fi
  if [ "$actual" != "$expected" ]; then
    printf 'Expected %s, got %s: %s\n' "$expected" "$actual" "$branch" >&2
    exit 1
  fi
}
check pass feat/add-bets
check pass fix/api-2
check pass feature/add-bets
check pass bugfix/add-bets
check pass hotfix/security-patch
check pass release/v1.2.0
check pass chore/upgrade-node-v20.11
check pass main
check pass master
check pass develop
check fail codex/add-bets
check fail docs/update
check fail feat/Add-Bets
check fail feat/-bets
check fail feat/add--bets
check fail feat/add-bets/
check fail release/v1.-2.0
check fail release/v1.2.0.
if printf '%s\n' 'refs/heads/feat/add-bets 123 refs/heads/master 000' | sh "$repo_root/.githooks/pre-push" origin example >/dev/null 2>&1; then
  printf 'Direct push to master must fail\n' >&2
  exit 1
fi
printf '%s\n' 'refs/heads/feat/add-bets 123 refs/heads/feat/add-bets 000' | sh "$repo_root/.githooks/pre-push" origin example
printf 'branch name tests passed\n'
