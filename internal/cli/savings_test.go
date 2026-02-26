package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"boring-budget/internal/cli/output"
)

func TestSavingsCommandJSONAddAndShow(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
		"add",
		"--type", "income",
		"--amount", "100.00",
		"--currency", "USD",
		"--date", "2026-01-01",
	}))
	mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
		"add",
		"--type", "expense",
		"--amount", "30.00",
		"--currency", "USD",
		"--date", "2026-02-05",
	}))
	mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
		"add",
		"--type", "expense",
		"--amount", "90.00",
		"--currency", "USD",
		"--date", "2026-02-10",
	}))

	transfer := executeSavingsCmdJSON(t, db, []string{
		"transfer", "add",
		"--amount", "40.00",
		"--currency", "USD",
		"--date", "2026-02-01",
		"--note", "monthly reserve",
	})
	if ok, _ := transfer["ok"].(bool); !ok {
		t.Fatalf("expected transfer add ok=true payload=%v", transfer)
	}
	transferData := mustMap(t, transfer["data"])
	transferEvent := mustMap(t, transferData["event"])
	if transferEvent["event_type"].(string) != "transfer_to_savings" {
		t.Fatalf("expected transfer_to_savings event, got %v", transferEvent["event_type"])
	}

	entryAdd := executeSavingsCmdJSON(t, db, []string{
		"entry", "add",
		"--amount", "10.00",
		"--currency", "USD",
		"--date", "2026-02-02",
	})
	if ok, _ := entryAdd["ok"].(bool); !ok {
		t.Fatalf("expected entry add ok=true payload=%v", entryAdd)
	}
	entryAddData := mustMap(t, entryAdd["data"])
	entryEvent := mustMap(t, entryAddData["event"])
	if entryEvent["event_type"].(string) != "independent_add" {
		t.Fatalf("expected independent_add event, got %v", entryEvent["event_type"])
	}

	show := executeSavingsCmdJSON(t, db, []string{
		"show",
		"--scope", "both",
		"--from", "2026-02-01",
		"--to", "2026-02-28",
	})
	if ok, _ := show["ok"].(bool); !ok {
		t.Fatalf("expected show ok=true payload=%v", show)
	}

	data := mustMap(t, show["data"])
	if data["scope"].(string) != "both" {
		t.Fatalf("expected scope both, got %v", data["scope"])
	}

	lifetime := mustMap(t, data["lifetime"])
	lifetimeRows := mustAnySlice(t, lifetime["by_currency"])
	lifetimeBalance := savingsBalanceForCurrency(t, lifetimeRows, "USD")
	if lifetimeBalance.general != -1000 || lifetimeBalance.savings != 0 || lifetimeBalance.total != -1000 {
		t.Fatalf("unexpected lifetime balance: %+v", lifetimeBalance)
	}

	rangeData := mustMap(t, data["range"])
	rangeRows := mustAnySlice(t, rangeData["by_currency"])
	rangeBalance := savingsBalanceForCurrency(t, rangeRows, "USD")
	if rangeBalance.general != -11000 || rangeBalance.savings != 0 || rangeBalance.total != -11000 {
		t.Fatalf("unexpected range balance: %+v", rangeBalance)
	}
}

func TestSavingsCommandJSONInvalidScope(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	payload := executeSavingsCmdJSON(t, db, []string{"show", "--scope", "month"})
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false payload=%v", payload)
	}
	errPayload := mustMap(t, payload["error"])
	if errPayload["code"].(string) != "INVALID_ARGUMENT" {
		t.Fatalf("expected INVALID_ARGUMENT, got %v", errPayload["code"])
	}
}

