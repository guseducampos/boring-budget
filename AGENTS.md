# AGENTS.md

Execution policy for agents and contributors working on `boring-budget`.

This file defines how to execute work. Product and technical behavior specifications live under `docs/`.

## 1) Mission

Build and maintain an agent-friendly Go CLI for budgeting with deterministic behavior, strong auditability, and safe/non-destructive data handling.

## 2) Source of Truth (`docs/`)

Treat `docs/` as the canonical source for specs and rules.

Ownership map:
- Canonical product + technical spec: `docs/SPEC.md`
- Docs navigation and read order: `docs/README.md`
- Agent-facing JSON contracts, errors, and exit codes: `docs/contracts/*`

`AGENTS.md` is operational policy only. Do not define new product/technical behavior here.

## 3) Required Working Loop (Harness-Oriented)

For non-trivial changes:
1. Run `./scripts/docs-list.sh`, then read the relevant `docs/` sections.
2. Identify impacted code paths and contracts.
3. Implement the smallest complete change that satisfies the documented behavior.
4. Run validation commands for touched layers.
5. Update docs/contracts/changelog when behavior or interfaces change.
6. Report what changed, what was validated, and any residual risk.

Treat tests and contract checks as acceptance criteria, not optional cleanup.

## 4) Change Boundaries

- Keep changes scoped to the requested outcome.
- Avoid unrelated refactors in the same change.
- Keep business logic out of CLI handlers (`internal/cli` should orchestrate, not decide).
- Do not invent behavior that is not specified in `docs/`.

## 5) Validation Commands

Baseline for most changes:
- `gofmt -w <edited-files>.go`
- `go test ./...`

Additional validation by change type:
- SQL query changes (`internal/store/sqlite/queries/*.sql`):
  - regenerate SQLC output (`sqlc generate`)
  - run `go test ./...`
- Migration changes (`migrations/*.sql`):
  - run integration coverage (at minimum `go test ./...`)
  - verify migration ordering and reversibility expectations
- JSON contract/output changes:
  - update `docs/contracts/*.json` where applicable
  - update golden files under `internal/cli/testdata/json_contracts`
  - run `go test ./...`

## 6) Definition of Done

A change is done only when all are true:
- behavior matches relevant specs/rules under `docs/`
- deterministic output/contracts are preserved where required
- required tests pass for impacted layers (normally `go test ./...`)
- docs/contracts are updated when externally observable behavior changes
- `skills/boring-budget-agent/*` is updated when CLI API command surfaces are added or changed in a breaking way
- `CHANGELOG.md` is updated for significant work under `Unreleased`
- the change remains atomic and scoped

## 7) Scope Control

Do not add unrelated features without updating the relevant spec documents in `docs/` first.

If you need to change operating policy (how agents execute), update this file too.

## 8) Commit and Changelog Policy

- Keep commits atomic: one logical change per commit.
- Do not mix unrelated feature/refactor/fix work.
- Use clear commit messages describing that single unit.
- Do not mark work complete without updating `CHANGELOG.md` for significant changes.

## 9) Go Formatting Policy

- Use `gofmt` for all Go code.
- Run `gofmt` on every edited `.go` file before completion.
- Do not introduce conflicting formatting standards/tools.
