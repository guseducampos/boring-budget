package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
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

	if _, err := fmt.Fprintf(w, "[%s] budgetto\n", status); err != nil {
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
		payload, err := json.MarshalIndent(envelope.Data, "", "  ")
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
