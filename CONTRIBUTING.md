# Contributing

Install the Git hooks once after cloning the repository:

```sh
make install-hooks
```

The `commit-msg` hook checks the first line of every commit message. Use
the Conventional Commits format:

```text
type(scope)!: description
```

The scope and `!` are optional. Use `!` for a breaking change. The allowed
types are `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`,
`build`, `ci`, `chore`, and `revert`. The description must not be empty.
For example:

```text
feat(api): add bets endpoint
fix: reject invalid amounts
refactor(api)!: remove legacy endpoint
```

Merge and revert commits must also use this format. Git hooks are local to
each clone; run `make test-hooks` to verify the hook.

## Branches and merging

Follow [Conventional Branch 1.0.0](https://conventionalbranch.org/v1.0.0/).
Name work branches `type/short-description`. The types are `feature`, `feat`,
`bugfix`, `fix`, `hotfix`, `release`, and `chore`. Use lowercase Latin letters
and digits in the description; separate segments with single hyphens or dots.
Examples: `feat/add-bets`, `fix/invalid-amount`, and `release/v1.2.0`.
The trunk names `main`, `master`, and `develop` do not require a prefix.

Open a pull request to merge into `master`. Merge only after the CI checks pass.
Use a Conventional Commit message for the resulting commit, including when
using squash merge. The local pre-push hook checks branch names and blocks
direct pushes to `master`; CI also checks pull request branch names.

Local hooks can be bypassed. On the Git hosting service, protect `master` by
requiring pull requests and successful `check` and `branch-name` jobs, and
restrict direct pushes. Configure these settings after connecting a remote.
