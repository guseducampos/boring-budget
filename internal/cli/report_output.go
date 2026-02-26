package cli

import (
	"fmt"

	"boring-budget/internal/cli/output"
	"boring-budget/internal/domain"
	"boring-budget/internal/reporting"
)

func toReportOutputData(report domain.Report) (map[string]any, error) {
	payload, err := reporting.ToMajorUnitMap(report)
	if err != nil {
		return nil, fmt.Errorf("format report payload: %w", err)
	}

	return payload, nil
}

func toReportOutputWarnings(warnings []domain.Warning) ([]map[string]any, error) {
	if len(warnings) == 0 {
		return []map[string]any{}, nil
	}

	payload, err := reporting.ToMajorUnitMapSlice(warnings)
	if err != nil {
		return nil, fmt.Errorf("format report warning payload: %w", err)
	}
	return payload, nil
}

func toReportWarningPayloads(warnings []domain.Warning) ([]output.WarningPayload, error) {
	reportWarningsRaw, err := toReportOutputWarnings(warnings)
	if err != nil {
		return nil, err
	}

	reportWarnings := make([]output.WarningPayload, 0, len(reportWarningsRaw))
	for _, warning := range reportWarningsRaw {
		code, _ := warning["code"].(string)
		message, _ := warning["message"].(string)
		reportWarnings = append(reportWarnings, output.WarningPayload{
			Code:    code,
			Message: message,
			Details: warning["details"],
		})
	}
	return reportWarnings, nil
}
