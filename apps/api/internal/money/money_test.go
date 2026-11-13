package money

import (
	"errors"
	"testing"
)

func TestParseYuanAndFormat(t *testing.T) {
	cases := []struct {
		in   string
		want Minor
		str  string
	}{
		{"100.00", 10000, "100.00"},
		{"0.01", 1, "0.01"},
		{"100", 10000, "100.00"},
		{"0.5", 50, "0.50"},
		{"-12.34", -1234, "-12.34"},
		{"24800", 2480000, "24800.00"},
	}
	for _, c := range cases {
		got, err := ParseYuan(c.in)
		if err != nil {
			t.Fatalf("ParseYuan(%q): %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("ParseYuan(%q)=%d want %d", c.in, got, c.want)
		}
		if got.String() != c.str {
			t.Errorf("Minor(%d).String()=%q want %q", got, got.String(), c.str)
		}
	}
}

func TestParseYuanRejectsFloatLikeInput(t *testing.T) {
	for _, s := range []string{"", "abc", "1.234", ".", "1,000", "--1", "1.2.3"} {
		if _, err := ParseYuan(s); !errors.Is(err, ErrInvalidAmount) {
			t.Errorf("ParseYuan(%q) should reject as invalid, got %v", s, err)
		}
	}
}

func TestDivRoundHalfUp(t *testing.T) {
	cases := []struct {
		num, den, want int64
	}{
		{5, 2, 3}, {-5, 2, -3}, {4, 2, 2},
		{1, 3, 0}, {2, 3, 1}, {-2, 3, -1},
		{221000, 200, 1105}, // 2210.00 / 200 shares -> 11.05 (scaled by 100)
	}
	for _, c := range cases {
		got, err := DivRound(c.num, c.den)
		if err != nil {
			t.Fatalf("DivRound(%d,%d): %v", c.num, c.den, err)
		}
		if got != c.want {
			t.Errorf("DivRound(%d,%d)=%d want %d", c.num, c.den, got, c.want)
		}
	}
	if _, err := DivRound(1, 0); !errors.Is(err, ErrInvalidAmount) {
		t.Error("zero divisor must be rejected")
	}
}

func TestMulBPSAndMinimumFee(t *testing.T) {
	// 100.00 CNY at 25bps = 0.25 CNY = 25 cents
	got, err := MulBPS(10000, 25)
	if err != nil || got != 25 {
		t.Fatalf("MulBPS(10000,25)=%d err=%v want 25", got, err)
	}
	// half-up: 1 cent at 50% -> 1 cent (0.5 rounds up)
	got, err = MulBPS(1, 5000)
	if err != nil || got != 1 {
		t.Fatalf("MulBPS(1,5000)=%d err=%v want 1", got, err)
	}
	// commission floor: 3 cents computed, 5 cents minimum -> 5
	fee, err := FeeWithMinimum(10000, 3, 5)
	if err != nil || fee != 5 {
		t.Fatalf("FeeWithMinimum=%d err=%v want 5", fee, err)
	}
}

func TestAddOverflow(t *testing.T) {
	if _, err := Add(Minor(1<<63-1), 1); !errors.Is(err, ErrOverflow) {
		t.Error("overflow must be detected")
	}
}
