package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintHumanLocalizesUTCFields(t *testing.T) {
	original := CurrentDisplayTimezone()
	SetDisplayTimezone("America/New_York")
	t.Cleanup(func() {
		SetDisplayTimezone(original)
	})

	env := NewSuccessEnvelope(map[string]any{
		"created_at_utc": "2026-02-11T15:00:00Z",
		"note":           "unchanged",
	}, nil)

	var out bytes.Buffer
	if err := Print(&out, FormatHuman, env); err != nil {
		t.Fatalf("print human: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "2026-02-11T10:00:00-05:00") {
		t.Fatalf("expected localized timestamp in output, got %s", output)
	}
	if strings.Contains(output, "2026-02-11T15:00:00Z") {
		t.Fatalf("expected utc timestamp to be localized in human output, got %s", output)
	}
}

func TestPrintJSONKeepsUTCTimestamps(t *testing.T) {
	original := CurrentDisplayTimezone()
	SetDisplayTimezone("America/New_York")
	t.Cleanup(func() {
		SetDisplayTimezone(original)
	})

	env := NewSuccessEnvelope(map[string]any{
		"created_at_utc": "2026-02-11T15:00:00Z",
	}, nil)

	var out bytes.Buffer
	if err := Print(&out, FormatJSON, env); err != nil {
		t.Fatalf("print json: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "2026-02-11T15:00:00Z") {
		t.Fatalf("expected JSON output to keep UTC timestamp, got %s", output)
	}
}