func TestSavingsCommandJSONInvalidDateRange(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	payload := executeSavingsCmdJSON(t, db, []string{
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

func TestSavingsCommandJSONAddRequiresAmount(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	payload := executeSavingsCmdJSON(t, db, []string{
		"transfer", "add",
		"--currency", "USD",
		"--date", "2026-02-01",
	})
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false payload=%v", payload)
	}
	errPayload := mustMap(t, payload["error"])
	if errPayload["code"].(string) != "INVALID_ARGUMENT" {
		t.Fatalf("expected INVALID_ARGUMENT, got %v", errPayload["code"])
	}
}

func TestSavingsCommandJSONStorageErrorMapsDBError(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	payload := executeSavingsCmdJSON(t, db, []string{"show", "--scope", "both"})
	if ok, _ := payload["ok"].(bool); ok {
		t.Fatalf("expected ok=false payload=%v", payload)
	}
	errPayload := mustMap(t, payload["error"])
	if errPayload["code"].(string) != "DB_ERROR" {
		t.Fatalf("expected DB_ERROR, got %v", errPayload["code"])
	}
}

func TestSavingsCommandJSONAppliesLinkedAccountDefaults(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	addGeneral := executeBankAccountCmdJSON(t, db, []string{
		"add",
		"--alias", "General Wallet",
		"--last4", "1111",
	})
	generalID := int64(mustMap(t, mustMap(t, addGeneral["data"])["bank_account"])["id"].(float64))

	addSavings := executeBankAccountCmdJSON(t, db, []string{
		"add",
		"--alias", "Savings Wallet",
		"--last4", "2222",
	})
	savingsID := int64(mustMap(t, mustMap(t, addSavings["data"])["bank_account"])["id"].(float64))

	mustEntrySuccess(t, executeBankAccountCmdJSON(t, db, []string{
		"link", "set",
		"--target", "general_balance",
		"--account-id", int64ToString(generalID),
	}))
	mustEntrySuccess(t, executeBankAccountCmdJSON(t, db, []string{
		"link", "set",
		"--target", "savings",
		"--account-id", int64ToString(savingsID),
	}))

	transfer := executeSavingsCmdJSON(t, db, []string{
		"transfer", "add",
		"--amount", "10.00",
		"--currency", "USD",
		"--date", "2026-02-01",
	})
	mustEntrySuccess(t, transfer)
	transferEvent := mustMap(t, mustMap(t, transfer["data"])["event"])
	if int64(transferEvent["source_bank_account_id"].(float64)) != generalID {
		t.Fatalf("expected source_bank_account_id %d, got %v", generalID, transferEvent["source_bank_account_id"])
	}
	if int64(transferEvent["destination_bank_account_id"].(float64)) != savingsID {
		t.Fatalf("expected destination_bank_account_id %d, got %v", savingsID, transferEvent["destination_bank_account_id"])
	}

	entryAdd := executeSavingsCmdJSON(t, db, []string{
		"entry", "add",
		"--amount", "3.00",
		"--currency", "USD",
		"--date", "2026-02-02",
	})
	mustEntrySuccess(t, entryAdd)
	entryEvent := mustMap(t, mustMap(t, entryAdd["data"])["event"])
	if _, ok := entryEvent["source_bank_account_id"]; ok {
		t.Fatalf("expected no source_bank_account_id for independent_add, got %v", entryEvent["source_bank_account_id"])
	}
	if int64(entryEvent["destination_bank_account_id"].(float64)) != savingsID {
		t.Fatalf("expected destination_bank_account_id %d, got %v", savingsID, entryEvent["destination_bank_account_id"])
	}
}

func TestSavingsCommandJSONAllowsSameAccountForGeneralAndSavingsLinks(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	addUnified := executeBankAccountCmdJSON(t, db, []string{
		"add",
		"--alias", "Unified Wallet",
		"--last4", "3333",
	})
	unifiedID := int64(mustMap(t, mustMap(t, addUnified["data"])["bank_account"])["id"].(float64))

	mustEntrySuccess(t, executeBankAccountCmdJSON(t, db, []string{
		"link", "set",
		"--target", "general_balance",
		"--account-id", int64ToString(unifiedID),
	}))
	mustEntrySuccess(t, executeBankAccountCmdJSON(t, db, []string{
		"link", "set",
		"--target", "savings",
		"--account-id", int64ToString(unifiedID),
	}))

	transfer := executeSavingsCmdJSON(t, db, []string{
		"transfer", "add",
		"--amount", "12.00",
		"--currency", "USD",
		"--date", "2026-02-03",
	})
	mustEntrySuccess(t, transfer)
	event := mustMap(t, mustMap(t, transfer["data"])["event"])
	if int64(event["source_bank_account_id"].(float64)) != unifiedID {
		t.Fatalf("expected source_bank_account_id %d, got %v", unifiedID, event["source_bank_account_id"])
	}
	if int64(event["destination_bank_account_id"].(float64)) != unifiedID {
		t.Fatalf("expected destination_bank_account_id %d, got %v", unifiedID, event["destination_bank_account_id"])
	}
}

func TestSavingsCommandHumanOutput(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	out := executeSavingsCmdRaw(t, db, output.FormatHuman, []string{
		"entry", "add",
		"--amount", "5.00",
		"--currency", "USD",
		"--date", "2026-02-01",
	})
	if !strings.Contains(out, "[OK] boring-budget") {
		t.Fatalf("expected human output status line, got %q", out)
	}
	if !strings.Contains(out, "\"event_type\": \"independent_add\"") {
		t.Fatalf("expected human output to include event_type, got %q", out)
	}
}

func executeSavingsCmdJSON(t *testing.T, db *sql.DB, args []string) map[string]any {
	t.Helper()

	raw := executeSavingsCmdRaw(t, db, output.FormatJSON, args)
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal savings payload: %v raw=%s", err, raw)
	}
	return payload
}

func executeSavingsCmdRaw(t *testing.T, db *sql.DB, format string, args []string) string {
	t.Helper()

	opts := &RootOptions{Output: format, db: db}
	cmd := NewSavingsCmd(opts)

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute savings cmd %v: %v", args, err)
	}

	return strings.TrimSpace(buf.String())
}

type savingsBalanceSnapshot struct {
	general int64
	savings int64
	total   int64
}

func savingsBalanceForCurrency(t *testing.T, rows []any, currency string) savingsBalanceSnapshot {
	t.Helper()

	for _, row := range rows {
		item := mustMap(t, row)
		if item["currency_code"].(string) == currency {
			return savingsBalanceSnapshot{
				general: int64(item["general_balance_minor"].(float64)),
				savings: int64(item["savings_balance_minor"].(float64)),
				total:   int64(item["total_balance_minor"].(float64)),
			}
		}
	}
	return savingsBalanceSnapshot{}
}
