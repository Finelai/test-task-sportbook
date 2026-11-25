#!/bin/sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT HUP INT TERM

git init -q "$test_dir/repo"
git -C "$test_dir/repo" config user.name 'Hook Test'
git -C "$test_dir/repo" config user.email 'hook-test@example.com'
git -C "$test_dir/repo" config core.hooksPath "$repo_root/.githooks"

check_commit() {
  expected=$1
  message=$2
  if git -C "$test_dir/repo" commit --allow-empty -m "$message" >/dev/null 2>&1; then
    actual=pass
  else
    actual=fail
  fi
  if [ "$actual" != "$expected" ]; then
    printf 'Expected %s, got %s: %s\n' "$expected" "$actual" "$message" >&2
    exit 1
  fi
}

check_commit pass 'feat: add betting'
check_commit pass 'fix(api)!: remove legacy endpoint'
check_commit pass 'docs: update setup'
check_commit fail 'add betting'
check_commit fail 'feat: '
check_commit fail 'unknown: add betting'
check_commit fail 'feat:add betting'
printf 'commit-msg hook tests passed\n'
