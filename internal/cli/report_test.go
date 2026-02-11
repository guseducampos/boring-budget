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

func TestReportCommandJSONScopesAndFilters(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	categoryID := insertTestCategory(t, db, "Food")
	labelWorkID := insertTestLabel(t, db, "work")
	labelHomeID := insertTestLabel(t, db, "home")

	firstCap := executeCapCmdJSON(t, db, []string{"set", "--month", "2026-02", "--amount-minor", "1500", "--currency", "USD"})
	if ok, _ := firstCap["ok"].(bool); !ok {
		t.Fatalf("expected first cap set ok=true payload=%v", firstCap)
	}
	secondCap := executeCapCmdJSON(t, db, []string{"set", "--month", "2026-02", "--amount-minor", "1700", "--currency", "USD"})
	if ok, _ := secondCap["ok"].(bool); !ok {
		t.Fatalf("expected second cap set ok=true payload=%v", secondCap)
	}

	mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
		"add",
		"--type", "income",
		"--amount-minor", "5000",
		"--currency", "USD",
		"--date", "2026-02-01",
		"--label-id", strconv.FormatInt(labelWorkID, 10),
	}))
	mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
		"add",
		"--type", "expense",
		"--amount-minor", "1200",
		"--currency", "USD",
		"--date", "2026-02-02",
		"--category-id", strconv.FormatInt(categoryID, 10),
		"--label-id", strconv.FormatInt(labelWorkID, 10),
	}))
	mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
		"add",
		"--type", "expense",
		"--amount-minor", "800",
		"--currency", "USD",
		"--date", "2026-02-05",
		"--label-id", strconv.FormatInt(labelHomeID, 10),
	}))
	mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
		"add",
		"--type", "expense",
		"--amount-minor", "400",
		"--currency", "EUR",
		"--date", "2026-02-10",
		"--category-id", strconv.FormatInt(categoryID, 10),
	}))
	mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
		"add",
		"--type", "income",
		"--amount-minor", "1000",
		"--currency", "USD",
		"--date", "2026-03-01",
	}))

	monthly := executeReportCmdJSON(t, db, []string{"monthly", "--month", "2026-02", "--group-by", "month"})
	if ok, _ := monthly["ok"].(bool); !ok {
		t.Fatalf("expected monthly report ok=true payload=%v", monthly)
	}

	monthlyData := mustMap(t, monthly["data"])
	monthlyPeriod := mustMap(t, monthlyData["period"])
	if monthlyPeriod["scope"].(string) != "monthly" {
		t.Fatalf("expected period scope monthly, got %v", monthlyPeriod["scope"])
	}
	if monthlyPeriod["month_key"].(string) != "2026-02" {
		t.Fatalf("expected month_key 2026-02, got %v", monthlyPeriod["month_key"])
	}

	earnings := mustMap(t, monthlyData["earnings"])
	earningTotals := mustAnySlice(t, earnings["by_currency"])
	if got := reportTotalForCurrency(t, earningTotals, "USD"); got != 5000 {
		t.Fatalf("expected monthly earnings USD=5000, got %d", got)
	}

	spending := mustMap(t, monthlyData["spending"])
	spendingTotals := mustAnySlice(t, spending["by_currency"])
	if got := reportTotalForCurrency(t, spendingTotals, "USD"); got != 2000 {
		t.Fatalf("expected monthly spending USD=2000, got %d", got)
	}
	if got := reportTotalForCurrency(t, spendingTotals, "EUR"); got != 400 {
		t.Fatalf("expected monthly spending EUR=400, got %d", got)
	}
	spendingCategories := mustAnySlice(t, spending["categories"])
	foundFoodCategory := false
	for _, rawCategory := range spendingCategories {
		category := mustMap(t, rawCategory)
		if category["category_key"].(string) == "category:"+strconv.FormatInt(categoryID, 10) && category["category_label"].(string) == "Food" {
			foundFoodCategory = true
		}
	}
	if !foundFoodCategory {
		t.Fatalf("expected spending category label Food, got %v", spendingCategories)
	}

	net := mustMap(t, monthlyData["net"])
	netTotals := mustAnySlice(t, net["by_currency"])
	if got := reportTotalForCurrency(t, netTotals, "USD"); got != 3000 {
		t.Fatalf("expected monthly net USD=3000, got %d", got)
	}
	if got := reportTotalForCurrency(t, netTotals, "EUR"); got != -400 {
		t.Fatalf("expected monthly net EUR=-400, got %d", got)
	}

	capStatus := mustAnySlice(t, monthlyData["cap_status"])
	if len(capStatus) != 1 {
		t.Fatalf("expected one cap status item, got %d (%v)", len(capStatus), capStatus)
	}
	firstCapStatus := mustMap(t, capStatus[0])
	if int64(firstCapStatus["spend_total_minor"].(float64)) != 2000 {
		t.Fatalf("expected spend_total_minor 2000, got %v", firstCapStatus["spend_total_minor"])
	}
	if int64(firstCapStatus["overspend_minor"].(float64)) != 300 {
		t.Fatalf("expected overspend_minor 300, got %v", firstCapStatus["overspend_minor"])
	}

	capChanges := mustAnySlice(t, monthlyData["cap_changes"])
	if len(capChanges) != 2 {
		t.Fatalf("expected two cap changes, got %d", len(capChanges))
	}

	ranged := executeReportCmdJSON(t, db, []string{
		"range",
		"--from", "2026-02-01",
		"--to", "2026-02-28",
		"--label-id", strconv.FormatInt(labelWorkID, 10),
		"--label-mode", "all",
		"--group-by", "day",
	})
	if ok, _ := ranged["ok"].(bool); !ok {
		t.Fatalf("expected range report ok=true payload=%v", ranged)
	}

	rangeData := mustMap(t, ranged["data"])
	rangeEarnings := mustMap(t, rangeData["earnings"])
	rangeSpending := mustMap(t, rangeData["spending"])
	if got := reportTotalForCurrency(t, mustAnySlice(t, rangeEarnings["by_currency"]), "USD"); got != 5000 {
		t.Fatalf("expected filtered range earnings USD=5000, got %d", got)
	}
	if got := reportTotalForCurrency(t, mustAnySlice(t, rangeSpending["by_currency"]), "USD"); got != 1200 {
		t.Fatalf("expected filtered range spending USD=1200, got %d", got)
	}

	bimonthly := executeReportCmdJSON(t, db, []string{"bimonthly", "--month", "2026-02"})
	if ok, _ := bimonthly["ok"].(bool); !ok {
		t.Fatalf("expected bimonthly report ok=true payload=%v", bimonthly)
	}
	bimonthlyData := mustMap(t, bimonthly["data"])
	bimonthlyPeriod := mustMap(t, bimonthlyData["period"])
	if bimonthlyPeriod["scope"].(string) != "bimonthly" {
		t.Fatalf("expected bimonthly scope, got %v", bimonthlyPeriod["scope"])
	}

	quarterly := executeReportCmdJSON(t, db, []string{"quarterly", "--month", "2026-02"})
	if ok, _ := quarterly["ok"].(bool); !ok {
		t.Fatalf("expected quarterly report ok=true payload=%v", quarterly)
	}
	quarterlyData := mustMap(t, quarterly["data"])
	quarterlyPeriod := mustMap(t, quarterlyData["period"])
	if quarterlyPeriod["scope"].(string) != "quarterly" {
		t.Fatalf("expected quarterly scope, got %v", quarterlyPeriod["scope"])
	}
}

