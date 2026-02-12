---
summary: Router for repo documentation; use this to quickly find the correct spec or contract doc before implementing changes.
read_when:
  - Starting any non-trivial task.
  - Unsure where product vs technical vs contract rules live.
---

# Documentation Router

This repository is optimized for agent workflows. Read docs in this order.

## 1) Quick discovery

Run:

```bash
./scripts/docs-list.sh
```

This prints markdown docs with `summary` and `read_when` metadata.

## 2) Canonical specs

- `docs/SPEC.md`
  - Single canonical product + technical specification.
  - Use for domain rules, architecture constraints, and delivery boundaries.

## 3) Contracts for automation

- `docs/contracts/README.md`
- `docs/contracts/errors.md`
- `docs/contracts/exit-codes.md`
- `docs/contracts/*.json`

Use these for deterministic CLI JSON behavior, warning/error handling, and exit-code mapping.

## 4) Docs update policy

When behavior changes:
1. Update `docs/SPEC.md` (or `docs/contracts/*` for contract-only changes).
2. Update tests/golden files as needed.
3. Update `CHANGELOG.md` under `Unreleased`.
