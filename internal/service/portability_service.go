package service

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"budgetto/internal/domain"
)

const (
	PortabilityFormatJSON = "json"
	PortabilityFormatCSV  = "csv"
)

type PortabilityService struct {
	entryService  *EntryService
	reportService *ReportService
	db            *sql.DB
}

type PortabilityImportResult struct {
	Imported int64            `json:"imported"`
	Skipped  int64            `json:"skipped"`
	Warnings []domain.Warning `json:"warnings"`
}

type PortabilityReportExportResult struct {
	Warnings []domain.Warning `json:"warnings"`
}

type PortabilityServiceOption func(*PortabilityService)

type portabilityEntryRecord struct {
	Type               string  `json:"type"`
	AmountMinor        int64   `json:"amount_minor"`
	CurrencyCode       string  `json:"currency_code"`
	TransactionDateUTC string  `json:"transaction_date_utc"`
	CategoryID         *int64  `json:"category_id,omitempty"`
	LabelIDs           []int64 `json:"label_ids,omitempty"`
	Note               string  `json:"note,omitempty"`
}

type portabilityJSONEnvelope struct {
	Entries []portabilityEntryRecord `json:"entries"`
}

type portabilityReportJSONEnvelope struct {
	Report   domain.Report    `json:"report"`
	Warnings []domain.Warning `json:"warnings"`
}

func WithPortabilityReportService(reportService *ReportService) PortabilityServiceOption {
	return func(s *PortabilityService) {
		s.reportService = reportService
	}
}

func NewPortabilityService(entryService *EntryService, db *sql.DB, opts ...PortabilityServiceOption) (*PortabilityService, error) {
	if entryService == nil {
		return nil, fmt.Errorf("portability service: entry service is required")
	}
	if db == nil {
		return nil, fmt.Errorf("portability service: db is required")
	}

	service := &PortabilityService{
		entryService: entryService,
		db:           db,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(service)
		}
	}

	return service, nil
}

func (s *PortabilityService) Export(ctx context.Context, format, filePath string, filter domain.EntryListFilter) (int64, error) {
	normalizedFormat := normalizePortabilityFormat(format)
	if normalizedFormat == "" {
		return 0, fmt.Errorf("unsupported export format: %s", format)
	}

	entries, err := s.entryService.List(ctx, filter)
	if err != nil {
		return 0, err
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return 0, err
	}

	switch normalizedFormat {
	case PortabilityFormatJSON:
		if err := writeEntriesJSON(filePath, entries); err != nil {
			return 0, err
		}
	case PortabilityFormatCSV:
		if err := writeEntriesCSV(filePath, entries); err != nil {
			return 0, err
		}
	}

	return int64(len(entries)), nil
}

