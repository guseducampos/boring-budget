# Budgetto CLI Product Plan (v3)

## 1) Product Vision

Build a Go CLI app that helps users track personal finances with clear visibility of income vs. expenses, flexible categorization, and proactive budget guardrails.

## 2) Core Product Decisions

- Transactions support any date: past, present, or future.
- Entries support multiple currencies.
- Overspending behavior: allow entry and warn (never block by default).
- Caps apply to expenses only.
- Balance views: both lifetime and selected-date-range.
- Orphan policy: uncategorized entries are always allowed.
- Orphan insight: optionally warn when uncategorized usage is too high.

## 3) Core Entities

- Transaction (income or expense)
- Category (dynamic, user-defined)
- Label (multi-label per transaction)
- Monthly expense cap
- Cap change history

## 4) Transaction Requirements

### Required fields

- Type (`income` or `expense`)
- Amount
- Currency
- Transaction date

### Optional fields

- Category
- Note
- Labels (0..n)

### Rules

- Transactions without category are treated as `Orphan`.
- Users can add, update, and delete transactions.

## 5) Categories

- Dynamic CRUD for categories.
- Categories can be used for both income and expenses.
- Deleting a category should preserve transactions (move to Orphan behavior or equivalent safe handling).

## 6) Labels

### Label management

- Create, list, rename, delete labels.
- Label names should be unique case-insensitively.

### Label assignment

- A transaction can have multiple labels.
- Transactions may have no labels.

### Label querying

- Filter by labels with operators:
  - `ANY` (at least one label matches)
  - `ALL` (must include all labels)
  - `NONE` (exclude labels, optional but recommended)
- Label filters must be combinable with category and date filters.

## 7) Budget Caps and Overspending

- Users can set a monthly expense cap.
- On adding/updating an expense:
  - Evaluate the cap for that transaction’s month.
  - If over cap, save entry and return warning with overspend amount.
- Cap modifications are allowed at any time.
- Reports must show cap change history for each month where updates occurred:
  - previous cap
  - new cap
  - change timestamp

## 8) Reporting Requirements

### Report structure

- Reports must always separate:
  - Earnings
  - Spending

### Time scopes

- Custom date range
- Monthly
- Bimonthly
- Quarterly

### Grouping

- Group output by date granularity (day/week/month).

### Filters

- By labels
- By categories
- By dates
- Any combination of the above

### Additional outputs

- Include Orphan entries in breakdowns.
- Show cap status and overspend information for applicable periods.

## 9) Balance Views

- Lifetime net balance.
- Selected date range net balance.
- User can choose one view or both.

## 10) Multi-Currency Behavior

- Every transaction includes a currency.
- User has a default currency (initially USD or EUR).
- MVP reporting recommendation:
  - Show totals grouped by currency.
- Future enhancement:
  - Optional converted totals with explicit FX rate strategy.

## 11) MVP Scope

1. Transaction CRUD with type, amount, currency, and transaction date.
2. Category CRUD (dynamic).
3. Label CRUD and multi-label assignment to transactions.
4. Combined filtering by labels, categories, and dates.
5. Monthly expense cap with allow+warn overspend behavior.
6. Cap change history visible in monthly reports.
7. Reports split into earnings vs spending, with date grouping.
8. Lifetime and date-range balance views.
9. Orphan support plus optional orphan overuse warning.

## 12) Post-MVP Candidates

- Converted multi-currency reporting with FX management.
- Forecasting and trend insights.
- CSV/JSON exports.
- Optional strict mode to reject entries when over cap.
