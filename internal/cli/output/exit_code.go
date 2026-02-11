package output

import (
	"strings"
	"sync/atomic"
)

var processExitCode atomic.Int32

func ResetProcessExitCode() {
	processExitCode.Store(0)
}

func CurrentProcessExitCode() int {
	return int(processExitCode.Load())
}

func SetProcessExitCodeFromEnvelope(envelope Envelope) {
	if envelope.Ok || envelope.Error == nil {
		processExitCode.Store(0)
		return
	}

	processExitCode.Store(int32(ExitCodeForErrorCode(envelope.Error.Code)))
}

func ExitCodeForErrorCode(errorCode string) int {
	switch strings.ToUpper(strings.TrimSpace(errorCode)) {
	case "INVALID_ARGUMENT", "INVALID_DATE_RANGE", "INVALID_CURRENCY_CODE":
		return 2
	case "NOT_FOUND":
		return 3
	case "CONFLICT":
		return 4
	case "DB_ERROR":
		return 5
	case "FX_RATE_UNAVAILABLE":
		return 6
	case "CONFIG_ERROR":
		return 7
	case "INTERNAL_ERROR":
		return 1
	default:
		return 1
	}
}
