package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"boring-budget/internal/cli/output"
)

const updateJSONContractsGoldenEnv = "BUDGETTO_UPDATE_GOLDEN"

func TestJSONContractsGoldenCoreCommands(t *testing.T) {
	t.Parallel()

	t.Run("entry_add", func(t *testing.T) {
		db := newCLITestDB(t)
		t.Cleanup(func() { _ = db.Close() })

		categoryID := insertTestCategory(t, db, "Food")
		labelID := insertTestLabel(t, db, "work")

		raw := executeEntryCmdRaw(t, db, output.FormatJSON, []string{
			"add",
			"--type", "expense",
			"--amount", "12.50",
			"--currency", "usd",
			"--date", "2026-02-01T08:15:00-03:00",
			"--category-id", int64ToString(categoryID),
			"--label-id", int64ToString(labelID),
			"--note", "coffee",
		})

		assertJSONContractGolden(t, "entry_add.golden.json", raw)
	})

	t.Run("cap_set", func(t *testing.T) {
		db := newCLITestDB(t)
		t.Cleanup(func() { _ = db.Close() })

		raw := executeCapCmdRaw(t, db, output.FormatJSON, []string{
			"set",
			"--month", "2026-02",
			"--amount", "450.00",
			"--currency", "usd",
		})

		assertJSONContractGolden(t, "cap_set.golden.json", raw)
	})

	t.Run("cap_show", func(t *testing.T) {
		db := newCLITestDB(t)
		t.Cleanup(func() { _ = db.Close() })

		setPayload := executeCapCmdJSON(t, db, []string{
			"set",
			"--month", "2026-02",
			"--amount", "450.00",
			"--currency", "USD",
		})
		if ok, _ := setPayload["ok"].(bool); !ok {
			t.Fatalf("expected cap set ok=true payload=%v", setPayload)
		}

		raw := executeCapCmdRaw(t, db, output.FormatJSON, []string{
			"show",
			"--month", "2026-02",
		})

		assertJSONContractGolden(t, "cap_show.golden.json", raw)
	})

	t.Run("cap_history", func(t *testing.T) {
		db := newCLITestDB(t)
		t.Cleanup(func() { _ = db.Close() })

		firstSet := executeCapCmdJSON(t, db, []string{
			"set",
			"--month", "2026-02",
			"--amount", "450.00",
			"--currency", "USD",
		})
		if ok, _ := firstSet["ok"].(bool); !ok {
			t.Fatalf("expected first cap set ok=true payload=%v", firstSet)
		}

		secondSet := executeCapCmdJSON(t, db, []string{
			"set",
			"--month", "2026-02",
			"--amount", "500.00",
			"--currency", "USD",
		})
		if ok, _ := secondSet["ok"].(bool); !ok {
			t.Fatalf("expected second cap set ok=true payload=%v", secondSet)
		}

		raw := executeCapCmdRaw(t, db, output.FormatJSON, []string{
			"history",
			"--month", "2026-02",
		})

		assertJSONContractGolden(t, "cap_history.golden.json", raw)
	})

	t.Run("entry_update", func(t *testing.T) {
		db := newCLITestDB(t)
		t.Cleanup(func() { _ = db.Close() })

		categoryA := insertTestCategory(t, db, "A")
		categoryB := insertTestCategory(t, db, "B")
		labelA := insertTestLabel(t, db, "alpha")
		labelB := insertTestLabel(t, db, "beta")

		addPayload := executeEntryCmdJSON(t, db, []string{
			"add",
			"--type", "expense",
			"--amount", "12.00",
			"--currency", "USD",
			"--date", "2026-02-01",
			"--category-id", int64ToString(categoryA),
			"--label-id", int64ToString(labelA),
			"--note", "before",
		})
		mustEntrySuccess(t, addPayload)
		addData := mustMap(t, addPayload["data"])
		entryID := int64(mustMap(t, addData["entry"])["id"].(float64))

		raw := executeEntryCmdRaw(t, db, output.FormatJSON, []string{
			"update", int64ToString(entryID),
			"--type", "income",
			"--amount", "35.00",
			"--currency", "EUR",
			"--date", "2026-02-05",
			"--category-id", int64ToString(categoryB),
			"--label-id", int64ToString(labelB),
			"--note", "after",
		})

		assertJSONContractGolden(t, "entry_update.golden.json", raw)
	})

	t.Run("card_add", func(t *testing.T) {
		db := newCLITestDB(t)
		t.Cleanup(func() { _ = db.Close() })

		raw := executeCardCmdRaw(t, db, output.FormatJSON, []string{
			"add",
			"--nickname", "Main Credit",
			"--description", "Primary credit card",
			"--last4", "1234",
			"--brand", "VISA",
			"--card-type", "credit",
			"--due-day", "15",
		})

		assertJSONContractGolden(t, "card_add.golden.json", raw)
	})

	t.Run("card_due_show", func(t *testing.T) {
		db := newCLITestDB(t)
		t.Cleanup(func() { _ = db.Close() })

		addPayload := executeCardCmdJSON(t, db, []string{
			"add",
			"--nickname", "Main Credit",
			"--description", "Primary credit card",
			"--last4", "1234",
			"--brand", "VISA",
			"--card-type", "credit",
			"--due-day", "15",
		})
		if ok, _ := addPayload["ok"].(bool); !ok {
			t.Fatalf("expected card add ok=true payload=%v", addPayload)
		}

		raw := executeCardCmdRaw(t, db, output.FormatJSON, []string{
			"due", "show",
			"--card-id", "1",
			"--as-of", "2026-02-10",
		})

		assertJSONContractGolden(t, "card_due_show.golden.json", raw)
	})

	t.Run("card_debt_show", func(t *testing.T) {
		db := newCLITestDB(t)
		t.Cleanup(func() { _ = db.Close() })

		addPayload := executeCardCmdJSON(t, db, []string{
			"add",
			"--nickname", "Main Credit",
			"--description", "Primary credit card",
			"--last4", "1234",
			"--brand", "VISA",
			"--card-type", "credit",
			"--due-day", "15",
		})
		if ok, _ := addPayload["ok"].(bool); !ok {
			t.Fatalf("expected card add ok=true payload=%v", addPayload)
		}

		mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
			"add",
			"--type", "expense",
			"--amount", "20.00",
			"--currency", "USD",
			"--date", "2026-02-01",
			"--payment-method", "card",
			"--card-id", "1",
		}))

		raw := executeCardCmdRaw(t, db, output.FormatJSON, []string{
			"debt", "show",
			"--card-id", "1",
		})

		assertJSONContractGolden(t, "card_debt_show.golden.json", raw)
	})

	t.Run("card_payment_add", func(t *testing.T) {
		db := newCLITestDB(t)
		t.Cleanup(func() { _ = db.Close() })

		addPayload := executeCardCmdJSON(t, db, []string{
			"add",
			"--nickname", "Main Credit",
			"--description", "Primary credit card",
			"--last4", "1234",
			"--brand", "VISA",
			"--card-type", "credit",
			"--due-day", "15",
		})
		if ok, _ := addPayload["ok"].(bool); !ok {
			t.Fatalf("expected card add ok=true payload=%v", addPayload)
		}

		mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
			"add",
			"--type", "expense",
			"--amount", "20.00",
			"--currency", "USD",
			"--date", "2026-02-01",
			"--payment-method", "card",
			"--card-id", "1",
		}))

		raw := executeCardCmdRaw(t, db, output.FormatJSON, []string{
			"payment", "add",
			"--card-id", "1",
			"--amount", "5.00",
			"--currency", "USD",
		})

		assertJSONContractGolden(t, "card_payment_add.golden.json", raw)
	})

	t.Run("report_monthly", func(t *testing.T) {
		db := newCLITestDB(t)
		t.Cleanup(func() { _ = db.Close() })

		categoryID := insertTestCategory(t, db, "Food")
		capSetPayload := executeCapCmdJSON(t, db, []string{
			"set",
			"--month", "2026-02",
			"--amount", "15.00",
			"--currency", "USD",
		})
		if ok, _ := capSetPayload["ok"].(bool); !ok {
			t.Fatalf("expected cap set ok=true payload=%v", capSetPayload)
		}

		mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
			"add",
			"--type", "income",
			"--amount", "50.00",
			"--currency", "USD",
			"--date", "2026-02-01",
		}))
		mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
			"add",
			"--type", "expense",
			"--amount", "12.00",
			"--currency", "USD",
			"--date", "2026-02-02",
			"--category-id", int64ToString(categoryID),
		}))

		raw := executeReportCmdRaw(t, db, output.FormatJSON, []string{
			"monthly",
			"--month", "2026-02",
			"--group-by", "month",
		})

		assertJSONContractGolden(t, "report_monthly.golden.json", raw)
	})

	t.Run("report_range", func(t *testing.T) {
		db := newCLITestDB(t)
		t.Cleanup(func() { _ = db.Close() })

		categoryID := insertTestCategory(t, db, "Food")
		labelID := insertTestLabel(t, db, "work")

		capSetPayload := executeCapCmdJSON(t, db, []string{
			"set",
			"--month", "2026-02",
			"--amount", "18.00",
			"--currency", "USD",
		})
		if ok, _ := capSetPayload["ok"].(bool); !ok {
			t.Fatalf("expected cap set ok=true payload=%v", capSetPayload)
		}

		mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
			"add",
			"--type", "income",
			"--amount", "50.00",
			"--currency", "USD",
			"--date", "2026-02-01",
			"--label-id", int64ToString(labelID),
		}))
		mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
			"add",
			"--type", "expense",
			"--amount", "12.00",
			"--currency", "USD",
			"--date", "2026-02-02",
			"--category-id", int64ToString(categoryID),
			"--label-id", int64ToString(labelID),
		}))
		mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
			"add",
			"--type", "expense",
			"--amount", "4.00",
			"--currency", "EUR",
			"--date", "2026-02-10",
			"--category-id", int64ToString(categoryID),
		}))

		raw := executeReportCmdRaw(t, db, output.FormatJSON, []string{
			"range",
			"--from", "2026-02-01",
			"--to", "2026-02-28",
			"--label-id", int64ToString(labelID),
			"--label-mode", "all",
			"--group-by", "day",
		})

		assertJSONContractGolden(t, "report_range.golden.json", raw)
	})

	t.Run("report_bimonthly", func(t *testing.T) {
		db := newCLITestDB(t)
		t.Cleanup(func() { _ = db.Close() })

		categoryID := insertTestCategory(t, db, "Rent")

		mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
			"add",
			"--type", "income",
			"--amount", "50.00",
			"--currency", "USD",
			"--date", "2026-02-01",
		}))
		mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
			"add",
			"--type", "expense",
			"--amount", "10.00",
			"--currency", "USD",
			"--date", "2026-03-03",
			"--category-id", int64ToString(categoryID),
		}))

		raw := executeReportCmdRaw(t, db, output.FormatJSON, []string{
			"bimonthly",
			"--month", "2026-02",
			"--group-by", "month",
		})

		assertJSONContractGolden(t, "report_bimonthly.golden.json", raw)
	})

	t.Run("report_quarterly", func(t *testing.T) {
		db := newCLITestDB(t)
		t.Cleanup(func() { _ = db.Close() })

		categoryID := insertTestCategory(t, db, "Rent")

		mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
			"add",
			"--type", "income",
			"--amount", "50.00",
			"--currency", "USD",
			"--date", "2026-02-01",
		}))
		mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
			"add",
			"--type", "expense",
			"--amount", "10.00",
			"--currency", "USD",
			"--date", "2026-03-03",
			"--category-id", int64ToString(categoryID),
		}))
		mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
			"add",
			"--type", "expense",
			"--amount", "5.00",
			"--currency", "USD",
			"--date", "2026-04-10",
			"--category-id", int64ToString(categoryID),
		}))

		raw := executeReportCmdRaw(t, db, output.FormatJSON, []string{
			"quarterly",
			"--month", "2026-02",
			"--group-by", "month",
		})

		assertJSONContractGolden(t, "report_quarterly.golden.json", raw)
	})

	t.Run("balance_show", func(t *testing.T) {
		db := newCLITestDB(t)
		t.Cleanup(func() { _ = db.Close() })

		labelID := insertTestLabel(t, db, "work")

		mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
			"add",
			"--type", "income",
			"--amount", "100.00",
			"--currency", "USD",
			"--date", "2026-01-10",
			"--label-id", int64ToString(labelID),
		}))
		mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
			"add",
			"--type", "expense",
			"--amount", "30.00",
			"--currency", "USD",
			"--date", "2026-02-05",
			"--label-id", int64ToString(labelID),
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

		raw := executeBalanceCmdRaw(t, db, output.FormatJSON, []string{
			"show",
			"--scope", "both",
			"--from", "2026-02-01",
			"--to", "2026-02-28",
			"--label-id", int64ToString(labelID),
			"--label-mode", "any",
		})

		assertJSONContractGolden(t, "balance_show.golden.json", raw)
	})

	t.Run("data_export_entries", func(t *testing.T) {
		db := newCLITestDB(t)
		t.Cleanup(func() { _ = db.Close() })

		mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
			"add",
			"--type", "income",
			"--amount", "90.00",
			"--currency", "USD",
			"--date", "2026-01-31",
			"--note", "salary",
		}))

		exportPath := filepath.Join(t.TempDir(), "exports", "entries.json")
		opts := &RootOptions{Output: output.FormatJSON, db: db}
		raw := executeDataCmdRawWithOptions(t, opts, []string{
			"export",
			"--resource", "entries",
			"--format", "json",
			"--file", exportPath,
		})

		if _, err := os.Stat(exportPath); err != nil {
			t.Fatalf("expected export file at %q: %v", exportPath, err)
		}

		assertJSONContractGolden(t, "data_export_entries.golden.json", raw)
	})

	t.Run("data_export_report", func(t *testing.T) {
		db := newCLITestDB(t)
		t.Cleanup(func() { _ = db.Close() })

		mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
			"add",
			"--type", "income",
			"--amount", "120.00",
			"--currency", "USD",
			"--date", "2026-02-01",
			"--note", "salary",
		}))
		mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
			"add",
			"--type", "expense",
			"--amount", "30.00",
			"--currency", "USD",
			"--date", "2026-02-05",
			"--note", "rent",
		}))

		exportPath := filepath.Join(t.TempDir(), "exports", "report.json")
		opts := &RootOptions{Output: output.FormatJSON, db: db}
		raw := executeDataCmdRawWithOptions(t, opts, []string{
			"export",
			"--resource", "report",
			"--format", "json",
			"--file", exportPath,
			"--report-scope", "monthly",
			"--report-month", "2026-02",
			"--report-group-by", "month",
		})

		if _, err := os.Stat(exportPath); err != nil {
			t.Fatalf("expected report export file at %q: %v", exportPath, err)
		}

		assertJSONContractGolden(t, "data_export_report.golden.json", raw)
	})

	t.Run("setup_init", func(t *testing.T) {
		db := newCLITestDB(t)
		t.Cleanup(func() { _ = db.Close() })

		raw := executeSetupCmdRaw(t, db, output.FormatJSON, []string{
			"init",
			"--default-currency", "usd",
			"--timezone", "UTC",
			"--opening-balance", "1000.00",
			"--opening-balance-date", "2026-02-11",
			"--month-cap", "500.00",
			"--month-cap-month", "2026-02",
		})

		assertJSONContractGolden(t, "setup_init.golden.json", raw)
	})
}

