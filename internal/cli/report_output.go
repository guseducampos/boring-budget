package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"boring-budget/internal/domain"
)

func toReportOutputData(report domain.Report) (map[string]any, error) {
	raw, err := json.Marshal(report)
	if err != nil {
		return nil, fmt.Errorf("marshal report: %w", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal report: %w", err)
	}

	convertMinorFields(payload)
	return payload, nil
}

func toReportOutputWarnings(warnings []domain.Warning) ([]map[string]any, error) {
	if len(warnings) == 0 {
		return []map[string]any{}, nil
	}

	out := make([]map[string]any, 0, len(warnings))
	for _, warning := range warnings {
		raw, err := json.Marshal(map[string]any{
			"code":    warning.Code,
			"message": warning.Message,
			"details": warning.Details,
		})
		if err != nil {
			return nil, fmt.Errorf("marshal report warning: %w", err)
		}

		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			return nil, fmt.Errorf("unmarshal report warning: %w", err)
		}
		convertMinorFields(payload)
		out = append(out, payload)
	}

	return out, nil
}

func convertMinorFields(node any) {
	switch typed := node.(type) {
	case map[string]any:
		for _, value := range typed {
			convertMinorFields(value)
		}
		convertMinorFieldsInMap(typed)
	case []any:
		for _, value := range typed {
			convertMinorFields(value)
		}
	}
}

func convertMinorFieldsInMap(values map[string]any) {
	currencyCode := currencyForMap(values)
	if currencyCode == "" {
		return
	}

	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	for _, key := range keys {
		renameTo := ""
		switch {
		case strings.HasSuffix(key, "_minor_signed"):
			renameTo = strings.TrimSuffix(key, "_minor_signed") + "_major_signed"
		case strings.HasSuffix(key, "_minor"):
			renameTo = strings.TrimSuffix(key, "_minor") + "_major"
		default:
			continue
		}

		minorValue, ok := int64FromJSONNumber(values[key])
		if !ok {
			continue
		}
		majorValue, err := domain.FormatMinorToMajorString(minorValue, currencyCode)
		if err != nil {
			continue
		}
		values[renameTo] = majorValue
		delete(values, key)
	}
}

func currencyForMap(values map[string]any) string {
	if code, ok := values["currency_code"].(string); ok {
		return strings.TrimSpace(code)
	}
	if code, ok := values["target_currency"].(string); ok {
		return strings.TrimSpace(code)
	}
	return ""
}

func int64FromJSONNumber(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		return int64(typed), true
	case int64:
		return typed, true
	case int:
		return int64(typed), true
	default:
		return 0, false
	}
}
