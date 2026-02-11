package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidLabelName  = errors.New("invalid label name")
	ErrInvalidLabelID    = errors.New("invalid label id")
	ErrLabelNotFound     = errors.New("label not found")
	ErrLabelNameConflict = errors.New("label name conflict")
	ErrStorage           = errors.New("label storage error")
)

type Label struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	CreatedAtUTC time.Time  `json:"created_at_utc"`
	UpdatedAtUTC time.Time  `json:"updated_at_utc"`
	DeletedAtUTC *time.Time `json:"deleted_at_utc,omitempty"`
}

type LabelDeleteResult struct {
	LabelID       int64     `json:"label_id"`
	DetachedLinks int64     `json:"detached_links"`
	DeletedAtUTC  time.Time `json:"deleted_at_utc"`
}

func NormalizeLabelName(name string) (string, error) {
	normalized := strings.TrimSpace(name)
	if normalized == "" {
		return "", ErrInvalidLabelName
	}
	return normalized, nil
}

func ValidateLabelID(id int64) error {
	if id <= 0 {
		return ErrInvalidLabelID
	}
	return nil
}
