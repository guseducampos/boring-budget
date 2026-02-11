package service

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"budgetto/internal/domain"
	sqlitestore "budgetto/internal/store/sqlite"
)

func TestPortabilityServiceImportRollsBackBatchWhenOneRecordFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	portabilitySvc, _, db := newPortabilityServiceTestHarness(t)
	defer db.Close()

	importPath := writePortabilityImportJSON(t, []portabilityEntryRecord{
		{
			Type:               domain.EntryTypeIncome,
			AmountMinor:        10000,
			CurrencyCode:       "USD",
			TransactionDateUTC: "2026-02-01T00:00:00Z",
			Note:               "salary",
		},
		{
			Type:               domain.EntryTypeExpense,
			AmountMinor:        1200,
			CurrencyCode:       "USD",
			TransactionDateUTC: "2026-02-02T00:00:00Z",
			LabelIDs:           []int64{999999},
			Note:               "missing label",
		},
	})

	_, err := portabilitySvc.Import(ctx, PortabilityFormatJSON, importPath, false)
	if !errors.Is(err, domain.ErrLabelNotFound) {
		t.Fatalf("expected ErrLabelNotFound, got %v", err)
	}

	if count := activePortabilityTransactionCount(t, ctx, db); count != 0 {
		t.Fatalf("expected rollback to keep zero transactions, got %d", count)
	}
}

func TestPortabilityServiceImportIdempotentKeepsDuplicateHandling(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	portabilitySvc, entrySvc, db := newPortabilityServiceTestHarness(t)
	defer db.Close()

	if _, err := entrySvc.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeIncome,
		AmountMinor:        9000,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-03-01T10:00:00Z",
		Note:               "existing salary",
	}); err != nil {
		t.Fatalf("seed existing entry: %v", err)
	}

	importPath := writePortabilityImportJSON(t, []portabilityEntryRecord{
		{
			Type:               domain.EntryTypeIncome,
			AmountMinor:        9000,
			CurrencyCode:       "USD",
			TransactionDateUTC: "2026-03-01T10:00:00Z",
			Note:               "existing salary",
		},
		{
			Type:               domain.EntryTypeExpense,
			AmountMinor:        2500,
			CurrencyCode:       "USD",
			TransactionDateUTC: "2026-03-02T00:00:00Z",
			Note:               "new groceries",
		},
		{
			Type:               domain.EntryTypeExpense,
			AmountMinor:        2500,
			CurrencyCode:       "USD",
			TransactionDateUTC: "2026-03-02T00:00:00Z",
			Note:               "new groceries",
		},
	})

	firstImport, err := portabilitySvc.Import(ctx, PortabilityFormatJSON, importPath, true)
	if err != nil {
		t.Fatalf("first idempotent import: %v", err)
	}
	if firstImport.Imported != 1 {
		t.Fatalf("expected imported=1 on first run, got %d", firstImport.Imported)
	}
	if firstImport.Skipped != 2 {
		t.Fatalf("expected skipped=2 on first run, got %d", firstImport.Skipped)
	}
	if len(firstImport.Warnings) != 0 {
		t.Fatalf("expected no warnings on first run, got %+v", firstImport.Warnings)
	}

	secondImport, err := portabilitySvc.Import(ctx, PortabilityFormatJSON, importPath, true)
	if err != nil {
		t.Fatalf("second idempotent import: %v", err)
	}
	if secondImport.Imported != 0 {
		t.Fatalf("expected imported=0 on second run, got %d", secondImport.Imported)
	}
	if secondImport.Skipped != 3 {
		t.Fatalf("expected skipped=3 on second run, got %d", secondImport.Skipped)
	}
	if len(secondImport.Warnings) != 0 {
		t.Fatalf("expected no warnings on second run, got %+v", secondImport.Warnings)
	}

	if count := activePortabilityTransactionCount(t, ctx, db); count != 2 {
		t.Fatalf("expected exactly two active transactions after idempotent imports, got %d", count)
	}
}

func TestPortabilityServiceImportRequiresTransactionalEntryRepositoryBinding(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, _, db := newPortabilityServiceTestHarness(t)
	defer db.Close()

	entrySvc, err := NewEntryService(nonTransactionalEntryRepo{})
	if err != nil {
		t.Fatalf("new entry service: %v", err)
	}

	portabilitySvc, err := NewPortabilityService(entrySvc, db)
	if err != nil {
		t.Fatalf("new portability service: %v", err)
	}

	importPath := writePortabilityImportJSON(t, []portabilityEntryRecord{
		{
			Type:               domain.EntryTypeIncome,
			AmountMinor:        1000,
			CurrencyCode:       "USD",
			TransactionDateUTC: "2026-02-01T00:00:00Z",
			Note:               "salary",
		},
	})

	_, err = portabilitySvc.Import(ctx, PortabilityFormatJSON, importPath, false)
	if err == nil {
		t.Fatalf("expected transactional import binding error")
	}
	if !strings.Contains(err.Error(), "entry repository does not support transactional import") {
		t.Fatalf("expected transactional binding error, got %v", err)
	}

	if count := activePortabilityTransactionCount(t, ctx, db); count != 0 {
		t.Fatalf("expected rollback to keep zero transactions, got %d", count)
	}
}

