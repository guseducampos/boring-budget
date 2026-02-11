package output

import "testing"

func TestExitCodeForErrorCode(t *testing.T) {
	t.Parallel()

	cases := []struct {
		code string
		exit int
	}{
		{code: "INVALID_ARGUMENT", exit: 2},
		{code: "NOT_FOUND", exit: 3},
		{code: "CONFLICT", exit: 4},
		{code: "DB_ERROR", exit: 5},
		{code: "FX_RATE_UNAVAILABLE", exit: 6},
		{code: "CONFIG_ERROR", exit: 7},
		{code: "INTERNAL_ERROR", exit: 1},
		{code: "UNKNOWN", exit: 1},
	}

	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			t.Parallel()
			if got := ExitCodeForErrorCode(tc.code); got != tc.exit {
				t.Fatalf("expected exit %d for %s, got %d", tc.exit, tc.code, got)
			}
		})
	}
}

func TestProcessExitCodeFromEnvelope(t *testing.T) {
	t.Parallel()

	ResetProcessExitCode()
	SetProcessExitCodeFromEnvelope(NewSuccessEnvelope(map[string]any{"ok": true}, nil))
	if got := CurrentProcessExitCode(); got != 0 {
		t.Fatalf("expected exit code 0 for success, got %d", got)
	}

	SetProcessExitCodeFromEnvelope(NewErrorEnvelope("NOT_FOUND", "missing", map[string]any{}, nil))
	if got := CurrentProcessExitCode(); got != 3 {
		t.Fatalf("expected exit code 3 for NOT_FOUND, got %d", got)
	}
}
