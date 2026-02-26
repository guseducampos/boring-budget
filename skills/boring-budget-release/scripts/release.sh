#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'USAGE'
Usage:
  scripts/release.sh [options]

Options:
  --type <patch|minor|major|custom>  Release bump type (default: patch)
  --version <vX.Y.Z>                 Custom version (required for --type custom)
  --message <msg>                    Commit message (used when committing)
  --branch <name>                    Branch to push (default: current branch)
  --remote <name>                    Remote to push (default: origin)
  --no-commit                        Skip staging+commit (requires clean worktree)
  --skip-tests                       Skip go test ./...
  --yes                              Do not prompt for confirmation
  -h, --help                         Show this help
USAGE
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

validate_semver() {
  local value="$1"
  [[ "$value" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]
}

next_tag_from_latest() {
  local latest="$1"
  local bump="$2"

  if [[ ! "$latest" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
    echo "latest tag is not semver: $latest" >&2
    exit 1
  fi

  local major="${BASH_REMATCH[1]}"
  local minor="${BASH_REMATCH[2]}"
  local patch="${BASH_REMATCH[3]}"

  case "$bump" in
    patch)
      patch=$((patch + 1))
      ;;
    minor)
      minor=$((minor + 1))
      patch=0
      ;;
    major)
      major=$((major + 1))
      minor=0
      patch=0
      ;;
    *)
      echo "invalid bump type: $bump" >&2
      exit 1
      ;;
  esac

  echo "v${major}.${minor}.${patch}"
}

worktree_dirty() {
  [[ -n "$(git status --porcelain)" ]]
}

release_type="patch"
custom_version=""
commit_message=""
branch=""
remote="origin"
no_commit=0
skip_tests=0
assume_yes=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --type)
      release_type="${2:-}"
      shift 2
      ;;
    --version)
      custom_version="${2:-}"
      shift 2
      ;;
    --message)
      commit_message="${2:-}"
      shift 2
      ;;
    --branch)
      branch="${2:-}"
      shift 2
      ;;
    --remote)
      remote="${2:-}"
      shift 2
      ;;
    --no-commit)
      no_commit=1
      shift
      ;;
    --skip-tests)
      skip_tests=1
      shift
      ;;
    --yes)
      assume_yes=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage
      exit 1
      ;;
  esac
done

require_cmd git
require_cmd go

if [[ -z "$branch" ]]; then
  branch="$(git rev-parse --abbrev-ref HEAD)"
fi

if [[ "$branch" == "HEAD" ]]; then
  echo "detached HEAD is not supported; checkout a branch first" >&2
  exit 1
fi

if [[ "$release_type" != "patch" && "$release_type" != "minor" && "$release_type" != "major" && "$release_type" != "custom" ]]; then
  echo "invalid --type: $release_type" >&2
  exit 1
fi

if [[ "$release_type" == "custom" ]]; then
  if [[ -z "$custom_version" ]]; then
    echo "--version is required when --type custom" >&2
    exit 1
  fi
  if ! validate_semver "$custom_version"; then
    echo "invalid custom version: $custom_version (expected vX.Y.Z)" >&2
    exit 1
  fi
fi

if [[ $no_commit -eq 1 ]]; then
  if worktree_dirty; then
    echo "--no-commit requires a clean worktree" >&2
    exit 1
  fi
else
  if worktree_dirty; then
    if [[ -z "$commit_message" ]]; then
      echo "--message is required when committing changes" >&2
      exit 1
    fi
    git add -A
    git commit -m "$commit_message"
  else
    echo "no local changes to commit; continuing"
  fi
fi

if [[ $skip_tests -eq 0 ]]; then
  echo "running go test ./..."
  go test ./...
else
  echo "skipping tests"
fi

git fetch --tags "$remote"

latest_tag="$(git tag --list 'v*' --sort=-version:refname | head -n1)"
if [[ -z "$latest_tag" ]]; then
  latest_tag="v0.0.0"
fi

if [[ "$release_type" == "custom" ]]; then
  release_tag="$custom_version"
else
  release_tag="$(next_tag_from_latest "$latest_tag" "$release_type")"
fi

if git rev-parse "$release_tag" >/dev/null 2>&1; then
  echo "tag already exists locally: $release_tag" >&2
  exit 1
fi
if git ls-remote --tags "$remote" "refs/tags/$release_tag" | grep -q .; then
  echo "tag already exists on remote: $release_tag" >&2
  exit 1
fi

summary="
Remote:        $remote
Branch:        $branch
Latest tag:    $latest_tag
Release tag:   $release_tag
Commit SHA:    $(git rev-parse --short HEAD)
"

echo "$summary"

if [[ $assume_yes -eq 0 ]]; then
  read -r -p "Proceed with push and tag creation? [y/N] " answer
  if [[ "$answer" != "y" && "$answer" != "Y" ]]; then
    echo "aborted"
    exit 1
  fi
fi

git push "$remote" "$branch"
git tag -a "$release_tag" -m "Release $release_tag"
git push "$remote" "$release_tag"

echo "release tag pushed: $release_tag"
echo "GitHub Release workflow should start from tag push (workflow: .github/workflows/release.yml)."
