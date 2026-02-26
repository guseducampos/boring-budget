---
name: boring-budget-release
description: Automate boring-budget release preparation by staging and committing changes, pushing the branch, cutting a semantic version tag, and pushing the tag to trigger the GitHub release workflow.
---

# Boring Budget Release

Use this skill when you want one command to automate release prep and tag publishing for this repository.

## What it does

1. Optionally stages all tracked/untracked changes and creates a commit.
2. Pushes the target branch to the selected remote.
3. Resolves a release tag (`patch|minor|major|custom`).
4. Creates an annotated release tag.
5. Pushes the tag so `.github/workflows/release.yml` runs.

## Script

- Script path: `scripts/release.sh`
- Run from repository root.

## Usage

```bash
# Typical patch release with commit + push + tag
skills/boring-budget-release/scripts/release.sh \
  --type patch \
  --message "chore: prepare release"

# Minor release without creating a commit (already committed)
skills/boring-budget-release/scripts/release.sh \
  --type minor \
  --no-commit

# Custom tag
skills/boring-budget-release/scripts/release.sh \
  --type custom \
  --version v1.4.0 \
  --message "chore: release v1.4.0"
```

## Flags

- `--type <patch|minor|major|custom>`: release bump type. Default `patch`.
- `--version <vX.Y.Z>`: required when `--type custom`.
- `--message <msg>`: commit message when creating a commit.
- `--branch <name>`: branch to push. Default current branch.
- `--remote <name>`: remote name. Default `origin`.
- `--no-commit`: skip staging/commit and require clean worktree.
- `--skip-tests`: skip `go test ./...` before pushing.
- `--yes`: skip confirmation prompt.

## Safety rules

- Requires semver tags in format `vX.Y.Z`.
- Fails if the release tag already exists locally or on remote.
- If `--no-commit` is used, the worktree must be clean.
- If commit mode is used and there are no changes, commit step is skipped.