func assertJSONContractGolden(t *testing.T, fixtureName, rawJSON string) {
	t.Helper()

	var payload any
	if err := json.Unmarshal([]byte(rawJSON), &payload); err != nil {
		t.Fatalf("unmarshal raw json payload: %v raw=%s", err, rawJSON)
	}

	normalizedPayload := normalizeJSONContractValue("", payload)
	actual := marshalNormalizedJSON(t, normalizedPayload)

	goldenPath := filepath.Join(jsonContractsGoldenDir(t), fixtureName)
	if os.Getenv(updateJSONContractsGoldenEnv) == "1" {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("create golden dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, actual, 0o644); err != nil {
			t.Fatalf("write golden file: %v", err)
		}
	}

	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file %q: %v", goldenPath, err)
	}

	if !bytes.Equal(expected, actual) {
		t.Fatalf("golden mismatch for %s\nexpected:\n%s\nactual:\n%s", goldenPath, expected, actual)
	}
}

func marshalNormalizedJSON(t *testing.T, value any) []byte {
	t.Helper()

	buf := &bytes.Buffer{}
	encoder := json.NewEncoder(buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		t.Fatalf("marshal normalized payload: %v", err)
	}
	return buf.Bytes()
}

func normalizeJSONContractValue(key string, value any) any {
	switch typed := value.(type) {
	case map[string]any:
		normalized := make(map[string]any, len(typed))
		for childKey, childValue := range typed {
			normalized[childKey] = normalizeJSONContractValue(childKey, childValue)
		}
		return normalized
	case []any:
		normalized := make([]any, len(typed))
		for i := range typed {
			normalized[i] = normalizeJSONContractValue(key, typed[i])
		}
		return normalized
	case string:
		switch {
		case key == "timestamp_utc" || strings.HasSuffix(key, "_at_utc"):
			return "<timestamp_utc>"
		case isPathField(key):
			return filepath.Base(typed)
		default:
			return typed
		}
	default:
		return value
	}
}

func isPathField(key string) bool {
	switch key {
	case "file", "backup_file", "db_path", "restored_from":
		return true
	default:
		return false
	}
}

func jsonContractsGoldenDir(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime caller failed")
	}

	return filepath.Join(filepath.Dir(currentFile), "testdata", "json_contracts")
}

func int64ToString(value int64) string {
	return strconv.FormatInt(value, 10)
}

func executeSetupCmdRaw(t *testing.T, db *sql.DB, format string, args []string) string {
	t.Helper()

	opts := &RootOptions{Output: format, db: db}
	cmd := NewSetupCmd(opts)

	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute setup cmd %v: %v", args, err)
	}

	return strings.TrimSpace(buf.String())
}