func (s *PortabilityService) Import(ctx context.Context, format, filePath string, idempotent bool) (PortabilityImportResult, error) {
	normalizedFormat := normalizePortabilityFormat(format)
	if normalizedFormat == "" {
		return PortabilityImportResult{}, fmt.Errorf("unsupported import format: %s", format)
	}

	existingSignatures := map[string]struct{}{}
	if idempotent {
		existing, err := s.entryService.List(ctx, domain.EntryListFilter{})
		if err != nil {
			return PortabilityImportResult{}, err
		}
		for _, entry := range existing {
			existingSignatures[entrySignature(entry)] = struct{}{}
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PortabilityImportResult{}, fmt.Errorf("portability import begin tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	txEntryRepo, ok := bindEntryRepositoryToTx(s.entryService.repo, tx)
	if !ok {
		return PortabilityImportResult{}, fmt.Errorf("portability import: entry repository does not support transactional import")
	}

	entryServiceOptions := []EntryServiceOption{}
	if s.entryService.capLookup != nil {
		txCapLookup, ok := bindEntryCapLookupToTx(s.entryService.capLookup, tx)
		if !ok {
			return PortabilityImportResult{}, fmt.Errorf("portability import: cap lookup does not support transactional import")
		}
		entryServiceOptions = append(entryServiceOptions, WithEntryCapLookup(txCapLookup))
	}

	txEntryService, err := NewEntryService(txEntryRepo, entryServiceOptions...)
	if err != nil {
		return PortabilityImportResult{}, err
	}

	result := PortabilityImportResult{Warnings: []domain.Warning{}}
	if err := streamImportRecords(normalizedFormat, filePath, func(record portabilityEntryRecord) error {
		candidate := domain.Entry{
			Type:               record.Type,
			AmountMinor:        record.AmountMinor,
			CurrencyCode:       record.CurrencyCode,
			TransactionDateUTC: record.TransactionDateUTC,
			CategoryID:         record.CategoryID,
			LabelIDs:           record.LabelIDs,
			Note:               record.Note,
		}

		signature := entrySignature(candidate)
		if idempotent {
			if _, exists := existingSignatures[signature]; exists {
				result.Skipped++
				return nil
			}
		}

		created, err := txEntryService.AddWithWarnings(ctx, domain.EntryAddInput{
			Type:               record.Type,
			AmountMinor:        record.AmountMinor,
			CurrencyCode:       record.CurrencyCode,
			TransactionDateUTC: record.TransactionDateUTC,
			CategoryID:         record.CategoryID,
			LabelIDs:           record.LabelIDs,
			Note:               record.Note,
		})
		if err != nil {
			return err
		}

		result.Imported++
		result.Warnings = append(result.Warnings, created.Warnings...)
		existingSignatures[entrySignature(created.Entry)] = struct{}{}
		return nil
	}); err != nil {
		return PortabilityImportResult{}, err
	}

	if err := tx.Commit(); err != nil {
		return PortabilityImportResult{}, fmt.Errorf("portability import commit: %w", err)
	}

	return result, nil
}

func (s *PortabilityService) ExportReport(ctx context.Context, format, filePath string, req ReportRequest) (PortabilityReportExportResult, error) {
	normalizedFormat := normalizePortabilityFormat(format)
	if normalizedFormat == "" {
		return PortabilityReportExportResult{}, fmt.Errorf("unsupported export format: %s", format)
	}

	if s.reportService == nil {
		return PortabilityReportExportResult{}, fmt.Errorf("report export unavailable: report service is not configured")
	}

	result, err := s.reportService.Generate(ctx, req)
	if err != nil {
		return PortabilityReportExportResult{}, err
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return PortabilityReportExportResult{}, err
	}

	switch normalizedFormat {
	case PortabilityFormatJSON:
		if err := writeReportJSON(filePath, result.Report, result.Warnings); err != nil {
			return PortabilityReportExportResult{}, err
		}
	case PortabilityFormatCSV:
		if err := writeReportCSV(filePath, result.Report, result.Warnings); err != nil {
			return PortabilityReportExportResult{}, err
		}
	}

	return PortabilityReportExportResult{Warnings: result.Warnings}, nil
}

func (s *PortabilityService) Backup(ctx context.Context, outputPath string) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}

	escapedPath := strings.ReplaceAll(outputPath, "'", "''")
	query := fmt.Sprintf("VACUUM INTO '%s';", escapedPath)
	_, err := s.db.ExecContext(ctx, query)
	return err
}

func normalizePortabilityFormat(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case PortabilityFormatJSON:
		return PortabilityFormatJSON
	case PortabilityFormatCSV:
		return PortabilityFormatCSV
	default:
		return ""
	}
}

func writeEntriesJSON(filePath string, entries []domain.Entry) error {
	records := make([]portabilityEntryRecord, 0, len(entries))
	for _, entry := range entries {
		records = append(records, portabilityEntryRecord{
			Type:               entry.Type,
			AmountMinor:        entry.AmountMinor,
			CurrencyCode:       entry.CurrencyCode,
			TransactionDateUTC: entry.TransactionDateUTC,
			CategoryID:         entry.CategoryID,
			LabelIDs:           entry.LabelIDs,
			Note:               entry.Note,
		})
	}

	payload := portabilityJSONEnvelope{Entries: records}
	content, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, content, 0o644)
}

