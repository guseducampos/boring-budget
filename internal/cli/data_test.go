package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"budgetto/internal/cli/output"
	"budgetto/internal/domain"
	sqlitestore "budgetto/internal/store/sqlite"
)

type dataExportFile struct {
	Entries []dataExportEntry `json:"entries"`
}

type dataExportEntry struct {
	Type               string  `json:"type"`
	AmountMinor        int64   `json:"amount_minor"`
	CurrencyCode       string  `json:"currency_code"`
	TransactionDateUTC string  `json:"transaction_date_utc"`
	CategoryID         *int64  `json:"category_id,omitempty"`
	LabelIDs           []int64 `json:"label_ids,omitempty"`
	Note               string  `json:"note,omitempty"`
}

type dataExportReportFile struct {
	Report   domain.Report    `json:"report"`
	Warnings []domain.Warning `json:"warnings"`
}

func TestDataCommandJSONExportImportIdempotent(t *testing.T) {
	t.Parallel()

	sourceDB := newCLITestDB(t)
	t.Cleanup(func() { _ = sourceDB.Close() })

	mustEntrySuccess(t, executeEntryCmdJSON(t, sourceDB, []string{
		"add",
		"--type", "income",
		"--amount-minor", "9000",
		"--currency", "USD",
		"--date", "2026-01-31",
		"--note", "salary",
	}))
	mustEntrySuccess(t, executeEntryCmdJSON(t, sourceDB, []string{
		"add",
		"--type", "expense",
		"--amount-minor", "2500",
		"--currency", "USD",
		"--date", "2026-02-01T08:15:00-03:00",
		"--note", "groceries",
	}))

	exportPath := filepath.Join(t.TempDir(), "exports", "entries.json")
	sourceOpts := &RootOptions{Output: output.FormatJSON, db: sourceDB}
	exportPayload := executeDataCmdJSONWithOptions(t, sourceOpts, []string{
		"export",
		"--format", "json",
		"--file", exportPath,
	})

	assertSuccessJSONEnvelope(t, exportPayload)
	exportData := mustMap(t, exportPayload["data"])
	if int64(exportData["exported"].(float64)) != 2 {
		t.Fatalf("expected exported=2, got %v", exportData["exported"])
	}
	if exportData["format"].(string) != "json" {
		t.Fatalf("expected format=json, got %v", exportData["format"])
	}
	if exportData["file"].(string) != exportPath {
		t.Fatalf("expected file %q, got %v", exportPath, exportData["file"])
	}

	exportFile := readExportFile(t, exportPath)
	if len(exportFile.Entries) != 2 {
		t.Fatalf("expected two exported entries, got %d", len(exportFile.Entries))
	}

	first := exportFile.Entries[0]
	if first.Type != "income" || first.AmountMinor != 9000 || first.CurrencyCode != "USD" || first.TransactionDateUTC != "2026-01-31T00:00:00Z" || first.Note != "salary" {
		t.Fatalf("unexpected first exported entry: %+v", first)
	}
	if first.CategoryID != nil || len(first.LabelIDs) != 0 {
		t.Fatalf("expected first entry without category/labels, got %+v", first)
	}

	second := exportFile.Entries[1]
	if second.Type != "expense" || second.AmountMinor != 2500 || second.CurrencyCode != "USD" || second.TransactionDateUTC != "2026-02-01T11:15:00Z" || second.Note != "groceries" {
		t.Fatalf("unexpected second exported entry: %+v", second)
	}
	if second.CategoryID != nil || len(second.LabelIDs) != 0 {
		t.Fatalf("expected second entry without category/labels, got %+v", second)
	}

	targetDB := newCLITestDB(t)
	t.Cleanup(func() { _ = targetDB.Close() })
	targetOpts := &RootOptions{Output: output.FormatJSON, db: targetDB}

	firstImport := executeDataCmdJSONWithOptions(t, targetOpts, []string{
		"import",
		"--format", "json",
		"--file", exportPath,
	})
	assertSuccessJSONEnvelope(t, firstImport)
	firstImportData := mustMap(t, firstImport["data"])
	if int64(firstImportData["imported"].(float64)) != 2 {
		t.Fatalf("expected imported=2, got %v", firstImportData["imported"])
	}
	if int64(firstImportData["skipped"].(float64)) != 0 {
		t.Fatalf("expected skipped=0, got %v", firstImportData["skipped"])
	}
	if idempotent, _ := firstImportData["idempotent"].(bool); idempotent {
		t.Fatalf("expected idempotent=false on first import, got true")
	}

	idempotentImport := executeDataCmdJSONWithOptions(t, targetOpts, []string{
		"import",
		"--format", "json",
		"--file", exportPath,
		"--idempotent",
	})
	assertSuccessJSONEnvelope(t, idempotentImport)
	idempotentData := mustMap(t, idempotentImport["data"])
	if int64(idempotentData["imported"].(float64)) != 0 {
		t.Fatalf("expected imported=0 on idempotent re-import, got %v", idempotentData["imported"])
	}
	if int64(idempotentData["skipped"].(float64)) != 2 {
		t.Fatalf("expected skipped=2 on idempotent re-import, got %v", idempotentData["skipped"])
	}
	if idempotent, _ := idempotentData["idempotent"].(bool); !idempotent {
		t.Fatalf("expected idempotent=true on second import, got false")
	}

	if count := activeTransactionCount(t, targetDB); count != 2 {
		t.Fatalf("expected imported transaction count 2, got %d", count)
	}
}

