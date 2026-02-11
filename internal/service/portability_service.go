package service

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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
	entryService *EntryService
	db           *sql.DB
}

type PortabilityImportResult struct {
	Imported int64            `json:"imported"`
	Skipped  int64            `json:"skipped"`
	Warnings []domain.Warning `json:"warnings"`
}

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

func NewPortabilityService(entryService *EntryService, db *sql.DB) (*PortabilityService, error) {
	if entryService == nil {
		return nil, fmt.Errorf("portability service: entry service is required")
	}
	if db == nil {
		return nil, fmt.Errorf("portability service: db is required")
	}

	return &PortabilityService{
		entryService: entryService,
		db:           db,
	}, nil
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

	records, err := readImportRecords(normalizedFormat, filePath)
	if err != nil {
		return PortabilityImportResult{}, err
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
	for _, record := range records {
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
				continue
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
			return PortabilityImportResult{}, err
		}

		result.Imported++
		result.Warnings = append(result.Warnings, created.Warnings...)
		existingSignatures[entrySignature(created.Entry)] = struct{}{}
	}

	if err := tx.Commit(); err != nil {
		return PortabilityImportResult{}, fmt.Errorf("portability import commit: %w", err)
	}

	return result, nil
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

func readImportRecords(format, filePath string) ([]portabilityEntryRecord, error) {
	switch format {
	case PortabilityFormatJSON:
		return readImportRecordsJSON(filePath)
	case PortabilityFormatCSV:
		return readImportRecordsCSV(filePath)
	default:
		return nil, fmt.Errorf("unsupported format")
	}
}

func readImportRecordsJSON(filePath string) ([]portabilityEntryRecord, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	payload := portabilityJSONEnvelope{}
	if err := json.Unmarshal(content, &payload); err == nil && payload.Entries != nil {
		return payload.Entries, nil
	}

	records := []portabilityEntryRecord{}
	if err := json.Unmarshal(content, &records); err != nil {
		return nil, err
	}
	return records, nil
}

func readImportRecordsCSV(filePath string) ([]portabilityEntryRecord, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []portabilityEntryRecord{}, nil
	}

	start := 0
	if len(rows[0]) > 0 && strings.EqualFold(strings.TrimSpace(rows[0][0]), "type") {
		start = 1
	}

	records := make([]portabilityEntryRecord, 0, len(rows)-start)
	for i := start; i < len(rows); i++ {
		row := rows[i]
		if len(row) < 7 {
			return nil, fmt.Errorf("invalid csv row %d: expected 7 columns", i+1)
		}

		amountMinor, err := strconv.ParseInt(strings.TrimSpace(row[1]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid amount_minor at row %d: %w", i+1, err)
		}

		var categoryID *int64
		if strings.TrimSpace(row[4]) != "" {
			parsedCategoryID, err := strconv.ParseInt(strings.TrimSpace(row[4]), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid category_id at row %d: %w", i+1, err)
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
				return nil, fmt.Errorf("invalid label_ids value at row %d: %w", i+1, err)
			}
			labelIDs = append(labelIDs, parsedLabelID)
		}

		records = append(records, portabilityEntryRecord{
			Type:               strings.TrimSpace(row[0]),
			AmountMinor:        amountMinor,
			CurrencyCode:       strings.TrimSpace(row[2]),
			TransactionDateUTC: strings.TrimSpace(row[3]),
			CategoryID:         categoryID,
			LabelIDs:           labelIDs,
			Note:               strings.TrimSpace(row[6]),
		})
	}

	return records, nil
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
	method := reflect.ValueOf(repo).MethodByName("BindTx")
	if !method.IsValid() {
		return nil, false
	}

	txArgType := reflect.TypeOf((*sql.Tx)(nil))
	if method.Type().NumIn() != 1 || method.Type().In(0) != txArgType || method.Type().NumOut() != 1 {
		return nil, false
	}

	results := method.Call([]reflect.Value{reflect.ValueOf(tx)})
	boundRepo, ok := results[0].Interface().(EntryRepository)
	if !ok || boundRepo == nil {
		return nil, false
	}

	return boundRepo, true
}

func bindEntryCapLookupToTx(capLookup EntryCapLookup, tx *sql.Tx) (EntryCapLookup, bool) {
	method := reflect.ValueOf(capLookup).MethodByName("BindTx")
	if !method.IsValid() {
		return nil, false
	}

	txArgType := reflect.TypeOf((*sql.Tx)(nil))
	if method.Type().NumIn() != 1 || method.Type().In(0) != txArgType || method.Type().NumOut() != 1 {
		return nil, false
	}

	results := method.Call([]reflect.Value{reflect.ValueOf(tx)})
	boundCapLookup, ok := results[0].Interface().(EntryCapLookup)
	if !ok || boundCapLookup == nil {
		return nil, false
	}

	return boundCapLookup, true
}
