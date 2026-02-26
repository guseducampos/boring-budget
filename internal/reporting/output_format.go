package reporting

import (
	"encoding/json"
	"fmt"
	"strings"

	"boring-budget/internal/domain"
)

// ToMajorUnitMap converts a report-like payload into a JSON object map where
// money fields are expressed as major-unit strings.
func ToMajorUnitMap(value any) (map[string]any, error) {
	normalized, err := toMajorUnitJSONValue(value)
	if err != nil {
		return nil, err
	}

	payload, ok := normalized.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected object payload")
	}
	return payload, nil
}

// ToMajorUnitMapSlice converts a report-like payload into a JSON array of maps
// where money fields are expressed as major-unit strings.
func ToMajorUnitMapSlice(value any) ([]map[string]any, error) {
	normalized, err := toMajorUnitJSONValue(value)
	if err != nil {
		return nil, err
	}

	array, ok := normalized.([]any)
	if !ok {
		return nil, fmt.Errorf("expected array payload")
	}

	out := make([]map[string]any, 0, len(array))
	for _, item := range array {
		typed, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected array object item")
		}
		out = append(out, typed)
	}
	return out, nil
}

func toMajorUnitJSONValue(value any) (any, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}

	convertMinorFields(payload)
	return payload, nil
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

		if values[key] == nil {
			values[renameTo] = nil
			delete(values, key)
			continue
		}
		if currencyCode == "" {
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
	case int32:
		return int64(typed), true
	case int:
		return int64(typed), true
	default:
		return 0, false
	}
}