func writeEntriesCSV(filePath string, entries []domain.Entry) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{"type", "amount_minor", "currency_code", "transaction_date_utc", "category_id", "label_ids", "note"}
	if err := writer.Write(header); err != nil {
		return err
	}

	for _, entry := range entries {
		categoryValue := ""
		if entry.CategoryID != nil {
			categoryValue = strconv.FormatInt(*entry.CategoryID, 10)
		}

		labelIDs := append([]int64(nil), entry.LabelIDs...)
		sort.Slice(labelIDs, func(i, j int) bool {
			return labelIDs[i] < labelIDs[j]
		})
		labelValues := make([]string, 0, len(labelIDs))
		for _, labelID := range labelIDs {
			labelValues = append(labelValues, strconv.FormatInt(labelID, 10))
		}

		row := []string{
			entry.Type,
			strconv.FormatInt(entry.AmountMinor, 10),
			entry.CurrencyCode,
			entry.TransactionDateUTC,
			categoryValue,
			strings.Join(labelValues, "|"),
			entry.Note,
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return writer.Error()
}

func writeReportJSON(filePath string, report domain.Report, warnings []domain.Warning) error {
	payload := portabilityReportJSONEnvelope{
		Report:   report,
		Warnings: warnings,
	}

	content, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, content, 0o644)
}

