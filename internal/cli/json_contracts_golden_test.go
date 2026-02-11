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

	"budgetto/internal/cli/output"
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
			"--amount-minor", "1250",
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
			"--amount-minor", "45000",
			"--currency", "usd",
		})

		assertJSONContractGolden(t, "cap_set.golden.json", raw)
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
			"--amount-minor", "1200",
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
			"--amount-minor", "3500",
			"--currency", "EUR",
			"--date", "2026-02-05",
			"--category-id", int64ToString(categoryB),
			"--label-id", int64ToString(labelB),
			"--note", "after",
		})

		assertJSONContractGolden(t, "entry_update.golden.json", raw)
	})

	t.Run("report_monthly", func(t *testing.T) {
		db := newCLITestDB(t)
		t.Cleanup(func() { _ = db.Close() })

		categoryID := insertTestCategory(t, db, "Food")
		capSetPayload := executeCapCmdJSON(t, db, []string{
			"set",
			"--month", "2026-02",
			"--amount-minor", "1500",
			"--currency", "USD",
		})
		if ok, _ := capSetPayload["ok"].(bool); !ok {
			t.Fatalf("expected cap set ok=true payload=%v", capSetPayload)
		}

		mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
			"add",
			"--type", "income",
			"--amount-minor", "5000",
			"--currency", "USD",
			"--date", "2026-02-01",
		}))
		mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
			"add",
			"--type", "expense",
			"--amount-minor", "1200",
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

	t.Run("data_export_entries", func(t *testing.T) {
		db := newCLITestDB(t)
		t.Cleanup(func() { _ = db.Close() })

		mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
			"add",
			"--type", "income",
			"--amount-minor", "9000",
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
			"--amount-minor", "12000",
			"--currency", "USD",
			"--date", "2026-02-01",
			"--note", "salary",
		}))
		mustEntrySuccess(t, executeEntryCmdJSON(t, db, []string{
			"add",
			"--type", "expense",
			"--amount-minor", "3000",
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
			"--opening-balance-minor", "100000",
			"--opening-balance-date", "2026-02-11",
			"--month-cap-minor", "50000",
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
