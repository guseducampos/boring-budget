package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"budgetto/internal/cli/output"
)

func TestEntryCommandJSONLifecycleAndFilters(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	categoryID := insertTestCategory(t, db, "Food")
	labelWorkID := insertTestLabel(t, db, "work")
	labelHomeID := insertTestLabel(t, db, "home")

	addOne := executeEntryCmdJSON(t, db, []string{
		"add",
		"--type", "expense",
		"--amount-minor", "1250",
		"--currency", "usd",
		"--date", "2026-02-01",
		"--category-id", strconv.FormatInt(categoryID, 10),
		"--label-id", strconv.FormatInt(labelWorkID, 10),
		"--note", "coffee",
	})

	if ok, _ := addOne["ok"].(bool); !ok {
		t.Fatalf("expected first add ok=true payload=%v", addOne)
	}

	addOneData := mustMap(t, addOne["data"])
	firstEntry := mustMap(t, addOneData["entry"])
	firstEntryID := int64(firstEntry["id"].(float64))
	if firstEntry["currency_code"].(string) != "USD" {
		t.Fatalf("expected normalized currency USD, got %v", firstEntry["currency_code"])
	}

	firstLabels := mustAnySlice(t, firstEntry["label_ids"])
	if len(firstLabels) != 1 || int64(firstLabels[0].(float64)) != labelWorkID {
		t.Fatalf("expected one label id %d, got %v", labelWorkID, firstLabels)
	}

	addTwo := executeEntryCmdJSON(t, db, []string{
		"add",
		"--type", "expense",
		"--amount-minor", "2100",
		"--currency", "USD",
		"--date", "2026-02-02T10:30:00-03:00",
		"--label-id", strconv.FormatInt(labelWorkID, 10),
		"--label-id", strconv.FormatInt(labelHomeID, 10),
	})
	if ok, _ := addTwo["ok"].(bool); !ok {
		t.Fatalf("expected second add ok=true payload=%v", addTwo)
	}

	addThree := executeEntryCmdJSON(t, db, []string{
		"add",
		"--type", "income",
		"--amount-minor", "5000",
		"--currency", "USD",
		"--date", "2026-02-03",
	})
	if ok, _ := addThree["ok"].(bool); !ok {
		t.Fatalf("expected third add ok=true payload=%v", addThree)
	}

	typeFiltered := executeEntryCmdJSON(t, db, []string{"list", "--type", "expense"})
	typeFilteredData := mustMap(t, typeFiltered["data"])
	if int(typeFilteredData["count"].(float64)) != 2 {
		t.Fatalf("expected expense count 2, got %v", typeFilteredData["count"])
	}

	allMode := executeEntryCmdJSON(t, db, []string{
		"list",
		"--label-id", strconv.FormatInt(labelWorkID, 10),
		"--label-id", strconv.FormatInt(labelHomeID, 10),
		"--label-mode", "all",
	})
	allModeData := mustMap(t, allMode["data"])
	if int(allModeData["count"].(float64)) != 1 {
		t.Fatalf("expected label-mode all count 1, got %v", allModeData["count"])
	}

	noneMode := executeEntryCmdJSON(t, db, []string{
		"list",
		"--label-id", strconv.FormatInt(labelHomeID, 10),
		"--label-mode", "none",
	})
	noneModeData := mustMap(t, noneMode["data"])
	if int(noneModeData["count"].(float64)) != 2 {
		t.Fatalf("expected label-mode none count 2, got %v", noneModeData["count"])
	}

	dateFiltered := executeEntryCmdJSON(t, db, []string{
		"list",
		"--from", "2026-02-02",
		"--to", "2026-02-02",
	})
	dateFilteredData := mustMap(t, dateFiltered["data"])
	if int(dateFilteredData["count"].(float64)) != 1 {
		t.Fatalf("expected date range count 1, got %v", dateFilteredData["count"])
	}

	deletePayload := executeEntryCmdJSON(t, db, []string{"delete", strconv.FormatInt(firstEntryID, 10)})
	if ok, _ := deletePayload["ok"].(bool); !ok {
		t.Fatalf("expected delete ok=true payload=%v", deletePayload)
	}
	deleteData := mustMap(t, deletePayload["data"])
	deleted := mustMap(t, deleteData["deleted"])
	if int64(deleted["entry_id"].(float64)) != firstEntryID {
		t.Fatalf("expected deleted entry id %d, got %v", firstEntryID, deleted["entry_id"])
	}

	finalExpense := executeEntryCmdJSON(t, db, []string{"list", "--type", "expense"})
	finalExpenseData := mustMap(t, finalExpense["data"])
	if int(finalExpenseData["count"].(float64)) != 1 {
		t.Fatalf("expected final expense count 1, got %v", finalExpenseData["count"])
	}
}

