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

func TestScheduleCommandJSONAddListDeleteLifecycle(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	categoryID := insertTestCategory(t, db, "Housing")

	addPayload := executeScheduleCmdJSON(t, db, []string{
		"add",
		"--name", "Rent",
		"--amount", "1200.00",
		"--currency", "usd",
		"--day", "10",
		"--start-month", "2026-01",
		"--end-month", "2026-03",
		"--category-id", strconv.FormatInt(categoryID, 10),
		"--note", "fixed expense",
	})
	if ok, _ := addPayload["ok"].(bool); !ok {
		t.Fatalf("expected add ok=true payload=%v", addPayload)
	}

	addData := mustMap(t, addPayload["data"])
	schedule := mustMap(t, addData["schedule"])
	scheduleID := int64(schedule["id"].(float64))
	if schedule["currency_code"].(string) != "USD" {
		t.Fatalf("expected normalized currency USD, got %v", schedule["currency_code"])
	}
	if int64(schedule["amount_minor"].(float64)) != 120000 {
		t.Fatalf("expected amount_minor 120000, got %v", schedule["amount_minor"])
	}

	listPayload := executeScheduleCmdJSON(t, db, []string{"list"})
	listData := mustMap(t, listPayload["data"])
	if int(listData["count"].(float64)) != 1 {
		t.Fatalf("expected list count 1, got %v", listData["count"])
	}

	deletePayload := executeScheduleCmdJSON(t, db, []string{"delete", strconv.FormatInt(scheduleID, 10)})
	if ok, _ := deletePayload["ok"].(bool); !ok {
		t.Fatalf("expected delete ok=true payload=%v", deletePayload)
	}

	listAfterDelete := executeScheduleCmdJSON(t, db, []string{"list"})
	listAfterDeleteData := mustMap(t, listAfterDelete["data"])
	if int(listAfterDeleteData["count"].(float64)) != 0 {
		t.Fatalf("expected active list count 0 after delete, got %v", listAfterDeleteData["count"])
	}

	listIncludeDeleted := executeScheduleCmdJSON(t, db, []string{"list", "--include-deleted"})
	includeDeletedData := mustMap(t, listIncludeDeleted["data"])
	if int(includeDeletedData["count"].(float64)) != 1 {
		t.Fatalf("expected include-deleted count 1, got %v", includeDeletedData["count"])
	}
	first := mustMap(t, mustAnySlice(t, includeDeletedData["schedules"])[0])
	if first["deleted_at_utc"] == nil {
		t.Fatalf("expected deleted_at_utc to be present in include-deleted output")
	}
}