func TestReportCommandJSONRangeRequiresFromAndTo(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	payload := executeReportCmdJSON(t, db, []string{"range", "--from", "2026-02-01"})
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false payload=%v", payload)
	}

	errPayload := mustMap(t, payload["error"])
	if errPayload["code"].(string) != "INVALID_ARGUMENT" {
		t.Fatalf("expected INVALID_ARGUMENT, got %v", errPayload["code"])
	}
}

func TestReportCommandJSONInvalidGroupBy(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	payload := executeReportCmdJSON(t, db, []string{"monthly", "--month", "2026-02", "--group-by", "year"})
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false payload=%v", payload)
	}

	errPayload := mustMap(t, payload["error"])
	if errPayload["code"].(string) != "INVALID_ARGUMENT" {
		t.Fatalf("expected INVALID_ARGUMENT, got %v", errPayload["code"])
	}
}

func TestReportCommandJSONStorageErrorMapsDBError(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	payload := executeReportCmdJSON(t, db, []string{"monthly", "--month", "2026-02"})
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false payload=%v", payload)
	}

	errPayload := mustMap(t, payload["error"])
	if errPayload["code"].(string) != "DB_ERROR" {
		t.Fatalf("expected DB_ERROR, got %v", errPayload["code"])
	}
}

func TestReportCommandHumanOutput(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
		"add",
		"--type", "income",
		"--amount-minor", "100",
		"--currency", "USD",
		"--date", "2026-02-01",
	}))

	out := executeReportCmdRaw(t, db, output.FormatHuman, []string{"monthly", "--month", "2026-02"})
	if !strings.Contains(out, "[OK] boring-budget") {
		t.Fatalf("expected human output status line, got %q", out)
	}
	if !strings.Contains(out, "\"grouping\": \"month\"") {
		t.Fatalf("expected human output to include grouping, got %q", out)
	}
}

func executeReportCmdJSON(t *testing.T, db *sql.DB, args []string) map[string]any {
	t.Helper()

	raw := executeReportCmdRaw(t, db, output.FormatJSON, args)
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal report payload: %v raw=%s", err, raw)
	}
	return payload
}

func executeReportCmdRaw(t *testing.T, db *sql.DB, format string, args []string) string {
	t.Helper()

	opts := &RootOptions{Output: format, db: db}
	cmd := NewReportCmd(opts)

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute report cmd %v: %v", args, err)
	}

	return strings.TrimSpace(buf.String())
}

func reportTotalForCurrency(t *testing.T, rows []any, currency string) int64 {
	t.Helper()

	for _, row := range rows {
		item := mustMap(t, row)
		if item["currency_code"].(string) == currency {
			return int64(item["total_minor"].(float64))
		}
	}
	return 0
}

func mustEntrySuccess(t *testing.T, payload map[string]any) {
	t.Helper()

	if ok, _ := payload["ok"].(bool); !ok {
		t.Fatalf("expected entry command success payload=%v", payload)
	}
}