func TestDataCommandJSONBackupRestore(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "budgetto.db")
	migrationsDir := cliMigrationsPath(t)

	db, err := sqlitestore.OpenAndMigrate(context.Background(), dbPath, migrationsDir)
	if err != nil {
		t.Fatalf("open and migrate db for backup/restore: %v", err)
	}

	opts := &RootOptions{
		Output:        output.FormatJSON,
		DBPath:        dbPath,
		MigrationsDir: migrationsDir,
		db:            db,
	}
	t.Cleanup(func() {
		if opts.db != nil {
			_ = opts.db.Close()
		}
	})

	mustEntrySuccess(t, executeEntryCmdJSON(t, opts.db, []string{
		"add",
		"--type", "income",
		"--amount-minor", "1000",
		"--currency", "USD",
		"--date", "2026-02-01",
		"--note", "before-backup",
	}))
	if count := activeTransactionCount(t, opts.db); count != 1 {
		t.Fatalf("expected one transaction before backup, got %d", count)
	}

	backupPath := filepath.Join(tempDir, "snapshots", "budgetto-backup.sqlite")
	backupPayload := executeDataCmdJSONWithOptions(t, opts, []string{
		"backup",
		"--file", backupPath,
	})
	assertSuccessJSONEnvelope(t, backupPayload)
	backupData := mustMap(t, backupPayload["data"])
	if backupData["backup_file"].(string) != backupPath {
		t.Fatalf("expected backup_file %q, got %v", backupPath, backupData["backup_file"])
	}

	backupInfo, err := os.Stat(backupPath)
	if err != nil {
		t.Fatalf("stat backup file: %v", err)
	}
	if backupInfo.Size() == 0 {
		t.Fatalf("expected non-empty backup file at %q", backupPath)
	}

	mustEntrySuccess(t, executeEntryCmdJSON(t, opts.db, []string{
		"add",
		"--type", "expense",
		"--amount-minor", "200",
		"--currency", "USD",
		"--date", "2026-02-02",
		"--note", "after-backup",
	}))
	if count := activeTransactionCount(t, opts.db); count != 2 {
		t.Fatalf("expected two transactions before restore, got %d", count)
	}

	restorePayload := executeDataCmdJSONWithOptions(t, opts, []string{
		"restore",
		"--file", backupPath,
	})
	assertSuccessJSONEnvelope(t, restorePayload)
	restoreData := mustMap(t, restorePayload["data"])
	if restoreData["restored_from"].(string) != backupPath {
		t.Fatalf("expected restored_from %q, got %v", backupPath, restoreData["restored_from"])
	}
	if restoreData["db_path"].(string) != dbPath {
		t.Fatalf("expected db_path %q, got %v", dbPath, restoreData["db_path"])
	}

	if count := activeTransactionCount(t, opts.db); count != 1 {
		t.Fatalf("expected one transaction after restore, got %d", count)
	}
	if count := activeTransactionCountByNote(t, opts.db, "before-backup"); count != 1 {
		t.Fatalf("expected before-backup entry after restore, got %d", count)
	}
	if count := activeTransactionCountByNote(t, opts.db, "after-backup"); count != 0 {
		t.Fatalf("expected after-backup entry to be removed by restore, got %d", count)
	}
}