func writeReportCSV(filePath string, report domain.Report, warnings []domain.Warning) error {
	file, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	header := []string{
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
	if err := writer.Write(header); err != nil {
		return err
	}

	base := []string{
		report.Period.Scope,
		report.Grouping,
		report.Period.FromUTC,
		report.Period.ToUTC,
		report.Period.MonthKey,
	}
	writeRow := func(recordType, section, periodKey, categoryID, categoryKey, categoryLabel, currencyCode, totalMinor, monthKey, capAmountMinor, spendTotalMinor, overspendMinor, isExceeded, changeID, oldAmountMinor, newAmountMinor, changedAtUTC, targetCurrency, usedEstimateRate, warningCode, warningMessage, warningDetailsJSON string) error {
		row := []string{
			recordType,
			base[0],
			base[1],
			base[2],
			base[3],
			base[4],
			section,
			periodKey,
			categoryID,
			categoryKey,
			categoryLabel,
			currencyCode,
			totalMinor,
			monthKey,
			capAmountMinor,
			spendTotalMinor,
			overspendMinor,
			isExceeded,
			changeID,
			oldAmountMinor,
			newAmountMinor,
			changedAtUTC,
			targetCurrency,
			usedEstimateRate,
			warningCode,
			warningMessage,
			warningDetailsJSON,
		}
		return writer.Write(row)
	}

	if err := writeRow("report_meta", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", ""); err != nil {
		return err
	}

	for _, total := range report.Earnings.ByCurrency {
		if err := writeRow("currency_total", "earnings_by_currency", "", "", "", "", total.CurrencyCode, strconv.FormatInt(total.TotalMinor, 10), "", "", "", "", "", "", "", "", "", "", "", "", "", ""); err != nil {
			return err
		}
	}
	for _, total := range report.Spending.ByCurrency {
		if err := writeRow("currency_total", "spending_by_currency", "", "", "", "", total.CurrencyCode, strconv.FormatInt(total.TotalMinor, 10), "", "", "", "", "", "", "", "", "", "", "", "", "", ""); err != nil {
			return err
		}
	}
	for _, total := range report.Net.ByCurrency {
		if err := writeRow("currency_total", "net_by_currency", "", "", "", "", total.CurrencyCode, strconv.FormatInt(total.TotalMinor, 10), "", "", "", "", "", "", "", "", "", "", "", "", "", ""); err != nil {
			return err
		}
	}

	for _, group := range report.Earnings.Groups {
		if err := writeRow("group_total", "earnings_group", group.PeriodKey, "", "", "", group.CurrencyCode, strconv.FormatInt(group.TotalMinor, 10), "", "", "", "", "", "", "", "", "", "", "", "", "", ""); err != nil {
			return err
		}
	}
	for _, group := range report.Spending.Groups {
		if err := writeRow("group_total", "spending_group", group.PeriodKey, "", "", "", group.CurrencyCode, strconv.FormatInt(group.TotalMinor, 10), "", "", "", "", "", "", "", "", "", "", "", "", "", ""); err != nil {
			return err
		}
	}

	for _, category := range report.Earnings.Categories {
		categoryID := ""
		if category.CategoryID != nil {
			categoryID = strconv.FormatInt(*category.CategoryID, 10)
		}
		if err := writeRow("category_total", "earnings_category", "", categoryID, category.CategoryKey, category.CategoryLabel, category.CurrencyCode, strconv.FormatInt(category.TotalMinor, 10), "", "", "", "", "", "", "", "", "", "", "", "", "", ""); err != nil {
			return err
		}
	}
	for _, category := range report.Spending.Categories {
		categoryID := ""
		if category.CategoryID != nil {
			categoryID = strconv.FormatInt(*category.CategoryID, 10)
		}
		if err := writeRow("category_total", "spending_category", "", categoryID, category.CategoryKey, category.CategoryLabel, category.CurrencyCode, strconv.FormatInt(category.TotalMinor, 10), "", "", "", "", "", "", "", "", "", "", "", "", "", ""); err != nil {
			return err
		}
	}

	if report.Converted != nil {
		usedEstimate := strconv.FormatBool(report.Converted.UsedEstimateRate)
		if err := writeRow("converted_summary", "earnings", "", "", "", "", "", strconv.FormatInt(report.Converted.EarningsMinor, 10), "", "", "", "", "", "", "", "", "", report.Converted.TargetCurrency, usedEstimate, "", "", ""); err != nil {
			return err
		}
		if err := writeRow("converted_summary", "spending", "", "", "", "", "", strconv.FormatInt(report.Converted.SpendingMinor, 10), "", "", "", "", "", "", "", "", "", report.Converted.TargetCurrency, usedEstimate, "", "", ""); err != nil {
			return err
		}
		if err := writeRow("converted_summary", "net", "", "", "", "", "", strconv.FormatInt(report.Converted.NetMinor, 10), "", "", "", "", "", "", "", "", "", report.Converted.TargetCurrency, usedEstimate, "", "", ""); err != nil {
			return err
		}
	}

	for _, status := range report.CapStatus {
		if err := writeRow(
			"cap_status",
			"",
			"",
			"",
			"",
			"",
			status.CurrencyCode,
			"",
			status.MonthKey,
			strconv.FormatInt(status.CapAmountMinor, 10),
			strconv.FormatInt(status.SpendTotalMinor, 10),
			strconv.FormatInt(status.OverspendMinor, 10),
			strconv.FormatBool(status.IsExceeded),
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
		); err != nil {
			return err
		}
	}

	for _, change := range report.CapChanges {
		oldAmount := ""
		if change.OldAmountMinor != nil {
			oldAmount = strconv.FormatInt(*change.OldAmountMinor, 10)
		}
		if err := writeRow(
			"cap_change",
			"",
			"",
			"",
			"",
			"",
			change.CurrencyCode,
			"",
			change.MonthKey,
			"",
			"",
			"",
			"",
			strconv.FormatInt(change.ID, 10),
			oldAmount,
			strconv.FormatInt(change.NewAmountMinor, 10),
			change.ChangedAtUTC,
			"",
			"",
			"",
			"",
			"",
		); err != nil {
			return err
		}
	}

	for _, warning := range warnings {
		detailsJSON := ""
		if warning.Details != nil {
			detailsContent, err := json.Marshal(warning.Details)
			if err != nil {
				return err
			}
			detailsJSON = string(detailsContent)
		}
		if err := writeRow("warning", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", warning.Code, warning.Message, detailsJSON); err != nil {
			return err
		}
	}

	return writer.Error()
}

func streamImportRecords(format, filePath string, consume func(portabilityEntryRecord) error) error {
	switch format {
	case PortabilityFormatJSON:
		return streamImportRecordsJSON(filePath, consume)
	case PortabilityFormatCSV:
		return streamImportRecordsCSV(filePath, consume)
	default:
		return fmt.Errorf("unsupported format")
	}
}

func streamImportRecordsJSON(filePath string, consume func(portabilityEntryRecord) error) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)

	firstToken, err := decoder.Token()
	if err != nil {
		return err
	}

	root, ok := firstToken.(json.Delim)
	if !ok {
		return fmt.Errorf("invalid json import payload: expected top-level array or object with entries")
	}

	switch root {
	case '[':
		if err := decodeEntryArray(decoder, consume); err != nil {
			return err
		}
	case '{':
		foundEntries := false
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}

			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("invalid json import payload: expected object property name")
			}

			if key == "entries" {
				entriesToken, err := decoder.Token()
				if err != nil {
					return err
				}
				entriesDelim, ok := entriesToken.(json.Delim)
				if !ok || entriesDelim != '[' {
					return fmt.Errorf("invalid json import payload: entries must be an array")
				}
				if err := decodeEntryArray(decoder, consume); err != nil {
					return err
				}
				foundEntries = true
				continue
			}

			if err := discardJSONValue(decoder); err != nil {
				return err
			}
		}

		endToken, err := decoder.Token()
		if err != nil {
			return err
		}
		endDelim, ok := endToken.(json.Delim)
		if !ok || endDelim != '}' {
			return fmt.Errorf("invalid json import payload: malformed object")
		}
		if !foundEntries {
			return fmt.Errorf("invalid json import payload: expected top-level array or object with entries")
		}
	default:
		return fmt.Errorf("invalid json import payload: expected top-level array or object with entries")
	}

	if token, err := decoder.Token(); err != io.EOF {
		if err != nil {
			return err
		}
		return fmt.Errorf("invalid json import payload: trailing data found: %v", token)
	}

	return nil
}

