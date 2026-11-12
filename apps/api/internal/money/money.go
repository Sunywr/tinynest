// Package money implements integer-minor (cents) arithmetic for TinyNest.
// Financial amounts must NEVER use floating point anywhere in the codebase.
package money

import (
	"errors"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

var (
	ErrInvalidAmount = errors.New("money: invalid amount")
	ErrOverflow      = errors.New("money: arithmetic overflow")
)

// Minor is an amount in cents (1 CNY = 100 Minor).
type Minor int64

// QuantityScale is the fixed-point scale for share quantities and prices.
const QuantityScale int64 = 1_000_000

// ParseYuan converts a decimal CNY string into Minor without any float.
// At most 2 fractional digits are accepted; rounding is never implicit here.
func ParseYuan(s string) (Minor, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, ErrInvalidAmount
	}
	neg := false
	if strings.HasPrefix(s, "-") {
		neg, s = true, s[1:]
	} else if strings.HasPrefix(s, "+") {
		s = s[1:]
	}
	parts := strings.SplitN(s, ".", 2)
	if parts[0] == "" || !allDigits(parts[0]) {
		return 0, ErrInvalidAmount
	}
	yuan, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %s", ErrInvalidAmount, s)
	}
	var cents int64
	if len(parts) == 2 {
		frac := parts[1]
		if frac == "" || len(frac) > 2 || !allDigits(frac) {
			return 0, fmt.Errorf("%w: %s (max 2 decimals)", ErrInvalidAmount, s)
		}
		for len(frac) < 2 {
			frac += "0"
		}
		cents, err = strconv.ParseInt(frac, 10, 64)
		if err != nil {
			return 0, ErrInvalidAmount
		}
	}
	if yuan > (1<<63-1-cents)/100 {
		return 0, ErrOverflow
	}
	total := yuan*100 + cents
	if neg {
		total = -total
	}
	return Minor(total), nil
}

func allDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// String renders the amount as a CNY decimal string, e.g. 10000 -> "100.00".
func (m Minor) String() string {
	sign := ""
	if m < 0 {
		sign, m = "-", -m
	}
	return fmt.Sprintf("%s%d.%02d", sign, int64(m)/100, int64(m)%100)
}

// Add / Sub guard against int64 overflow.
func Add(a, b Minor) (Minor, error) {
	r := int64(a) + int64(b)
	if (b > 0 && r < int64(a)) || (b < 0 && r > int64(a)) {
		return 0, ErrOverflow
	}
	return Minor(r), nil
}

func Sub(a, b Minor) (Minor, error) {
	return Add(a, -b)
}

// DivRound computes round-half-up(num/den) in the integer domain. den must be > 0.
// big.Int is used to avoid overflow when num and den are large.
func DivRound(num, den int64) (int64, error) {
	if den <= 0 {
		return 0, fmt.Errorf("%w: divisor must be positive", ErrInvalidAmount)
	}
	n := big.NewInt(num)
	d := big.NewInt(den)
	// half = floor(den/2); an exact .5 remainder only exists when den is even,
	// so floor is sufficient and keeps odd denominators correct (e.g. 1/3 -> 0).
	half := big.NewInt(den / 2)
	if num >= 0 {
		n.Add(n, half)
	} else {
		n.Sub(n, half)
	}
	n.Quo(n, d)
	if !n.IsInt64() {
		return 0, ErrOverflow
	}
	return n.Int64(), nil
}

// MulBPS computes round-half-up(amount * bps / 10000). bps = basis points.
func MulBPS(a Minor, bps int64) (Minor, error) {
	if bps < 0 {
		return 0, fmt.Errorf("%w: negative rate", ErrInvalidAmount)
	}
	prod := big.NewInt(int64(a))
	prod.Mul(prod, big.NewInt(bps))
	half := big.NewInt(5000)
	if prod.Sign() >= 0 {
		prod.Add(prod, half)
	} else {
		prod.Sub(prod, half)
	}
	prod.Quo(prod, big.NewInt(10000))
	if !prod.IsInt64() {
		return 0, ErrOverflow
	}
	return Minor(prod.Int64()), nil
}

// FeeWithMinimum returns max(round-half-up(gross*bps/10000), minFee).
func FeeWithMinimum(gross Minor, bps int64, minFee Minor) (Minor, error) {
	computed, err := MulBPS(gross, bps)
	if err != nil {
		return 0, err
	}
	if minFee > computed {
		return minFee, nil
	}
	return computed, nil
}

// MustPositive validates that a business amount is strictly greater than zero.
func MustPositive(m Minor) error {
	if m <= 0 {
		return fmt.Errorf("%w: amount must be positive, got %s", ErrInvalidAmount, m)
	}
	return nil
}
