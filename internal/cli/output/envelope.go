package output

import "time"

const (
	APIVersionV1 = "v1"
	FormatHuman  = "human"
	FormatJSON   = "json"
)

type Envelope struct {
	Ok       bool             `json:"ok"`
	Data     any              `json:"data"`
	Warnings []WarningPayload `json:"warnings"`
	Error    *ErrorPayload    `json:"error"`
	Meta     Meta             `json:"meta"`
}

type WarningPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details"`
}

type Meta struct {
	APIVersion   string `json:"api_version"`
	TimestampUTC string `json:"timestamp_utc"`
}

func NewSuccessEnvelope(data any, warnings []WarningPayload) Envelope {
	if warnings == nil {
		warnings = []WarningPayload{}
	}

	return Envelope{
		Ok:       true,
		Data:     data,
		Warnings: warnings,
		Error:    nil,
		Meta:     NewMetaNow(),
	}
}

func NewErrorEnvelope(code, message string, details any, warnings []WarningPayload) Envelope {
	if warnings == nil {
		warnings = []WarningPayload{}
	}

	return Envelope{
		Ok:       false,
		Data:     nil,
		Warnings: warnings,
		Error: &ErrorPayload{
			Code:    code,
			Message: message,
			Details: details,
		},
		Meta: NewMetaNow(),
	}
}

func NewMetaNow() Meta {
	return Meta{
		APIVersion:   APIVersionV1,
		TimestampUTC: time.Now().UTC().Format(time.RFC3339Nano),
	}
}
