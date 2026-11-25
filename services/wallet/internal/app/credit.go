package app

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

var ErrInvalidAmount = errors.New("invalid amount")
var ErrConflict = errors.New("idempotency conflict")

type CreditResult struct {
	OperationID string
	Available   int64
	Reserved    int64
}

// ParseAmount converts a positive decimal string to minor units without floating point.
func ParseAmount(s string) (int64, error) {
	parts := strings.Split(s, ".")
	if len(parts) > 2 || len(parts[0]) == 0 {
		return 0, ErrInvalidAmount
	}
	for _, p := range parts {
		if len(p) == 0 {
			return 0, ErrInvalidAmount
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return 0, ErrInvalidAmount
			}
		}
	}
	fraction := "00"
	if len(parts) == 2 {
		if len(parts[1]) > 2 {
			return 0, ErrInvalidAmount
		}
		fraction = parts[1]
		if len(fraction) == 1 {
			fraction += "0"
		}
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole > math.MaxInt64/100 {
		return 0, ErrInvalidAmount
	}
	frac, _ := strconv.ParseInt(fraction, 10, 64)
	if whole == math.MaxInt64/100 && frac > math.MaxInt64%100 {
		return 0, ErrInvalidAmount
	}
	n := whole*100 + frac
	if n <= 0 {
		return 0, ErrInvalidAmount
	}
	return n, nil
}
