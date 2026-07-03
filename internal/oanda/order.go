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
	if resp == nil || resp.OrderFillTransaction == nil {
		return 0, fmt.Errorf("no close fill transaction")
	}
	fill := resp.OrderFillTransaction
	if fill.Pl != "" {
		return ParsePrice(fill.Pl)
	}
	var total float64
	for _, tc := range fill.TradesClosed {
		if tc.RealizedPL == "" {
			continue
		}
		p, err := ParsePrice(tc.RealizedPL)
		if err != nil {
			return 0, err
		}
		total += p
	}
	return total, nil
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
