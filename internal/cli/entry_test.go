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

func TestEntryCommandJSONAddIncludesCapExceededWarning(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.ExecContext(context.Background(), `
		INSERT INTO monthly_caps (month_key, amount_minor, currency_code)
		VALUES ('2026-02', 5000, 'USD');
	`); err != nil {
		t.Fatalf("insert monthly cap: %v", err)
	}

	payload := executeEntryCmdJSON(t, db, []string{
		"add",
		"--type", "expense",
		"--amount-minor", "6200",
		"--currency", "USD",
		"--date", "2026-02-10",
	})
	if ok, _ := payload["ok"].(bool); !ok {
		t.Fatalf("expected ok=true payload=%v", payload)
	}

	warnings := mustAnySlice(t, payload["warnings"])
	if len(warnings) != 1 {
		t.Fatalf("expected exactly one warning, got %d (%v)", len(warnings), warnings)
	}

	firstWarning := mustMap(t, warnings[0])
	if firstWarning["code"].(string) != "CAP_EXCEEDED" {
		t.Fatalf("expected CAP_EXCEEDED warning, got %v", firstWarning["code"])
	}
	if firstWarning["message"].(string) != "Expense saved, monthly cap exceeded." {
		t.Fatalf("unexpected warning message: %v", firstWarning["message"])
	}

	details := mustMap(t, firstWarning["details"])
	if details["month_key"].(string) != "2026-02" {
		t.Fatalf("expected warning month_key 2026-02, got %v", details["month_key"])
	}

	capAmount := mustMap(t, details["cap_amount"])
	if int64(capAmount["amount_minor"].(float64)) != 5000 {
		t.Fatalf("expected cap_amount amount_minor 5000, got %v", capAmount["amount_minor"])
	}

	newSpendTotal := mustMap(t, details["new_spend_total"])
	if int64(newSpendTotal["amount_minor"].(float64)) != 6200 {
		t.Fatalf("expected new_spend_total amount_minor 6200, got %v", newSpendTotal["amount_minor"])
	}

	overspendAmount := mustMap(t, details["overspend_amount"])
	if int64(overspendAmount["amount_minor"].(float64)) != 1200 {
		t.Fatalf("expected overspend_amount amount_minor 1200, got %v", overspendAmount["amount_minor"])
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

func TestEntryCommandJSONUpdateLifecycle(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	categoryA := insertTestCategory(t, db, "A")
	categoryB := insertTestCategory(t, db, "B")
	labelA := insertTestLabel(t, db, "alpha")
	labelB := insertTestLabel(t, db, "beta")

	addPayload := executeEntryCmdJSON(t, db, []string{
		"add",
		"--type", "expense",
		"--amount-minor", "1200",
		"--currency", "USD",
		"--date", "2026-02-01",
		"--category-id", strconv.FormatInt(categoryA, 10),
		"--label-id", strconv.FormatInt(labelA, 10),
		"--note", "before",
	})
	if ok, _ := addPayload["ok"].(bool); !ok {
		t.Fatalf("expected add ok=true payload=%v", addPayload)
	}
	addData := mustMap(t, addPayload["data"])
	entryID := int64(mustMap(t, addData["entry"])["id"].(float64))

	updatePayload := executeEntryCmdJSON(t, db, []string{
		"update", strconv.FormatInt(entryID, 10),
		"--type", "income",
		"--amount-minor", "3500",
		"--currency", "EUR",
		"--date", "2026-02-05",
		"--category-id", strconv.FormatInt(categoryB, 10),
		"--label-id", strconv.FormatInt(labelB, 10),
		"--note", "after",
	})
	if ok, _ := updatePayload["ok"].(bool); !ok {
		t.Fatalf("expected update ok=true payload=%v", updatePayload)
	}
	updateData := mustMap(t, updatePayload["data"])
	updated := mustMap(t, updateData["entry"])
	if updated["type"].(string) != "income" || int64(updated["amount_minor"].(float64)) != 3500 || updated["currency_code"].(string) != "EUR" {
		t.Fatalf("unexpected updated values: %v", updated)
	}
	if int64(updated["category_id"].(float64)) != categoryB {
		t.Fatalf("expected category %d, got %v", categoryB, updated["category_id"])
	}
	labels := mustAnySlice(t, updated["label_ids"])
	if len(labels) != 1 || int64(labels[0].(float64)) != labelB {
		t.Fatalf("expected labels [%d], got %v", labelB, labels)
	}
	if updated["note"].(string) != "after" {
		t.Fatalf("expected note after, got %v", updated["note"])
	}

	clearPayload := executeEntryCmdJSON(t, db, []string{
		"update", strconv.FormatInt(entryID, 10),
		"--clear-category",
		"--clear-labels",
		"--clear-note",
	})
	if ok, _ := clearPayload["ok"].(bool); !ok {
		t.Fatalf("expected clear update ok=true payload=%v", clearPayload)
	}
	clearData := mustMap(t, clearPayload["data"])
	cleared := mustMap(t, clearData["entry"])
	if _, hasCategory := cleared["category_id"]; hasCategory {
		t.Fatalf("expected category_id omitted after clear, got %v", cleared["category_id"])
	}
	if _, hasLabels := cleared["label_ids"]; hasLabels {
		t.Fatalf("expected label_ids omitted after clear, got %v", cleared["label_ids"])
	}
	if _, hasNote := cleared["note"]; hasNote {
		t.Fatalf("expected note omitted after clear, got %v", cleared["note"])
	}
}

func TestEntryCommandJSONUpdateRequiresFields(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	payload := executeEntryCmdJSON(t, db, []string{"update", "1"})
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false payload=%v", payload)
	}
	errPayload := mustMap(t, payload["error"])
	if errPayload["code"].(string) != "INVALID_ARGUMENT" {
		t.Fatalf("expected INVALID_ARGUMENT, got %v", errPayload["code"])
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

	if !strings.Contains(out, "[OK] boring-budget") {
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
