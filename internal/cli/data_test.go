package cli

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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

func TestDataCommandCSVExportImportIdempotent(t *testing.T) {
	t.Parallel()

	sourceDB := newCLITestDB(t)
	t.Cleanup(func() { _ = sourceDB.Close() })

	sourceWorkLabelID := insertTestLabel(t, sourceDB, "work")
	sourceTripLabelID := insertTestLabel(t, sourceDB, "trip")

	mustEntrySuccess(t, executeEntryCmdJSON(t, sourceDB, []string{
		"add",
		"--type", "income",
		"--amount-minor", "9000",
		"--currency", "USD",
		"--date", "2026-03-01",
		"--note", "salary",
	}))
	mustEntrySuccess(t, executeEntryCmdJSON(t, sourceDB, []string{
		"add",
		"--type", "expense",
		"--amount-minor", "2700",
		"--currency", "USD",
		"--date", "2026-03-02",
		"--label-id", strconv.FormatInt(sourceTripLabelID, 10),
		"--label-id", strconv.FormatInt(sourceWorkLabelID, 10),
		"--note", "subway pass",
	}))

	exportPath := filepath.Join(t.TempDir(), "exports", "entries.csv")
	sourceOpts := &RootOptions{Output: output.FormatJSON, db: sourceDB}
	exportPayload := executeDataCmdJSONWithOptions(t, sourceOpts, []string{
		"export",
		"--format", "csv",
		"--file", exportPath,
	})

	assertSuccessJSONEnvelope(t, exportPayload)
	exportData := mustMap(t, exportPayload["data"])
	if int64(exportData["exported"].(float64)) != 2 {
		t.Fatalf("expected exported=2, got %v", exportData["exported"])
	}
	if exportData["format"].(string) != "csv" {
		t.Fatalf("expected format=csv, got %v", exportData["format"])
	}
	if exportData["file"].(string) != exportPath {
		t.Fatalf("expected file %q, got %v", exportPath, exportData["file"])
	}

	rows := readCSVFile(t, exportPath)
	if len(rows) != 3 {
		t.Fatalf("expected header + 2 rows in csv export, got %d rows", len(rows))
	}

	expectedHeader := []string{"type", "amount_minor", "currency_code", "transaction_date_utc", "category_id", "label_ids", "note"}
	assertCSVRowEqual(t, rows[0], expectedHeader)
	for _, row := range rows[1:] {
		if len(row) != len(expectedHeader) {
			t.Fatalf("expected stable csv column width=%d, got row=%v", len(expectedHeader), row)
		}
	}

	salaryRow, found := findEntryCSVRowByNote(rows, "salary")
	if !found {
		t.Fatalf("expected salary row in csv export: %v", rows)
	}
	if salaryRow[5] != "" {
		t.Fatalf("expected empty label_ids for salary row, got %q", salaryRow[5])
	}

	expenseRow, found := findEntryCSVRowByNote(rows, "subway pass")
	if !found {
		t.Fatalf("expected subway pass row in csv export: %v", rows)
	}
	expectedLabelCSV := pipeSeparatedSortedIDs(sourceWorkLabelID, sourceTripLabelID)
	if expenseRow[5] != expectedLabelCSV {
		t.Fatalf("expected sorted label_ids %q in csv export, got %q", expectedLabelCSV, expenseRow[5])
	}

	targetDB := newCLITestDB(t)
	t.Cleanup(func() { _ = targetDB.Close() })
	targetWorkLabelID := insertTestLabel(t, targetDB, "work")
	targetTripLabelID := insertTestLabel(t, targetDB, "trip")
	if targetWorkLabelID != sourceWorkLabelID || targetTripLabelID != sourceTripLabelID {
		t.Fatalf("expected target label IDs to match source for portability import, source=(%d,%d) target=(%d,%d)", sourceWorkLabelID, sourceTripLabelID, targetWorkLabelID, targetTripLabelID)
	}

	targetOpts := &RootOptions{Output: output.FormatJSON, db: targetDB}
	firstImport := executeDataCmdJSONWithOptions(t, targetOpts, []string{
		"import",
		"--format", "csv",
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

	secondImport := executeDataCmdJSONWithOptions(t, targetOpts, []string{
		"import",
		"--format", "csv",
		"--file", exportPath,
		"--idempotent",
	})
	assertSuccessJSONEnvelope(t, secondImport)
	secondImportData := mustMap(t, secondImport["data"])
	if int64(secondImportData["imported"].(float64)) != 0 {
		t.Fatalf("expected imported=0 on idempotent re-import, got %v", secondImportData["imported"])
	}
	if int64(secondImportData["skipped"].(float64)) != 2 {
		t.Fatalf("expected skipped=2 on idempotent re-import, got %v", secondImportData["skipped"])
	}
	if idempotent, _ := secondImportData["idempotent"].(bool); !idempotent {
		t.Fatalf("expected idempotent=true on second import, got false")
	}

	if count := activeTransactionCount(t, targetDB); count != 2 {
		t.Fatalf("expected imported transaction count 2, got %d", count)
	}

	importedEntries := executeEntryCmdJSON(t, targetDB, []string{"list"})
	importedData := mustMap(t, importedEntries["data"])
	importedRows := mustAnySlice(t, importedData["entries"])
	if len(importedRows) != 2 {
		t.Fatalf("expected 2 imported entries, got %d", len(importedRows))
	}

	importedExpense, found := findJSONEntryByNote(t, importedRows, "subway pass")
	if !found {
		t.Fatalf("expected imported expense with note subway pass, got %v", importedRows)
	}
	labels := mustAnySlice(t, importedExpense["label_ids"])
	assertJSONInt64SliceEqual(t, labels, []int64{targetWorkLabelID, targetTripLabelID})
}

func TestDataCommandCSVImportParsesLabelDelimiter(t *testing.T) {
	t.Parallel()

	db := newCLITestDB(t)
	t.Cleanup(func() { _ = db.Close() })

	labelA := insertTestLabel(t, db, "A")
	labelB := insertTestLabel(t, db, "B")

	importPath := filepath.Join(t.TempDir(), "imports", "entries.csv")
	writeCSVFile(t, importPath, [][]string{
		{"type", "amount_minor", "currency_code", "transaction_date_utc", "category_id", "label_ids", "note"},
		{
			"expense",
			"1750",
			"USD",
			"2026-03-03T00:00:00Z",
			"",
			"  " + strconv.FormatInt(labelB, 10) + " | " + strconv.FormatInt(labelA, 10) + " || ",
			"delimiter parse",
		},
	})

	opts := &RootOptions{Output: output.FormatJSON, db: db}
	firstImport := executeDataCmdJSONWithOptions(t, opts, []string{
		"import",
		"--format", "csv",
		"--file", importPath,
		"--idempotent",
	})
	assertSuccessJSONEnvelope(t, firstImport)
	firstImportData := mustMap(t, firstImport["data"])
	if int64(firstImportData["imported"].(float64)) != 1 {
		t.Fatalf("expected imported=1, got %v", firstImportData["imported"])
	}
	if int64(firstImportData["skipped"].(float64)) != 0 {
		t.Fatalf("expected skipped=0, got %v", firstImportData["skipped"])
	}

	secondImport := executeDataCmdJSONWithOptions(t, opts, []string{
		"import",
		"--format", "csv",
		"--file", importPath,
		"--idempotent",
	})
	assertSuccessJSONEnvelope(t, secondImport)
	secondImportData := mustMap(t, secondImport["data"])
	if int64(secondImportData["imported"].(float64)) != 0 {
		t.Fatalf("expected imported=0 on idempotent re-import, got %v", secondImportData["imported"])
	}
	if int64(secondImportData["skipped"].(float64)) != 1 {
		t.Fatalf("expected skipped=1 on idempotent re-import, got %v", secondImportData["skipped"])
	}

	listPayload := executeEntryCmdJSON(t, db, []string{"list"})
	listData := mustMap(t, listPayload["data"])
	entries := mustAnySlice(t, listData["entries"])
	if len(entries) != 1 {
		t.Fatalf("expected one imported entry, got %d", len(entries))
	}

	entry, found := findJSONEntryByNote(t, entries, "delimiter parse")
	if !found {
		t.Fatalf("expected imported entry note delimiter parse, got %v", entries)
	}
	labels := mustAnySlice(t, entry["label_ids"])
	assertJSONInt64SliceEqual(t, labels, []int64{labelA, labelB})
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

	for _, path := range []string{
		dbPath + ".restore.tmp",
		dbPath + ".restore.rollback",
		dbPath + ".restore.rollback-wal",
		dbPath + ".restore.rollback-shm",
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected restore artifact %q to be cleaned up; stat err=%v", path, err)
		}
	}
}

func TestDataCommandJSONRestoreFailureRollsBackDatabase(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "budgetto.db")
	migrationsDir := cliMigrationsPath(t)

	db, err := sqlitestore.OpenAndMigrate(context.Background(), dbPath, migrationsDir)
	if err != nil {
		t.Fatalf("open and migrate db for restore rollback: %v", err)
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
		"--note", "pre-rollback-check",
	}))
	mustEntrySuccess(t, executeEntryCmdJSON(t, opts.db, []string{
		"add",
		"--type", "expense",
		"--amount-minor", "250",
		"--currency", "USD",
		"--date", "2026-02-02",
		"--note", "still-present-after-failure",
	}))
	if count := activeTransactionCount(t, opts.db); count != 2 {
		t.Fatalf("expected two transactions before failed restore, got %d", count)
	}

	invalidBackupPath := filepath.Join(tempDir, "snapshots", "invalid.sqlite")
	if err := os.MkdirAll(filepath.Dir(invalidBackupPath), 0o755); err != nil {
		t.Fatalf("mkdir invalid backup dir: %v", err)
	}
	if err := os.WriteFile(invalidBackupPath, []byte("this-is-not-a-valid-sqlite-backup"), 0o644); err != nil {
		t.Fatalf("write invalid backup file: %v", err)
	}

	restorePayload := executeDataCmdJSONWithOptions(t, opts, []string{
		"restore",
		"--file", invalidBackupPath,
	})

	if ok, _ := restorePayload["ok"].(bool); ok {
		t.Fatalf("expected ok=false for invalid restore payload=%v", restorePayload)
	}

	errorMap := mustMap(t, restorePayload["error"])
	if code, _ := errorMap["code"].(string); strings.TrimSpace(code) == "" {
		t.Fatalf("expected non-empty error code in restore failure payload=%v", restorePayload)
	}

	if opts.db == nil {
		t.Fatalf("expected rollback to reopen original database handle")
	}
	if count := activeTransactionCount(t, opts.db); count != 2 {
		t.Fatalf("expected rollback to preserve two transactions, got %d", count)
	}
	if count := activeTransactionCountByNote(t, opts.db, "pre-rollback-check"); count != 1 {
		t.Fatalf("expected pre-rollback-check entry after failed restore, got %d", count)
	}
	if count := activeTransactionCountByNote(t, opts.db, "still-present-after-failure"); count != 1 {
		t.Fatalf("expected still-present-after-failure entry after failed restore, got %d", count)
	}

	for _, path := range []string{
		dbPath + ".restore.tmp",
		dbPath + ".restore.rollback",
		dbPath + ".restore.rollback-wal",
		dbPath + ".restore.rollback-shm",
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected restore artifact %q to be cleaned up after rollback; stat err=%v", path, err)
		}
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

