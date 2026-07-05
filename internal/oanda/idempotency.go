package oanda

import (
	"context"
	"fmt"
	"strconv"
)

// FindOrderByClientID checks open trades and pending orders for a prior submission
// with the same clientExtensions.id (OANDA idempotency key).
func (c *Client) FindOrderByClientID(ctx context.Context, clientID string) (OrderResult, bool, error) {
	if clientID == "" {
		return OrderResult{}, false, nil
	}

	trades, err := c.OpenTrades(ctx)
	if err != nil {
		return OrderResult{}, false, err
	}
	for _, t := range trades.Trades {
		if t.ClientExtensions == nil || t.ClientExtensions.ID != clientID {
			continue
		}
		result, err := tradeToResult(t)
		if err != nil {
			return OrderResult{}, false, err
		}
		return result, true, nil
	}

	pending, err := c.PendingOrders(ctx)
	if err != nil {
		return OrderResult{}, false, err
	}
	for _, o := range pending.Orders {
		if o.ClientExtensions == nil || o.ClientExtensions.ID != clientID {
			continue
		}
		units, _ := strconv.ParseInt(o.Units, 10, 64)
		if units < 0 {
			units = -units
		}
		price, _ := ParsePrice(o.Price)
		return OrderResult{
			OrderID:    o.ID,
			Instrument: o.Instrument,
			Units:      units,
			FillPrice:  price,
		}, true, nil
	}
	return OrderResult{}, false, nil
}

func tradeToResult(t Trade) (OrderResult, error) {
	units, err := strconv.ParseInt(t.CurrentUnits, 10, 64)
	if err != nil {
		return OrderResult{}, fmt.Errorf("parse units: %w", err)
	}
	if units < 0 {
		units = -units
	}
	price, err := ParsePrice(t.Price)
	if err != nil {
		return OrderResult{}, err
	}
	return OrderResult{
		TradeID:    t.ID,
		Instrument: t.Instrument,
		Units:      units,
		FillPrice:  price,
	}, nil
}

// FinancingCost sums swap/financing debits for a trade from recent transactions.
func FinancingCost(transactions []Transaction, tradeID string) float64 {
	var total float64
	for _, tx := range transactions {
		switch tx.Type {
		case "DAILY_FINANCING", "SWAP", "DIVIDEND_ADJUSTMENT":
		default:
			continue
		}
		if tx.Pl == "" {
			continue
		}
		// Financing rows reference the trade via tradeReduced or instrument match.
		if tx.TradeReduced != nil && tx.TradeReduced.TradeID != tradeID {
			continue
		}
		pl, err := ParsePrice(tx.Pl)
		if err != nil {
			continue
		}
		total += pl
	}
	return total
}
