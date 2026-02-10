# Budgetto

Agent-friendly personal budgeting CLI built with Go and SQLite.

## Overview

Budgetto is a local-first CLI for tracking:
- expenses and earnings
- categories and labels
- monthly caps and overspend alerts
- reporting by date range, month, bimonthly, and quarterly presets

The project is designed for both humans and LLM agents, with deterministic command behavior and structured JSON output contracts.

## Project Docs

- Product plan: `docs/PRODUCT_PLAN.md`
- Technical blueprint: `docs/TECHNICAL_BLUEPRINT.md`
- Contributor/agent rules: `AGENTS.md`

## Key Principles

- Money is stored in integer minor units (`amount_minor`) with `currency_code`.
- Timestamps are stored in UTC and rendered in user-configured timezone.
- Overspend checks warn but do not block writes.
- Deletes are non-destructive where possible.
- Core commands support both human and JSON output modes.

## Current Status

Planning and architecture are defined. Next phase is project scaffolding:
1. Go module and CLI skeleton
2. SQLite migrations and WAL setup
3. Core domain/service/store layers
4. Contracted JSON responses for agent integration