func TestPortabilityServiceExportReportJSON(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	portabilitySvc, entrySvc, db := newPortabilityServiceWithReportTestHarness(t)
	defer db.Close()

	if _, err := entrySvc.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeIncome,
		AmountMinor:        10000,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-02-10T10:00:00Z",
		Note:               "salary",
	}); err != nil {
		t.Fatalf("add income entry: %v", err)
	}
	if _, err := entrySvc.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeExpense,
		AmountMinor:        2500,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-02-11T10:00:00Z",
		Note:               "groceries",
	}); err != nil {
		t.Fatalf("add expense entry: %v", err)
	}

	reportPath := filepath.Join(t.TempDir(), "exports", "report.json")
	result, err := portabilitySvc.ExportReport(ctx, PortabilityFormatJSON, reportPath, ReportRequest{
		Period: domain.ReportPeriodInput{
			Scope:    domain.ReportScopeMonthly,
			MonthKey: "2026-02",
		},
		Grouping: domain.ReportGroupingMonth,
	})
	if err != nil {
		t.Fatalf("export report json: %v", err)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Code != domain.WarningCodeOrphanSpendingExceeded {
		t.Fatalf("expected orphan spending warning, got %+v", result.Warnings)
	}

	content, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report export file: %v", err)
	}

	payload := portabilityReportJSONEnvelope{}
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatalf("unmarshal report export payload: %v", err)
	}

	if payload.Report.Period.Scope != domain.ReportScopeMonthly {
		t.Fatalf("expected monthly scope, got %q", payload.Report.Period.Scope)
	}
	if payload.Report.Period.MonthKey != "2026-02" {
		t.Fatalf("expected month_key=2026-02, got %q", payload.Report.Period.MonthKey)
	}
	if len(payload.Report.Earnings.ByCurrency) != 1 || payload.Report.Earnings.ByCurrency[0].TotalMinor != 10000 {
		t.Fatalf("unexpected earnings by currency: %+v", payload.Report.Earnings.ByCurrency)
	}
	if len(payload.Report.Spending.ByCurrency) != 1 || payload.Report.Spending.ByCurrency[0].TotalMinor != 2500 {
		t.Fatalf("unexpected spending by currency: %+v", payload.Report.Spending.ByCurrency)
	}
	if len(payload.Warnings) != 1 || payload.Warnings[0].Code != domain.WarningCodeOrphanSpendingExceeded {
		t.Fatalf("expected warning in exported report payload, got %+v", payload.Warnings)
	}
}

func TestPortabilityServiceExportReportCSV(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	portabilitySvc, entrySvc, db := newPortabilityServiceWithReportTestHarness(t)
	defer db.Close()

	if _, err := entrySvc.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeIncome,
		AmountMinor:        7000,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-02-10T10:00:00Z",
		Note:               "salary",
	}); err != nil {
		t.Fatalf("add income entry: %v", err)
	}
	if _, err := entrySvc.Add(ctx, domain.EntryAddInput{
		Type:               domain.EntryTypeExpense,
		AmountMinor:        1200,
		CurrencyCode:       "USD",
		TransactionDateUTC: "2026-02-11T10:00:00Z",
		Note:               "lunch",
	}); err != nil {
		t.Fatalf("add expense entry: %v", err)
	}

	reportPath := filepath.Join(t.TempDir(), "exports", "report.csv")
	if _, err := portabilitySvc.ExportReport(ctx, PortabilityFormatCSV, reportPath, ReportRequest{
		Period: domain.ReportPeriodInput{
			Scope:    domain.ReportScopeMonthly,
			MonthKey: "2026-02",
		},
		Grouping: domain.ReportGroupingMonth,
	}); err != nil {
		t.Fatalf("export report csv: %v", err)
	}

	file, err := os.Open(reportPath)
	if err != nil {
		t.Fatalf("open report csv: %v", err)
	}
	defer file.Close()

	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatalf("read csv rows: %v", err)
	}
	if len(rows) < 4 {
		t.Fatalf("expected multiple rows, got %d", len(rows))
	}
	if rows[0][0] != "record_type" || rows[0][1] != "scope" {
		t.Fatalf("unexpected csv header: %v", rows[0])
	}
	if rows[1][0] != "report_meta" {
		t.Fatalf("expected first data row to be report_meta, got %v", rows[1])
	}

	foundEarnings := false
	foundWarning := false
	for _, row := range rows[1:] {
		if len(row) != len(rows[0]) {
			t.Fatalf("expected stable csv column width, got row=%v", row)
		}
		if row[0] == "currency_total" && row[6] == "earnings_by_currency" && row[11] == "USD" && row[12] == "7000" {
			foundEarnings = true
		}
		if row[0] == "warning" && row[24] == domain.WarningCodeOrphanSpendingExceeded {
			foundWarning = true
		}
	}

	if !foundEarnings {
		t.Fatalf("expected earnings currency row in csv export: %v", rows)
	}
	if !foundWarning {
		t.Fatalf("expected warning row in csv export: %v", rows)
	}
}

