package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"boring-budget/internal/cli/output"
)

func TestBalanceShowJSONScopesAndFilters(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	labelWorkID := insertTestLabel(t, db, "work")

	mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
		"add",
		"--type", "income",
		"--amount", "100.00",
		"--currency", "USD",
		"--date", "2026-01-10",
		"--label-id", strconv.FormatInt(labelWorkID, 10),
	}))
	mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
		"add",
		"--type", "expense",
		"--amount", "30.00",
		"--currency", "USD",
		"--date", "2026-02-05",
		"--label-id", strconv.FormatInt(labelWorkID, 10),
	}))
	mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
		"add",
		"--type", "expense",
		"--amount", "5.00",
		"--currency", "USD",
		"--date", "2026-03-01",
	}))
	mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
		"add",
		"--type", "income",
		"--amount", "7.00",
		"--currency", "EUR",
		"--date", "2026-02-10",
	}))

	payload := executeBalanceCmdJSON(t, db, []string{
		"show",
		"--scope", "both",
		"--from", "2026-02-01",
		"--to", "2026-02-28",
		"--label-id", strconv.FormatInt(labelWorkID, 10),
		"--label-mode", "any",
	})
	if ok, _ := payload["ok"].(bool); !ok {
		t.Fatalf("expected balance show ok=true payload=%v", payload)
	}

	data := mustMap(t, payload["data"])
	if data["scope"].(string) != "both" {
		t.Fatalf("expected scope both, got %v", data["scope"])
	}

	lifetime := mustMap(t, data["lifetime"])
	lifetimeByCurrency := mustAnySlice(t, lifetime["by_currency"])
	if got := balanceNetForCurrency(t, lifetimeByCurrency, "USD"); got != 7000 {
		t.Fatalf("expected lifetime USD net 7000, got %d", got)
	}

	rangeView := mustMap(t, data["range"])
	if rangeView["from_utc"].(string) == "" {
		t.Fatalf("expected range from_utc to be present")
	}
	rangeByCurrency := mustAnySlice(t, rangeView["by_currency"])
	if got := balanceNetForCurrency(t, rangeByCurrency, "USD"); got != -3000 {
		t.Fatalf("expected range USD net -3000, got %d", got)
	}

	lifetimeOnly := executeBalanceCmdJSON(t, db, []string{"show", "--scope", "lifetime"})
	if ok, _ := lifetimeOnly["ok"].(bool); !ok {
		t.Fatalf("expected lifetime-only balance ok=true payload=%v", lifetimeOnly)
	}
	lifetimeOnlyData := mustMap(t, lifetimeOnly["data"])
	if _, ok := lifetimeOnlyData["range"]; ok {
		t.Fatalf("expected lifetime scope to omit range view")
	}
}

func TestBalanceShowJSONInvalidScope(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	payload := executeBalanceCmdJSON(t, db, []string{"show", "--scope", "month"})
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false payload=%v", payload)
	}

	errPayload := mustMap(t, payload["error"])
	if errPayload["code"].(string) != "INVALID_ARGUMENT" {
		t.Fatalf("expected INVALID_ARGUMENT, got %v", errPayload["code"])
	}
}

func TestBalanceShowJSONInvalidDateRange(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	payload := executeBalanceCmdJSON(t, db, []string{
		"show",
		"--scope", "range",
		"--from", "2026-03-10",
		"--to", "2026-03-01",
	})
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false payload=%v", payload)
	}

	errPayload := mustMap(t, payload["error"])
	if errPayload["code"].(string) != "INVALID_DATE_RANGE" {
		t.Fatalf("expected INVALID_DATE_RANGE, got %v", errPayload["code"])
	}
}

func TestBalanceShowJSONStorageErrorMapsDBError(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	payload := executeBalanceCmdJSON(t, db, []string{"show", "--scope", "both"})
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false payload=%v", payload)
	}

	errPayload := mustMap(t, payload["error"])
	if errPayload["code"].(string) != "DB_ERROR" {
		t.Fatalf("expected DB_ERROR, got %v", errPayload["code"])
	}
}

func TestBalanceShowHumanOutput(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
		"add",
		"--type", "income",
		"--amount", "1.00",
		"--currency", "USD",
		"--date", "2026-02-01",
	}))

	out := executeBalanceCmdRaw(t, db, output.FormatHuman, []string{"show", "--scope", "both"})
	if !strings.Contains(out, "[OK] boring-budget") {
		t.Fatalf("expected human output status line, got %q", out)
	}
	if !strings.Contains(out, "\"scope\": \"both\"") {
		t.Fatalf("expected human output to include scope, got %q", out)
	}
}

func executeBalanceCmdJSON(t *testing.T, db *sql.DB, args []string) map[string]any {
	t.Helper()

	raw := executeBalanceCmdRaw(t, db, output.FormatJSON, args)
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal balance payload: %v raw=%s", err, raw)
	}
	return payload
}

func executeBalanceCmdRaw(t *testing.T, db *sql.DB, format string, args []string) string {
	t.Helper()

	opts := &RootOptions{Output: format, db: db}
	cmd := NewBalanceCmd(opts)

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute balance cmd %v: %v", args, err)
	}

	return strings.TrimSpace(buf.String())
}

func balanceNetForCurrency(t *testing.T, rows []any, currency string) int64 {
	t.Helper()

	for _, row := range rows {
		item := mustMap(t, row)
		if item["currency_code"].(string) == currency {
			return int64(item["net_minor"].(float64))
		}
	}
	return 0
}