func decodeEntryArray(decoder *json.Decoder, consume func(portabilityEntryRecord) error) error {
	for decoder.More() {
		record := portabilityEntryRecord{}
		if err := decoder.Decode(&record); err != nil {
			return err
		}
		if err := consume(record); err != nil {
			return err
		}
	}

	endToken, err := decoder.Token()
	if err != nil {
		return err
	}
	endDelim, ok := endToken.(json.Delim)
	if !ok || endDelim != ']' {
		return fmt.Errorf("invalid json import payload: malformed array")
	}

	return nil
}

func discardJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}

	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}

	switch delim {
	case '{':
		for decoder.More() {
			if _, err := decoder.Token(); err != nil {
				return err
			}
			if err := discardJSONValue(decoder); err != nil {
				return err
			}
		}
		endToken, err := decoder.Token()
		if err != nil {
			return err
		}
		endDelim, ok := endToken.(json.Delim)
		if !ok || endDelim != '}' {
			return fmt.Errorf("invalid json import payload: malformed object")
		}
		return nil
	case '[':
		for decoder.More() {
			if err := discardJSONValue(decoder); err != nil {
				return err
			}
		}
		endToken, err := decoder.Token()
		if err != nil {
			return err
		}
		endDelim, ok := endToken.(json.Delim)
		if !ok || endDelim != ']' {
			return fmt.Errorf("invalid json import payload: malformed array")
		}
		return nil
	default:
		return fmt.Errorf("invalid json import payload: malformed json value")
	}
}

