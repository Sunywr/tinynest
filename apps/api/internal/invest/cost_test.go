package invest

import (
	"errors"
	"testing"

	"tinynest/api/internal/money"
)

const S = money.QuantityScale

func shares(n int64) int64 { return n * S }

// Cases 18-21: consecutive buys, partial sale, full clear-out, then re-buy.
func TestCase18to21_MovingAverageCost(t *testing.T) {
	p := NewPosition(S)
	// Buy 100 shares @10.00 with 5.00 fee -> cost 1,005.00
	if err := p.Buy(shares(100), 100_500); err != nil {
		t.Fatal(err)
	}
	if p.TotalCostMinor != 100_500 || p.QtyScaled != shares(100) {
		t.Fatalf("after first buy: qty=%d cost=%d", p.QtyScaled, p.TotalCostMinor)
	}

	// Buy 100 more @12.00 with 5.00 fee -> 200 shares, total cost 2,210.00, avg 11.05
	if err := p.Buy(shares(100), 120_500); err != nil {
		t.Fatal(err)
	}
	if p.QtyScaled != shares(200) || p.TotalCostMinor != 221_000 {
		t.Fatalf("after second buy: qty=%d cost=%d, want 200/221000", p.QtyScaled, p.TotalCostMinor)
	}
	avg, err := p.AvgCostMinorPerUnit()
	if err != nil || avg != 1_105 {
		t.Fatalf("avg cost = %d cents, want 1105 (11.05 CNY), err=%v", avg, err)
	}

	// Sell 50 @13.00 with 5.00 fee -> net 645.00, carried cost 552.50, realised 92.50
	carry, realized, err := p.Sell(shares(50), 64_500)
	if err != nil {
		t.Fatal(err)
	}
	if carry != 55_250 || realized != 9_250 {
		t.Fatalf("carry=%d realised=%d, want 55250/9250", carry, realized)
	}
	if p.QtyScaled != shares(150) || p.TotalCostMinor != 165_750 {
		t.Fatalf("after partial sale: qty=%d cost=%d, want 150/165750", p.QtyScaled, p.TotalCostMinor)
	}

	// Clear out: all remaining cost is carried, no residue left behind.
	carry2, realized2, err := p.Sell(shares(150), 200_000)
	if err != nil {
		t.Fatal(err)
	}
	if carry2 != 165_750 {
		t.Errorf("full clear-out carried cost = %d, want 165750", carry2)
	}
	if p.QtyScaled != 0 || p.TotalCostMinor != 0 {
		t.Errorf("after clear-out qty=%d cost=%d, want 0/0", p.QtyScaled, p.TotalCostMinor)
	}
	if p.RealizedPnLMinor != realized+realized2 {
		t.Errorf("historical realised P&L not retained: %d", p.RealizedPnLMinor)
	}

	// Re-buy after clearing works and does not resurrect old cost.
	if err := p.Buy(shares(10), 10_000); err != nil {
		t.Fatal(err)
	}
	if p.TotalCostMinor != 10_000 || p.QtyScaled != shares(10) {
		t.Errorf("re-buy qty=%d cost=%d", p.QtyScaled, p.TotalCostMinor)
	}
}

// Case 22: net dividend = gross - tax; an unknown tax is never assumed zero.
func TestCase22_Dividend(t *testing.T) {
	p := NewPosition(S)
	tax := money.Minor(1_000)
	if err := p.AddDividend(10_000, 9_000, &tax); err != nil {
		t.Fatal(err)
	}
	if p.NetDividendMinor != 9_000 {
		t.Errorf("net dividend = %d, want 9000", p.NetDividendMinor)
	}
	// A nil tax means "unknown": the calculator only accumulates what was received,
	// and the business row must surface tax_unknown instead of implying a zero tax.
	if err := p.AddDividend(10_000, 9_500, nil); err != nil {
		t.Fatalf("unknown tax must be accepted: %v", err)
	}
	if p.NetDividendMinor != 18_500 {
		t.Errorf("net dividend = %d, want 18500", p.NetDividendMinor)
	}
	if err := p.AddDividend(10_000, 8_000, func() *money.Minor { v := money.Minor(1_000); return &v }()); err == nil {
		t.Error("gross != net + tax must be rejected")
	}
}

// Case 23: a manual valuation changes unrealised P&L but never historical cost/trades.
func TestCase23_Valuation(t *testing.T) {
	p := NewPosition(S)
	_ = p.Buy(shares(100), 100_500)
	before := p.TotalCostMinor
	if got := p.UnrealizedPnL(120_000); got != 19_500 {
		t.Errorf("unrealised = %d, want 19500", got)
	}
	if p.TotalCostMinor != before {
		t.Error("valuation must not mutate historical cost")
	}
	// total return = realised(0) + unrealised + net dividend
	_ = p.AddDividend(10_000, 9_000, func() *money.Minor { v := money.Minor(1_000); return &v }())
	if got := p.TotalReturn(120_000); got != 28_500 {
		t.Errorf("total return = %d, want 28500", got)
	}
	bps, err := p.ReferenceReturnBPS(120_000, 100_500)
	if err != nil {
		t.Fatal(err)
	}
	// 28500 / 100500 = 28.36% -> 2836 bps
	if bps < 2830 || bps > 2840 {
		t.Errorf("reference return = %d bps, want ~2836", bps)
	}
}

func TestSellGuards(t *testing.T) {
	p := NewPosition(S)
	_ = p.Buy(shares(10), 10_000)
	if _, _, err := p.Sell(shares(11), 10_000); !errors.Is(err, ErrInsufficientQty) {
		t.Errorf("over-sell must be rejected, got %v", err)
	}
	if _, _, err := p.Sell(0, 10_000); !errors.Is(err, ErrInsufficientQty) {
		t.Errorf("zero sell must be rejected, got %v", err)
	}
	if err := p.Buy(shares(1), 0); !errors.Is(err, ErrInvalidCost) {
		t.Errorf("zero cost must be rejected, got %v", err)
	}
}