func TestPortabilityServiceExportReportRequiresReportService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	portabilitySvc, _, db := newPortabilityServiceTestHarness(t)
	defer db.Close()

	_, err := portabilitySvc.ExportReport(ctx, PortabilityFormatJSON, filepath.Join(t.TempDir(), "report.json"), ReportRequest{
		Period: domain.ReportPeriodInput{
			Scope:    domain.ReportScopeMonthly,
			MonthKey: "2026-02",
		},
	})
	if err == nil {
		t.Fatalf("expected error when report service is not configured")
	}
	if !strings.Contains(err.Error(), "report export unavailable") {
		t.Fatalf("expected unavailable error, got %v", err)
	}
}

func newPortabilityServiceTestHarness(t *testing.T) (*PortabilityService, *EntryService, *sql.DB) {
	t.Helper()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "portability-service-test.db")
	db, err := sqlitestore.OpenAndMigrate(ctx, dbPath, portabilityMigrationsDirFromThisFile(t))
	if err != nil {
		t.Fatalf("open and migrate portability test db: %v", err)
	}

	entryRepo := sqlitestore.NewEntryRepo(db)
	capRepo := sqlitestore.NewCapRepo(db)
	entrySvc, err := NewEntryService(entryRepo, WithEntryCapLookup(capRepo))
	if err != nil {
		t.Fatalf("new entry service: %v", err)
	}

	portabilitySvc, err := NewPortabilityService(entrySvc, db)
	if err != nil {
		t.Fatalf("new portability service: %v", err)
	}

	return portabilitySvc, entrySvc, db
}

func newPortabilityServiceWithReportTestHarness(t *testing.T) (*PortabilityService, *EntryService, *sql.DB) {
	t.Helper()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "portability-service-report-test.db")
	db, err := sqlitestore.OpenAndMigrate(ctx, dbPath, portabilityMigrationsDirFromThisFile(t))
	if err != nil {
		t.Fatalf("open and migrate portability report test db: %v", err)
	}

	entryRepo := sqlitestore.NewEntryRepo(db)
	capRepo := sqlitestore.NewCapRepo(db)
	entrySvc, err := NewEntryService(entryRepo, WithEntryCapLookup(capRepo))
	if err != nil {
		t.Fatalf("new entry service: %v", err)
	}

	capSvc, err := NewCapService(capRepo)
	if err != nil {
		t.Fatalf("new cap service: %v", err)
	}

	reportSvc, err := NewReportService(entrySvc, capSvc)
	if err != nil {
		t.Fatalf("new report service: %v", err)
	}

	portabilitySvc, err := NewPortabilityService(entrySvc, db, WithPortabilityReportService(reportSvc))
	if err != nil {
		t.Fatalf("new portability service with report: %v", err)
	}

	return portabilitySvc, entrySvc, db
}

func portabilityMigrationsDirFromThisFile(t *testing.T) string {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve current file path")
	}

	projectRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))
	return filepath.Join(projectRoot, "migrations")
}

func writePortabilityImportJSON(t *testing.T, records []portabilityEntryRecord) string {
	t.Helper()

	filePath := filepath.Join(t.TempDir(), "import.json")
	payload := portabilityJSONEnvelope{Entries: records}
	content, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal import payload: %v", err)
	}

	if err := os.WriteFile(filePath, content, 0o644); err != nil {
		t.Fatalf("write import payload: %v", err)
	}

	return filePath
}

func activePortabilityTransactionCount(t *testing.T, ctx context.Context, db *sql.DB) int64 {
	t.Helper()

	var count int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM transactions WHERE deleted_at_utc IS NULL;`).Scan(&count); err != nil {
		t.Fatalf("count active transactions: %v", err)
	}

	return count
}

type nonTransactionalEntryRepo struct{}

func (nonTransactionalEntryRepo) Add(context.Context, domain.EntryAddInput) (domain.Entry, error) {
	return domain.Entry{}, errors.New("not implemented")
}

func (nonTransactionalEntryRepo) Update(context.Context, domain.EntryUpdateInput) (domain.Entry, error) {
	return domain.Entry{}, errors.New("not implemented")
}

func (nonTransactionalEntryRepo) List(context.Context, domain.EntryListFilter) ([]domain.Entry, error) {
	return nil, errors.New("not implemented")
}

func (nonTransactionalEntryRepo) Delete(context.Context, int64) (domain.EntryDeleteResult, error) {
	return domain.EntryDeleteResult{}, errors.New("not implemented")
}
