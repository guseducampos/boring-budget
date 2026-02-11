package domain

import (
	"errors"
	"strings"
)

const CategoryNameMaxLength = 120

var (
	ErrInvalidCategoryID    = errors.New("invalid category id")
	ErrCategoryNameRequired = errors.New("category name is required")
	ErrCategoryNameTooLong  = errors.New("category name exceeds maximum length")
	ErrCategoryNotFound     = errors.New("category not found")
	ErrCategoryNameConflict = errors.New("category name already exists")
)

type Category struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	CreatedAtUTC string `json:"created_at_utc"`
	UpdatedAtUTC string `json:"updated_at_utc"`
}

type CategoryDeleteResult struct {
	CategoryID           int64  `json:"category_id"`
	DeletedAtUTC         string `json:"deleted_at_utc"`
	OrphanedTransactions int64  `json:"orphaned_transactions"`
}

func NormalizeCategoryName(name string) (string, error) {
	normalized := strings.TrimSpace(name)
	if normalized == "" {
		return "", ErrCategoryNameRequired
	}
	if len(normalized) > CategoryNameMaxLength {
		return "", ErrCategoryNameTooLong
	}
	return normalized, nil
}
