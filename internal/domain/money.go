package domain

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

var (
	ErrInvalidAmount          = errors.New("invalid amount")
	ErrInvalidAmountPrecision = errors.New("invalid amount precision")
	ErrAmountOverflow         = errors.New("amount overflow")
)

var currencyMinorUnits = map[string]int{
	"BIF": 0,
	"CLP": 0,
	"DJF": 0,
	"GNF": 0,
	"ISK": 0,
	"JPY": 0,
	"KMF": 0,
	"KRW": 0,
	"PYG": 0,
	"RWF": 0,
	"UGX": 0,
	"UYI": 0,
	"VND": 0,
	"VUV": 0,
	"XAF": 0,
	"XOF": 0,
	"XPF": 0,
	"BHD": 3,
	"IQD": 3,
	"JOD": 3,
	"KWD": 3,
	"LYD": 3,
	"OMR": 3,
	"TND": 3,
	"CLF": 4,
	"UYW": 4,
}

func CurrencyMinorUnit(currencyCode string) (int, error) {
	normalized, err := NormalizeCurrencyCode(currencyCode)
	if err != nil {
		return 0, err
	}

	if minorUnit, ok := currencyMinorUnits[normalized]; ok {
		return minorUnit, nil
	}
	return 2, nil
}

func ParseMajorAmountToMinor(amount, currencyCode string) (int64, error) {
	amountValue := strings.TrimSpace(amount)
	if amountValue == "" {
		return 0, ErrInvalidAmount
	}

	if strings.HasPrefix(amountValue, "+") {
		amountValue = strings.TrimPrefix(amountValue, "+")
	}
	if strings.HasPrefix(amountValue, "-") {
		return 0, ErrInvalidAmount
	}

	parts := strings.Split(amountValue, ".")
	if len(parts) > 2 {
		return 0, ErrInvalidAmount
	}

	integerPart := parts[0]
	fractionalPart := ""
	if len(parts) == 2 {
		fractionalPart = parts[1]
	}
	hasIntegerDigits := integerPart != ""
	hasFractionDigits := fractionalPart != ""
	if !hasIntegerDigits && !hasFractionDigits {
		return 0, ErrInvalidAmount
	}
	if len(parts) == 2 && !hasFractionDigits {
		return 0, ErrInvalidAmount
	}

	if integerPart == "" {
		integerPart = "0"
	}
	if !isDigits(integerPart) || (fractionalPart != "" && !isDigits(fractionalPart)) {
		return 0, ErrInvalidAmount
	}

	minorUnit, err := CurrencyMinorUnit(currencyCode)
	if err != nil {
		return 0, err
	}

	if len(fractionalPart) > minorUnit {
		extraDigits := fractionalPart[minorUnit:]
		if strings.Trim(extraDigits, "0") != "" {
			return 0, ErrInvalidAmountPrecision
		}
		fractionalPart = fractionalPart[:minorUnit]
	}

	for len(fractionalPart) < minorUnit {
		fractionalPart += "0"
	}

	major, err := strconv.ParseInt(integerPart, 10, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return 0, ErrAmountOverflow
		}
		return 0, ErrInvalidAmount
	}

	factor, err := int64Pow10(minorUnit)
	if err != nil {
		return 0, err
	}
	if major > math.MaxInt64/factor {
		return 0, ErrAmountOverflow
	}
	minor := major * factor

	if minorUnit == 0 {
		return minor, nil
	}

	if fractionalPart == "" {
		return minor, nil
	}
	fractional, err := strconv.ParseInt(fractionalPart, 10, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return 0, ErrAmountOverflow
		}
		return 0, ErrInvalidAmount
	}
	if fractional > math.MaxInt64-minor {
		return 0, ErrAmountOverflow
	}
	return minor + fractional, nil
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

func int64Pow10(power int) (int64, error) {
	if power < 0 {
		return 0, ErrInvalidAmountPrecision
	}

	result := int64(1)
	for i := 0; i < power; i++ {
		if result > math.MaxInt64/10 {
			return 0, ErrAmountOverflow
		}
		result *= 10
	}
	return result, nil
}
