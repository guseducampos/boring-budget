---
summary: Command catalog for boring-budget CLI (all top-level groups and key subcommands).
read_when:
  - Looking up exact command names/areas quickly.
  - Keeping README value-first while preserving a command map.
---

# Command Catalog

Use runtime help for exact flags:

```bash
boring-budget --help
boring-budget <command> --help
```

## Global flags

```bash
--output human|json
--timezone <IANA TZ>
--db-path <sqlite file>
--migrations-dir <path>
```

## Command groups

```bash
boring-budget setup init|show
boring-budget category add|list|rename|delete
boring-budget label add|list|rename|delete
boring-budget bank-account add|list|update|delete
boring-budget bank-account link set|clear|list
boring-budget bank-account balance show
boring-budget card add|list|update|delete
boring-budget card due show|list
boring-budget card debt show
boring-budget card payment add
boring-budget entry add|update|list|delete
boring-budget savings transfer add
boring-budget savings entry add
boring-budget savings show
boring-budget schedule add|list|run|delete
boring-budget cap set|show|history
boring-budget report range|monthly|bimonthly|quarterly
boring-budget balance show
boring-budget data export|import|backup|restore
```