func TestEntryCommandJSONInvalidCurrencyCode(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	payload := executeEntryCmdJSON(t, db, []string{
		"add",
		"--type", "expense",
		"--amount-minor", "100",
		"--currency", "US",
		"--date", "2026-02-01",
	})

	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false payload=%v", payload)
	}

	errPayload := mustMap(t, payload["error"])
	if errPayload["code"].(string) != "INVALID_CURRENCY_CODE" {
		t.Fatalf("expected INVALID_CURRENCY_CODE, got %v", errPayload["code"])
	}
}

func TestEntryCommandJSONInvalidDateRange(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	payload := executeEntryCmdJSON(t, db, []string{"list", "--from", "2026-02-10", "--to", "2026-02-01"})
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false payload=%v", payload)
	}

	errPayload := mustMap(t, payload["error"])
	if errPayload["code"].(string) != "INVALID_DATE_RANGE" {
		t.Fatalf("expected INVALID_DATE_RANGE, got %v", errPayload["code"])
	}
}

func TestEntryCommandJSONNotFoundCategory(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	payload := executeEntryCmdJSON(t, db, []string{
		"add",
		"--type", "expense",
		"--amount-minor", "100",
		"--currency", "USD",
		"--date", "2026-02-01",
		"--category-id", "99999",
	})

	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false payload=%v", payload)
	}

	errPayload := mustMap(t, payload["error"])
	if errPayload["code"].(string) != "NOT_FOUND" {
		t.Fatalf("expected NOT_FOUND, got %v", errPayload["code"])
	}
}

func TestEntryCommandJSONDeleteInvalidID(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	payload := executeEntryCmdJSON(t, db, []string{"delete", "abc"})
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false payload=%v", payload)
	}

	errPayload := mustMap(t, payload["error"])
	if errPayload["code"].(string) != "INVALID_ARGUMENT" {
		t.Fatalf("expected INVALID_ARGUMENT, got %v", errPayload["code"])
	}
}

func TestEntryCommandJSONStorageErrorMapsDBError(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	payload := executeEntryCmdJSON(t, db, []string{"list"})
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false payload=%v", payload)
	}

	errPayload := mustMap(t, payload["error"])
	if errPayload["code"].(string) != "DB_ERROR" {
		t.Fatalf("expected DB_ERROR, got %v", errPayload["code"])
	}
}

func TestEntryCommandHumanOutput(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	out := executeEntryCmdRaw(t, db, output.FormatHuman, []string{
		"add",
		"--type", "expense",
		"--amount-minor", "2500",
		"--currency", "USD",
		"--date", "2026-02-01",
	})

	if !strings.Contains(out, "[OK] budgetto") {
		t.Fatalf("expected human output status line, got %q", out)
	}
	if !strings.Contains(out, "\"amount_minor\": 2500") {
		t.Fatalf("expected human output to include amount_minor, got %q", out)
	}
}

func executeEntryCmdJSON(t *testing.T, db *sql.DB, args []string) map[string]any {
	t.Helper()

	raw := executeEntryCmdRaw(t, db, output.FormatJSON, args)
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal entry payload: %v raw=%s", err, raw)
	}
	return payload
}

func executeEntryCmdRaw(t *testing.T, db *sql.DB, format string, args []string) string {
	t.Helper()

	opts := &RootOptions{Output: format, db: db}
	cmd := NewEntryCmd(opts)

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute entry cmd %v: %v", args, err)
	}

	return strings.TrimSpace(buf.String())
}

func insertTestCategory(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()

	result, err := db.ExecContext(context.Background(), `INSERT INTO categories (name) VALUES (?);`, name)
	if err != nil {
		t.Fatalf("insert test category: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("read inserted category id: %v", err)
	}

	return id
}

func insertTestLabel(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()

	result, err := db.ExecContext(context.Background(), `INSERT INTO labels (name) VALUES (?);`, name)
	if err != nil {
		t.Fatalf("insert test label: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("read inserted label id: %v", err)
	}

	return id
}

func mustAnySlice(t *testing.T, value any) []any {
	t.Helper()

	slice, ok := value.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", value)
	}
	return slice
}
