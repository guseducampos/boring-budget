# Releasing

## Homebrew setup (one-time)

1. Create the tap repository `gustavocampos/homebrew-tap`.
2. Add a repository secret in this repo:
   - `HOMEBREW_TAP_GITHUB_TOKEN`: a token with write access to `gustavocampos/homebrew-tap`.
3. Ensure the tap accepts formula updates under `Formula/`.

## Publish a release

1. Create and push a semver tag:

```bash
git tag v0.1.0
git push origin v0.1.0
```

2. GitHub Actions `Release` workflow will:
   - run tests
   - build archives for macOS/Linux/Windows
   - publish GitHub release artifacts + checksums
   - update Homebrew formula in the tap repo

## Verify Homebrew install

```bash
brew tap gustavocampos/tap
brew update
brew install boring-budget
boring-budget --help
```
