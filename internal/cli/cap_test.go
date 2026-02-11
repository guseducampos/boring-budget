package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"budgetto/internal/cli/output"
)

func TestCapCommandJSONSetShowHistory(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	firstSet := executeCapCmdJSON(t, db, []string{
		"set",
		"--month", "2026-02",
		"--amount-minor", "45000",
		"--currency", "usd",
	})
	if ok, _ := firstSet["ok"].(bool); !ok {
		t.Fatalf("expected first set ok=true payload=%v", firstSet)
	}

	firstData := mustMap(t, firstSet["data"])
	firstCap := mustMap(t, firstData["cap"])
	if firstCap["month_key"].(string) != "2026-02" {
		t.Fatalf("expected month_key 2026-02, got %v", firstCap["month_key"])
	}
	if int64(firstCap["amount_minor"].(float64)) != 45000 {
		t.Fatalf("expected amount_minor 45000, got %v", firstCap["amount_minor"])
	}
	if firstCap["currency_code"].(string) != "USD" {
		t.Fatalf("expected normalized currency USD, got %v", firstCap["currency_code"])
	}

	firstChange := mustMap(t, firstData["cap_change"])
	if old, ok := firstChange["old_amount_minor"]; !ok || old != nil {
		t.Fatalf("expected first old_amount_minor to be null, got %v", firstChange["old_amount_minor"])
	}

	secondSet := executeCapCmdJSON(t, db, []string{
		"set",
		"--month", "2026-02",
		"--amount-minor", "50000",
		"--currency", "USD",
	})
	if ok, _ := secondSet["ok"].(bool); !ok {
		t.Fatalf("expected second set ok=true payload=%v", secondSet)
	}

	show := executeCapCmdJSON(t, db, []string{"show", "--month", "2026-02"})
	showData := mustMap(t, show["data"])
	shownCap := mustMap(t, showData["cap"])
	if int64(shownCap["amount_minor"].(float64)) != 50000 {
		t.Fatalf("expected shown cap amount 50000, got %v", shownCap["amount_minor"])
	}

	history := executeCapCmdJSON(t, db, []string{"history", "--month", "2026-02"})
	historyData := mustMap(t, history["data"])
	if int(historyData["count"].(float64)) != 2 {
		t.Fatalf("expected history count 2, got %v", historyData["count"])
	}

	changes := mustAnySlice(t, historyData["changes"])
	if len(changes) != 2 {
		t.Fatalf("expected 2 history changes, got %d", len(changes))
	}

	firstHistory := mustMap(t, changes[0])
	if int64(firstHistory["new_amount_minor"].(float64)) != 45000 {
		t.Fatalf("expected first history new_amount_minor 45000, got %v", firstHistory["new_amount_minor"])
	}
	if old, ok := firstHistory["old_amount_minor"]; !ok || old != nil {
		t.Fatalf("expected first history old_amount_minor null, got %v", firstHistory["old_amount_minor"])
	}

	secondHistory := mustMap(t, changes[1])
	if int64(secondHistory["new_amount_minor"].(float64)) != 50000 {
		t.Fatalf("expected second history new_amount_minor 50000, got %v", secondHistory["new_amount_minor"])
	}
	if int64(secondHistory["old_amount_minor"].(float64)) != 45000 {
		t.Fatalf("expected second history old_amount_minor 45000, got %v", secondHistory["old_amount_minor"])
	}
}

func TestCapCommandJSONShowNotFound(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	payload := executeCapCmdJSON(t, db, []string{"show", "--month", "2026-02"})
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false payload=%v", payload)
	}

	errPayload := mustMap(t, payload["error"])
	if errPayload["code"].(string) != "NOT_FOUND" {
		t.Fatalf("expected NOT_FOUND, got %v", errPayload["code"])
	}
}

func TestCapCommandJSONInvalidCurrencyCode(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	payload := executeCapCmdJSON(t, db, []string{
		"set",
		"--month", "2026-02",
		"--amount-minor", "100",
		"--currency", "US",
	})
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false payload=%v", payload)
	}

	errPayload := mustMap(t, payload["error"])
	if errPayload["code"].(string) != "INVALID_CURRENCY_CODE" {
		t.Fatalf("expected INVALID_CURRENCY_CODE, got %v", errPayload["code"])
	}
}

func TestCapCommandJSONInvalidMonth(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	payload := executeCapCmdJSON(t, db, []string{
		"set",
		"--month", "2026-2",
		"--amount-minor", "100",
		"--currency", "USD",
	})
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false payload=%v", payload)
	}

	errPayload := mustMap(t, payload["error"])
	if errPayload["code"].(string) != "INVALID_ARGUMENT" {
		t.Fatalf("expected INVALID_ARGUMENT, got %v", errPayload["code"])
	}
}

func TestCapCommandJSONStorageErrorMapsDBError(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	payload := executeCapCmdJSON(t, db, []string{"history", "--month", "2026-02"})
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false payload=%v", payload)
	}

	errPayload := mustMap(t, payload["error"])
	if errPayload["code"].(string) != "DB_ERROR" {
		t.Fatalf("expected DB_ERROR, got %v", errPayload["code"])
	}
}

func TestCapCommandHumanOutput(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	out := executeCapCmdRaw(t, db, output.FormatHuman, []string{
		"set",
		"--month", "2026-02",
		"--amount-minor", "40000",
		"--currency", "USD",
	})

	if !strings.Contains(out, "[OK] budgetto") {
		t.Fatalf("expected human output status line, got %q", out)
	}
	if !strings.Contains(out, "\"amount_minor\": 40000") {
		t.Fatalf("expected human output to include amount_minor, got %q", out)
	}
}

func executeCapCmdJSON(t *testing.T, db *sql.DB, args []string) map[string]any {
	t.Helper()

	raw := executeCapCmdRaw(t, db, output.FormatJSON, args)
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal cap payload: %v raw=%s", err, raw)
	}
	return payload
}

func executeCapCmdRaw(t *testing.T, db *sql.DB, format string, args []string) string {
	t.Helper()

	opts := &RootOptions{Output: format, db: db}
	cmd := NewCapCmd(opts)

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute cap cmd %v: %v", args, err)
	}

	return strings.TrimSpace(buf.String())
}
