package oanda

import (
	"fmt"
	"strconv"
)

func ParseOrderResult(resp *CreateOrderResponse) (OrderResult, error) {
	if resp == nil {
		return OrderResult{}, fmt.Errorf("nil order response")
	}

	var fill *Transaction
	if resp.OrderFillTransaction != nil {
		fill = resp.OrderFillTransaction
	}

	if fill == nil {
		return OrderResult{}, fmt.Errorf("order not filled (cancelled or rejected)")
	}

	result := OrderResult{
		TransactionID: fill.ID,
		Instrument:    fill.Instrument,
	}

	if fill.Price != "" {
		p, err := ParsePrice(fill.Price)
		if err != nil {
			return OrderResult{}, err
		}
		result.FillPrice = p
	}

	if fill.Units != "" {
		u, err := strconv.ParseInt(fill.Units, 10, 64)
		if err != nil {
			return OrderResult{}, err
		}
		result.Units = u
	}

	if fill.TradeOpened != nil {
		result.TradeID = fill.TradeOpened.TradeID
		if result.Units == 0 && fill.TradeOpened.Units != "" {
			u, err := strconv.ParseInt(fill.TradeOpened.Units, 10, 64)
			if err != nil {
				return OrderResult{}, err
			}
			result.Units = u
		}
		if result.FillPrice == 0 && fill.TradeOpened.Price != "" {
			p, err := ParsePrice(fill.TradeOpened.Price)
			if err != nil {
				return OrderResult{}, err
			}
			result.FillPrice = p
		}
	}

	if result.TradeID == "" {
		return OrderResult{}, fmt.Errorf("filled order missing trade ID")
	}

	return result, nil
}

func ParseCreateOrderResult(resp *CreateOrderResponse) (OrderResult, error) {
	if resp == nil {
		return OrderResult{}, fmt.Errorf("nil order response")
	}
	if resp.OrderFillTransaction != nil {
		return ParseOrderResult(resp)
	}
	if resp.OrderCreateTransaction != nil {
		tx := resp.OrderCreateTransaction
		result := OrderResult{
			TransactionID: tx.ID,
			OrderID:       tx.ID,
			Instrument:    tx.Instrument,
		}
		if tx.Units != "" {
			u, _ := strconv.ParseInt(tx.Units, 10, 64)
			result.Units = u
		}
		if tx.Price != "" {
			p, _ := ParsePrice(tx.Price)
			result.FillPrice = p
		}
		return result, nil
	}
	return OrderResult{}, fmt.Errorf("order response missing transaction")
}

func RealizedPL(resp *CloseTradeResponse) (float64, error) {
	details := CloseFillDetails(resp)
	if !details.PLKnown {
		return 0, fmt.Errorf("no close fill transaction")
	}
	return details.RealizedPL, nil
}

// CloseFillDetails extracts exit price, units, and P&L from a close response.
func CloseFillDetails(resp *CloseTradeResponse) CloseDetails {
	var out CloseDetails
	if resp == nil || resp.OrderFillTransaction == nil {
		return out
	}
	fill := resp.OrderFillTransaction
	out.Instrument = fill.Instrument
	if fill.Price != "" {
		if p, err := ParsePrice(fill.Price); err == nil {
			out.ExitPrice = p
			out.HasExit = true
		}
	}
	if fill.Units != "" {
		if u, err := strconv.ParseInt(fill.Units, 10, 64); err == nil {
			out.UnitsClosed = u
		}
	}
	if fill.Pl != "" {
		if p, err := ParsePrice(fill.Pl); err == nil {
			out.RealizedPL = p
			out.PLKnown = true
			return out
		}
	}
	var total float64
	for _, tc := range fill.TradesClosed {
		if tc.RealizedPL == "" {
			continue
		}
		p, err := ParsePrice(tc.RealizedPL)
		if err != nil {
			continue
		}
		total += p
		out.PLKnown = true
		if out.UnitsClosed == 0 && tc.Units != "" {
			if u, err := strconv.ParseInt(tc.Units, 10, 64); err == nil {
				out.UnitsClosed = u
			}
		}
	}
	out.RealizedPL = total
	return out
}

type CloseDetails struct {
	Instrument  string
	UnitsClosed int64
	ExitPrice   float64
	HasExit     bool
	RealizedPL  float64
	PLKnown     bool
}

// ClosedTradePL sums realized P&L from recent transactions for a closed trade.
func ClosedTradePL(transactions []Transaction, tradeID string) (float64, bool) {
	var total float64
	found := false
	for _, tx := range transactions {
		for _, tc := range tx.TradesClosed {
			if tc.TradeID != tradeID {
				continue
			}
			if tc.RealizedPL == "" {
				continue
			}
			p, err := ParsePrice(tc.RealizedPL)
			if err != nil {
				continue
			}
			total += p
			found = true
		}
	}
	return total, found
}
