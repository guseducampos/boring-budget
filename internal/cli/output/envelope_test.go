package output

import "testing"

func TestIsValidFormat(t *testing.T) {
	t.Parallel()

	if !IsValidFormat("human") {
		t.Fatalf("expected human to be valid")
	}
	if !IsValidFormat("json") {
		t.Fatalf("expected json to be valid")
	}
	if !IsValidFormat(" JSON ") {
		t.Fatalf("expected spaced mixed-case json to be valid")
	}
	if IsValidFormat("yaml") {
		t.Fatalf("expected yaml to be invalid")
	}
}

func TestNewSuccessEnvelopeDefaults(t *testing.T) {
	t.Parallel()

	env := NewSuccessEnvelope(map[string]any{"hello": "world"}, nil)
	if !env.Ok {
		t.Fatalf("expected ok=true")
	}
	if env.Error != nil {
		t.Fatalf("expected nil error on success")
	}
	if env.Meta.APIVersion != APIVersionV1 {
		t.Fatalf("unexpected api version: %s", env.Meta.APIVersion)
	}
	if len(env.Warnings) != 0 {
		t.Fatalf("expected empty warnings slice by default")
	}
	if env.Meta.TimestampUTC == "" {
		t.Fatalf("expected timestamp_utc to be set")
	}
}