func TestDataCommandCSVExportReportShape(t *testing.T) {
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

	exportPath := filepath.Join(t.TempDir(), "exports", "report.csv")
	opts := &RootOptions{Output: output.FormatJSON, db: db}
	payload := executeDataCmdJSONWithOptions(t, opts, []string{
		"export",
		"--resource", "report",
		"--format", "csv",
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
		t.Fatalf("expected one warning in csv report export payload, got %v", warnings)
	}
	firstWarning := mustMap(t, warnings[0])
	if firstWarning["code"].(string) != domain.WarningCodeOrphanSpendingExceeded {
		t.Fatalf("expected orphan spending warning, got %v", firstWarning)
	}

	data := mustMap(t, payload["data"])
	if data["resource"].(string) != "report" {
		t.Fatalf("expected resource=report, got %v", data["resource"])
	}
	if data["format"].(string) != "csv" {
		t.Fatalf("expected format=csv, got %v", data["format"])
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

	rows := readCSVFile(t, exportPath)
	if len(rows) < 4 {
		t.Fatalf("expected csv report rows, got %d", len(rows))
	}

	expectedHeader := []string{
		"record_type",
		"scope",
		"grouping",
		"period_from_utc",
		"period_to_utc",
		"period_month_key",
		"section",
		"period_key",
		"category_id",
		"category_key",
		"category_label",
		"currency_code",
		"total_minor",
		"month_key",
		"cap_amount_minor",
		"spend_total_minor",
		"overspend_minor",
		"is_exceeded",
		"change_id",
		"old_amount_minor",
		"new_amount_minor",
		"changed_at_utc",
		"target_currency",
		"used_estimate_rate",
		"warning_code",
		"warning_message",
		"warning_details_json",
	}
	assertCSVRowEqual(t, rows[0], expectedHeader)

	for _, row := range rows[1:] {
		if len(row) != len(expectedHeader) {
			t.Fatalf("expected stable csv report column width=%d, got row=%v", len(expectedHeader), row)
		}
	}

	metaRow := rows[1]
	if metaRow[0] != "report_meta" {
		t.Fatalf("expected first report row type report_meta, got %v", metaRow)
	}
	if metaRow[1] != "monthly" || metaRow[2] != "month" || metaRow[5] != "2026-02" {
		t.Fatalf("unexpected report_meta shape: %v", metaRow)
	}

	foundEarningsTotal := false
	foundWarningRow := false
	for _, row := range rows[1:] {
		if row[0] == "currency_total" && row[6] == "earnings_by_currency" && row[11] == "USD" && row[12] == "12000" {
			foundEarningsTotal = true
		}
		if row[0] == "warning" && row[24] == domain.WarningCodeOrphanSpendingExceeded && strings.TrimSpace(row[25]) != "" {
			foundWarningRow = true
		}
	}

	if !foundEarningsTotal {
		t.Fatalf("expected earnings currency_total row in csv report export: %v", rows)
	}
	if !foundWarningRow {
		t.Fatalf("expected warning row in csv report export: %v", rows)
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

func readCSVFile(t *testing.T, filePath string) [][]string {
	t.Helper()

	file, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("open csv file: %v", err)
	}
	defer file.Close()

	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatalf("read csv rows: %v", err)
	}

	return rows
}

func writeCSVFile(t *testing.T, filePath string, rows [][]string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir for csv file: %v", err)
	}

	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("create csv file: %v", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			t.Fatalf("write csv row: %v", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		t.Fatalf("flush csv writer: %v", err)
	}
}

func assertCSVRowEqual(t *testing.T, actual, expected []string) {
	t.Helper()

	if len(actual) != len(expected) {
		t.Fatalf("expected csv row width=%d, got %d row=%v", len(expected), len(actual), actual)
	}
	for i := range expected {
		if actual[i] != expected[i] {
			t.Fatalf("expected csv[%d]=%q, got %q row=%v", i, expected[i], actual[i], actual)
		}
	}
}

func findEntryCSVRowByNote(rows [][]string, note string) ([]string, bool) {
	for _, row := range rows[1:] {
		if len(row) >= 7 && row[6] == note {
			return row, true
		}
	}
	return nil, false
}

func pipeSeparatedSortedIDs(ids ...int64) string {
	sorted := append([]int64(nil), ids...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	out := make([]string, 0, len(sorted))
	for _, id := range sorted {
		out = append(out, strconv.FormatInt(id, 10))
	}
	return strings.Join(out, "|")
}

func findJSONEntryByNote(t *testing.T, entries []any, note string) (map[string]any, bool) {
	t.Helper()

	for _, value := range entries {
		entry := mustMap(t, value)
		currentNote, _ := entry["note"].(string)
		if currentNote == note {
			return entry, true
		}
	}
	return nil, false
}

func assertJSONInt64SliceEqual(t *testing.T, raw []any, expected []int64) {
	t.Helper()

	if len(raw) != len(expected) {
		t.Fatalf("expected slice length %d, got %d", len(expected), len(raw))
	}

	parsed := make([]int64, 0, len(raw))
	for _, value := range raw {
		parsed = append(parsed, int64(value.(float64)))
	}
	sort.Slice(parsed, func(i, j int) bool {
		return parsed[i] < parsed[j]
	})
	sort.Slice(expected, func(i, j int) bool {
		return expected[i] < expected[j]
	})
	for i := range expected {
		if parsed[i] != expected[i] {
			t.Fatalf("expected slice %v, got %v", expected, parsed)
		}
	}
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