func TestDataCommandJSONExportReport(t *testing.T) {
	t.Parallel()

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
	payload := executeDataCmdJSONWithOptions(t, opts, []string{
		"export",
		"--resource", "report",
		"--format", "json",
		"--file", exportPath,
		"--report-scope", "monthly",
		"--report-month", "2026-02",
		"--report-group-by", "month",
	})

	ok, _ := payload["ok"].(bool)
	if !ok {
		t.Fatalf("expected ok=true payload=%v", payload)
	}
	if payload["error"] != nil {
		t.Fatalf("expected error=null payload=%v", payload)
	}

	warnings := mustAnySlice(t, payload["warnings"])
	if len(warnings) != 1 {
		t.Fatalf("expected one warning in report export payload, got %v", warnings)
	}
	firstWarning := mustMap(t, warnings[0])
	if firstWarning["code"].(string) != domain.WarningCodeOrphanSpendingExceeded {
		t.Fatalf("expected orphan spending warning, got %v", firstWarning)
	}

	data := mustMap(t, payload["data"])
	if data["resource"].(string) != "report" {
		t.Fatalf("expected resource=report, got %v", data["resource"])
	}
	if data["format"].(string) != "json" {
		t.Fatalf("expected format=json, got %v", data["format"])
	}
	if data["file"].(string) != exportPath {
		t.Fatalf("expected file %q, got %v", exportPath, data["file"])
	}
	if data["grouping"].(string) != "month" {
		t.Fatalf("expected grouping=month, got %v", data["grouping"])
	}

	period := mustMap(t, data["period"])
	if period["scope"].(string) != "monthly" {
		t.Fatalf("expected period.scope=monthly, got %v", period["scope"])
	}
	if period["month_key"].(string) != "2026-02" {
		t.Fatalf("expected period.month_key=2026-02, got %v", period["month_key"])
	}

	reportFile := readReportExportFile(t, exportPath)
	if reportFile.Report.Period.Scope != domain.ReportScopeMonthly {
		t.Fatalf("expected exported report scope monthly, got %q", reportFile.Report.Period.Scope)
	}
	if len(reportFile.Report.Earnings.ByCurrency) != 1 || reportFile.Report.Earnings.ByCurrency[0].TotalMinor != 12000 {
		t.Fatalf("unexpected report earnings: %+v", reportFile.Report.Earnings.ByCurrency)
	}
	if len(reportFile.Report.Spending.ByCurrency) != 1 || reportFile.Report.Spending.ByCurrency[0].TotalMinor != 3000 {
		t.Fatalf("unexpected report spending: %+v", reportFile.Report.Spending.ByCurrency)
	}
	if len(reportFile.Warnings) != 1 || reportFile.Warnings[0].Code != domain.WarningCodeOrphanSpendingExceeded {
		t.Fatalf("expected warning in exported report file, got %+v", reportFile.Warnings)
	}
}

func executeDataCmdJSONWithOptions(t *testing.T, opts *RootOptions, args []string) map[string]any {
	t.Helper()

	raw := executeDataCmdRawWithOptions(t, opts, args)
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("unmarshal data payload: %v raw=%s", err, raw)
	}
	return payload
}

func executeDataCmdRawWithOptions(t *testing.T, opts *RootOptions, args []string) string {
	t.Helper()

	cmd := NewDataCmd(opts)
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)

	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("execute data cmd %v: %v", args, err)
	}

	return strings.TrimSpace(buf.String())
}

func readExportFile(t *testing.T, filePath string) dataExportFile {
	t.Helper()

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read export file: %v", err)
	}

	payload := dataExportFile{}
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatalf("unmarshal export file: %v", err)
	}

	return payload
}

func readReportExportFile(t *testing.T, filePath string) dataExportReportFile {
	t.Helper()

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read report export file: %v", err)
	}

	payload := dataExportReportFile{}
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatalf("unmarshal report export file: %v", err)
	}

	return payload
}

func assertSuccessJSONEnvelope(t *testing.T, payload map[string]any) {
	t.Helper()

	ok, _ := payload["ok"].(bool)
	if !ok {
		t.Fatalf("expected ok=true payload=%v", payload)
	}

	if payload["error"] != nil {
		t.Fatalf("expected error=null on success payload=%v", payload)
	}

	warnings := mustAnySlice(t, payload["warnings"])
	if len(warnings) != 0 {
		t.Fatalf("expected warnings=[], got %v", warnings)
	}

	meta := mustMap(t, payload["meta"])
	if meta["api_version"].(string) != output.APIVersionV1 {
		t.Fatalf("expected api_version=%s, got %v", output.APIVersionV1, meta["api_version"])
	}

	timestamp, _ := meta["timestamp_utc"].(string)
	if strings.TrimSpace(timestamp) == "" {
		t.Fatalf("expected non-empty timestamp_utc")
	}
	if _, err := time.Parse(time.RFC3339Nano, timestamp); err != nil {
		t.Fatalf("expected RFC3339 timestamp_utc, got %q err=%v", timestamp, err)
	}
}

func activeTransactionCount(t *testing.T, db *sql.DB) int64 {
	t.Helper()

	var count int64
	if err := db.QueryRowContext(
		context.Background(),
		`SELECT COUNT(*) FROM transactions WHERE deleted_at_utc IS NULL;`,
	).Scan(&count); err != nil {
		t.Fatalf("count active transactions: %v", err)
	}

	return count
}

func activeTransactionCountByNote(t *testing.T, db *sql.DB, note string) int64 {
	t.Helper()

	var count int64
	if err := db.QueryRowContext(
		context.Background(),
		`SELECT COUNT(*) FROM transactions WHERE deleted_at_utc IS NULL AND COALESCE(note, '') = ?;`,
		note,
	).Scan(&count); err != nil {
		t.Fatalf("count active transactions by note: %v", err)
	}

	return count
}