func streamImportRecordsCSV(filePath string, consume func(portabilityEntryRecord) error) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := csv.NewReader(file)

	rowNumber := 0
	for {
		row, err := reader.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		rowNumber++
		if rowNumber == 1 && len(row) > 0 && strings.EqualFold(strings.TrimSpace(row[0]), "type") {
			continue
		}

		record, err := parseImportRecordCSVRow(row, rowNumber)
		if err != nil {
			return err
		}
		if err := consume(record); err != nil {
			return err
		}
	}
}

func parseImportRecordCSVRow(row []string, rowNumber int) (portabilityEntryRecord, error) {
	if len(row) < 7 {
		return portabilityEntryRecord{}, fmt.Errorf("invalid csv row %d: expected 7 columns", rowNumber)
	}

	amountMinor, err := strconv.ParseInt(strings.TrimSpace(row[1]), 10, 64)
	if err != nil {
		return portabilityEntryRecord{}, fmt.Errorf("invalid amount_minor at row %d: %w", rowNumber, err)
	}

	var categoryID *int64
	if strings.TrimSpace(row[4]) != "" {
		parsedCategoryID, err := strconv.ParseInt(strings.TrimSpace(row[4]), 10, 64)
		if err != nil {
			return portabilityEntryRecord{}, fmt.Errorf("invalid category_id at row %d: %w", rowNumber, err)
		}
		categoryID = &parsedCategoryID
	}

	labelIDs := []int64{}
	for _, part := range strings.Split(strings.TrimSpace(row[5]), "|") {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		parsedLabelID, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return portabilityEntryRecord{}, fmt.Errorf("invalid label_ids value at row %d: %w", rowNumber, err)
		}
		labelIDs = append(labelIDs, parsedLabelID)
	}

	return portabilityEntryRecord{
		Type:               strings.TrimSpace(row[0]),
		AmountMinor:        amountMinor,
		CurrencyCode:       strings.TrimSpace(row[2]),
		TransactionDateUTC: strings.TrimSpace(row[3]),
		CategoryID:         categoryID,
		LabelIDs:           labelIDs,
		Note:               strings.TrimSpace(row[6]),
	}, nil
}

func entrySignature(entry domain.Entry) string {
	labelIDs := append([]int64(nil), entry.LabelIDs...)
	sort.Slice(labelIDs, func(i, j int) bool {
		return labelIDs[i] < labelIDs[j]
	})

	categoryID := ""
	if entry.CategoryID != nil {
		categoryID = strconv.FormatInt(*entry.CategoryID, 10)
	}

	labelValues := make([]string, 0, len(labelIDs))
	for _, labelID := range labelIDs {
		labelValues = append(labelValues, strconv.FormatInt(labelID, 10))
	}

	return strings.Join([]string{
		entry.Type,
		strconv.FormatInt(entry.AmountMinor, 10),
		entry.CurrencyCode,
		entry.TransactionDateUTC,
		categoryID,
		strings.Join(labelValues, ","),
		entry.Note,
	}, "|")
}

func bindEntryRepositoryToTx(repo EntryRepository, tx *sql.Tx) (EntryRepository, bool) {
	binder, ok := repo.(EntryRepositoryTxBinder)
	if !ok {
		return nil, false
	}

	boundRepo := binder.BindTx(tx)
	if boundRepo == nil {
		return nil, false
	}

	return boundRepo, true
}

func bindEntryCapLookupToTx(capLookup EntryCapLookup, tx *sql.Tx) (EntryCapLookup, bool) {
	binder, ok := capLookup.(EntryCapLookupTxBinder)
	if !ok {
		return nil, false
	}

	boundCapLookup := binder.BindTx(tx)
	if boundCapLookup == nil {
		return nil, false
	}

	return boundCapLookup, true
}