func TestScheduleCommandJSONRunDryRunAndIdempotentExecution(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	addPayload := executeScheduleCmdJSON(t, db, []string{
		"add",
		"--name", "Gym",
		"--amount", "30.00",
		"--currency", "USD",
		"--day", "5",
		"--start-month", "2026-01",
	})
	if ok, _ := addPayload["ok"].(bool); !ok {
		t.Fatalf("expected add ok=true payload=%v", addPayload)
	}

	dryRunPayload := executeScheduleCmdJSON(t, db, []string{
		"run",
		"--through-date", "2026-03-20",
		"--dry-run",
	})
	if ok, _ := dryRunPayload["ok"].(bool); !ok {
		t.Fatalf("expected dry-run ok=true payload=%v", dryRunPayload)
	}
	dryRunData := mustMap(t, dryRunPayload["data"])
	dryRunResult := mustMap(t, dryRunData["run"])
	if int(dryRunResult["created_count"].(float64)) != 3 || int(dryRunResult["skipped_count"].(float64)) != 0 {
		t.Fatalf("unexpected dry-run counts: %v", dryRunResult)
	}

	if count := countRows(t, db, `SELECT COUNT(*) FROM transactions WHERE deleted_at_utc IS NULL;`); count != 0 {
		t.Fatalf("expected no transactions after dry-run, got %d", count)
	}

	firstRunPayload := executeScheduleCmdJSON(t, db, []string{
		"run",
		"--through-date", "2026-03-20",
	})
	if ok, _ := firstRunPayload["ok"].(bool); !ok {
		t.Fatalf("expected first run ok=true payload=%v", firstRunPayload)
	}
	firstRunData := mustMap(t, firstRunPayload["data"])
	firstRunResult := mustMap(t, firstRunData["run"])
	if int(firstRunResult["created_count"].(float64)) != 3 || int(firstRunResult["skipped_count"].(float64)) != 0 {
		t.Fatalf("unexpected first run counts: %v", firstRunResult)
	}

	if count := countRows(t, db, `SELECT COUNT(*) FROM transactions WHERE deleted_at_utc IS NULL;`); count != 3 {
		t.Fatalf("expected 3 created transactions, got %d", count)
	}
	if count := countRows(t, db, `SELECT COUNT(*) FROM transaction_payment_methods WHERE method_type = 'cash';`); count != 3 {
		t.Fatalf("expected 3 cash payment method rows, got %d", count)
	}

	secondRunPayload := executeScheduleCmdJSON(t, db, []string{
		"run",
		"--through-date", "2026-03-20",
	})
	if ok, _ := secondRunPayload["ok"].(bool); !ok {
		t.Fatalf("expected second run ok=true payload=%v", secondRunPayload)
	}
	secondRunData := mustMap(t, secondRunPayload["data"])
	secondRunResult := mustMap(t, secondRunData["run"])
	if int(secondRunResult["created_count"].(float64)) != 0 || int(secondRunResult["skipped_count"].(float64)) != 3 {
		t.Fatalf("unexpected second run counts: %v", secondRunResult)
	}
}

func TestScheduleCommandJSONRunWithScheduleID(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	firstAdd := executeScheduleCmdJSON(t, db, []string{
		"add",
		"--name", "Rent",
		"--amount", "1000.00",
		"--currency", "USD",
		"--day", "10",
		"--start-month", "2026-01",
	})
	secondAdd := executeScheduleCmdJSON(t, db, []string{
		"add",
		"--name", "Phone",
		"--amount", "80.00",
		"--currency", "USD",
		"--day", "12",
		"--start-month", "2026-01",
	})

	firstID := int64(mustMap(t, mustMap(t, firstAdd["data"])["schedule"])["id"].(float64))
	_ = secondAdd

	runPayload := executeScheduleCmdJSON(t, db, []string{
		"run",
		"--through-date", "2026-01-31",
		"--schedule-id", strconv.FormatInt(firstID, 10),
	})
	if ok, _ := runPayload["ok"].(bool); !ok {
		t.Fatalf("expected run ok=true payload=%v", runPayload)
	}

	if count := countRows(t, db, `SELECT COUNT(*) FROM transactions WHERE deleted_at_utc IS NULL;`); count != 1 {
		t.Fatalf("expected exactly one transaction for targeted schedule run, got %d", count)
	}
}

func TestScheduleCommandJSONRunRequiresThroughDate(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	payload := executeScheduleCmdJSON(t, db, []string{"run"})
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false payload=%v", payload)
	}

	errPayload := mustMap(t, payload["error"])
	if errPayload["code"].(string) != "INVALID_ARGUMENT" {
		t.Fatalf("expected INVALID_ARGUMENT, got %v", errPayload["code"])
	}
}

func executeScheduleCmdJSON(t *testing.T, db *sql.DB, args []string) map[string]any {
	t.Helper()

	raw := executeScheduleCmdRaw(t, db, output.FormatJSON, args)
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal schedule payload: %v raw=%s", err, raw)
	}
	return payload
}

func executeScheduleCmdRaw(t *testing.T, db *sql.DB, format string, args []string) string {
	t.Helper()

	opts := &RootOptions{Output: format, db: db}
	cmd := NewScheduleCmd(opts)

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute schedule cmd %v: %v", args, err)
	}

	return strings.TrimSpace(buf.String())
}

func countRows(t *testing.T, db *sql.DB, query string) int64 {
	t.Helper()

	var count int64
	if err := db.QueryRowContext(context.Background(), query).Scan(&count); err != nil {
		t.Fatalf("count rows query failed: %v", err)
	}
	return count
}
