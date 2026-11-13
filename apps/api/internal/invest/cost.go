// Package invest implements moving-weighted-average cost and realised/unrealised P&L.
// All arithmetic is integer-only; no float ever touches a financial number.
package invest

import (
	"errors"
	"fmt"
	"math/big"

	"tinynest/api/internal/money"
)

var (
	ErrInvalidQty      = errors.New("invest: invalid quantity")
	ErrInsufficientQty = errors.New("invest: sell quantity exceeds holding")
	ErrInvalidCost     = errors.New("invest: cost must be positive")
	ErrInvalidProceeds = errors.New("invest: net proceeds must be positive")
)

// Position is a rebuildable projection of a holding. The journal remains the source of truth.
type Position struct {
	QtyScaled        int64       `json:"quantity_scaled"`
	TotalCostMinor   money.Minor `json:"total_cost_minor"`
	Scale            int64       `json:"quantity_scale"`
	RealizedPnLMinor money.Minor `json:"realized_pnl_minor"`
	NetDividendMinor money.Minor `json:"net_dividend_minor"`
}

func NewPosition(scale int64) Position {
	if scale <= 0 {
		scale = money.QuantityScale
	}
	return Position{Scale: scale}
}

// proportional returns round-half-up(total*part/whole) using big.Int to avoid overflow.
func proportional(total, part, whole int64) (int64, error) {
	if whole <= 0 || part < 0 {
		return 0, ErrInvalidQty
	}
	n := big.NewInt(total)
	n.Mul(n, big.NewInt(part))
	half := big.NewInt(whole / 2)
	if n.Sign() >= 0 {
		n.Add(n, half)
	} else {
		n.Sub(n, half)
	}
	n.Quo(n, big.NewInt(whole))
	if !n.IsInt64() {
		return 0, money.ErrOverflow
	}
	return n.Int64(), nil
}

// Buy adds quantity and capitalises the full cost (gross + buy fees).
func (p *Position) Buy(qtyScaled int64, costMinor money.Minor) error {
	if qtyScaled <= 0 {
		return ErrInvalidQty
	}
	if costMinor <= 0 {
		return ErrInvalidCost
	}
	var err error
	if p.QtyScaled, err = safeAdd(p.QtyScaled, qtyScaled); err != nil {
		return err
	}
	if p.TotalCostMinor, err = money.Add(p.TotalCostMinor, costMinor); err != nil {
		return err
	}
	return nil
}

func safeAdd(a, b int64) (int64, error) {
	if b > 0 && a > (1<<63-1)-b {
		return 0, money.ErrOverflow
	}
	return a + b, nil
}

// AvgCostMinorPerUnit returns the moving average cost in minor units per whole share.
func (p Position) AvgCostMinorPerUnit() (money.Minor, error) {
	if p.QtyScaled <= 0 {
		return 0, nil
	}
	v, err := proportional(int64(p.TotalCostMinor), p.Scale, p.QtyScaled)
	if err != nil {
		return 0, err
	}
	return money.Minor(v), nil
}

// Sell reduces the holding and returns (carriedCost, realisedPnL).
// On a full clear-out the entire remaining cost is carried, eliminating rounding residue.
func (p *Position) Sell(qtyScaled int64, netProceedsMinor money.Minor) (money.Minor, money.Minor, error) {
	if qtyScaled <= 0 || qtyScaled > p.QtyScaled {
		return 0, 0, fmt.Errorf("%w: have %d, sell %d", ErrInsufficientQty, p.QtyScaled, qtyScaled)
	}
	if netProceedsMinor <= 0 {
		return 0, 0, ErrInvalidProceeds
	}
	var carried int64
	var err error
	if qtyScaled == p.QtyScaled {
		carried = int64(p.TotalCostMinor)
	} else {
		carried, err = proportional(int64(p.TotalCostMinor), qtyScaled, p.QtyScaled)
		if err != nil {
			return 0, 0, err
		}
	}
	carryMinor := money.Minor(carried)
	realized, err := money.Sub(netProceedsMinor, carryMinor)
	if err != nil {
		return 0, 0, err
	}
	p.QtyScaled -= qtyScaled
	p.TotalCostMinor -= carryMinor
	p.RealizedPnLMinor += realized
	return carryMinor, realized, nil
}

// AddDividend records a net dividend. taxMinor == nil means "unknown" and must be
// surfaced as such by the caller; it is never silently treated as zero.
func (p *Position) AddDividend(gross, net money.Minor, taxMinor *money.Minor) error {
	if taxMinor != nil && gross != net+*taxMinor {
		return fmt.Errorf("invest: gross(%d) != net(%d) + tax(%d)", gross, net, *taxMinor)
	}
	var err error
	if p.NetDividendMinor, err = money.Add(p.NetDividendMinor, net); err != nil {
		return err
	}
	return nil
}

// UnrealizedPnL uses a manually entered valuation; historical trades are untouched.
func (p Position) UnrealizedPnL(marketValueMinor money.Minor) money.Minor {
	return marketValueMinor - p.TotalCostMinor
}

// TotalReturn = realised + unrealised + net dividends.
func (p Position) TotalReturn(marketValueMinor money.Minor) money.Minor {
	return p.RealizedPnLMinor + p.UnrealizedPnL(marketValueMinor) + p.NetDividendMinor
}

// ReferenceReturnBPS = total return / historical cumulative buy cost, in basis points.
// It is NOT an annualised return and must never be labelled as such.
func (p Position) ReferenceReturnBPS(marketValueMinor, cumulativeBuyCostMinor money.Minor) (int64, error) {
	if cumulativeBuyCostMinor <= 0 {
		return 0, ErrInvalidCost
	}
	return money.DivRound(int64(p.TotalReturn(marketValueMinor))*10000, int64(cumulativeBuyCostMinor))
}
