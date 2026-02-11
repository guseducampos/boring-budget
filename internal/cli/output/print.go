package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

func IsValidFormat(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case FormatHuman, FormatJSON:
		return true
	default:
		return false
	}
}

func Print(w io.Writer, format string, envelope Envelope) error {
	SetProcessExitCodeFromEnvelope(envelope)

	switch strings.ToLower(strings.TrimSpace(format)) {
	case FormatJSON:
		payload, err := json.MarshalIndent(envelope, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal json envelope: %w", err)
		}
		if _, err := fmt.Fprintln(w, string(payload)); err != nil {
			return fmt.Errorf("write json output: %w", err)
		}
		return nil
	case FormatHuman:
		return printHuman(w, envelope)
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

func printHuman(w io.Writer, envelope Envelope) error {
	status := "OK"
	if !envelope.Ok {
		status = "ERROR"
	}

	if _, err := fmt.Fprintf(w, "[%s] boring-budget\n", status); err != nil {
		return err
	}

	if !envelope.Ok && envelope.Error != nil {
		if _, err := fmt.Fprintf(w, "%s: %s\n", envelope.Error.Code, envelope.Error.Message); err != nil {
			return err
		}
	}

	for _, warning := range envelope.Warnings {
		if _, err := fmt.Fprintf(w, "warning[%s]: %s\n", warning.Code, warning.Message); err != nil {
			return err
		}
	}

	if envelope.Data != nil {
		humanData := localizeDataForHuman(envelope.Data)
		payload, err := json.MarshalIndent(humanData, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal human data: %w", err)
		}
		if _, err := fmt.Fprintf(w, "%s\n", payload); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(w, "api=%s timestamp_utc=%s\n", envelope.Meta.APIVersion, envelope.Meta.TimestampUTC); err != nil {
		return err
	}

	return nil
}

func localizeDataForHuman(data any) any {
	displayTZ := CurrentDisplayTimezone()
	if strings.EqualFold(displayTZ, "UTC") {
		return data
	}

	location, err := time.LoadLocation(displayTZ)
	if err != nil {
		return data
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return data
	}

	var node any
	if err := json.Unmarshal(raw, &node); err != nil {
		return data
	}

	return localizeNode(node, "", location)
}

func localizeNode(node any, key string, location *time.Location) any {
	switch value := node.(type) {
	case map[string]any:
		updated := make(map[string]any, len(value))
		for childKey, childValue := range value {
			updated[childKey] = localizeNode(childValue, childKey, location)
		}
		return updated
	case []any:
		updated := make([]any, 0, len(value))
		for _, item := range value {
			updated = append(updated, localizeNode(item, key, location))
		}
		return updated
	case string:
		if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(key)), "_utc") {
			return value
		}
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
			parsed, err := time.Parse(layout, value)
			if err == nil {
				return parsed.In(location).Format(time.RFC3339Nano)
			}
		}
		return value
	default:
		return node
	}
}
