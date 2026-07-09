package oanda

import (
	"strconv"
	"strings"
)

// TradeDirection returns LONG or SHORT from signed unit count.
func TradeDirection(units int64) string {
	if units < 0 {
		return "SHORT"
	}
	if units > 0 {
		return "LONG"
	}
	return ""
}

// TradeDirectionStr parses OANDA unit strings (may be negative).
func TradeDirectionStr(units string) string {
	u, err := strconv.ParseInt(strings.TrimSpace(units), 10, 64)
	if err != nil {
		return ""
	}
	return TradeDirection(u)
}

// EstimateNotionalUSD approximates USD exposure for common OANDA instruments.
func EstimateNotionalUSD(instrument string, units int64, price float64) (float64, bool) {
	if price <= 0 || units == 0 {
		return 0, false
	}
	abs := units
	if abs < 0 {
		abs = -abs
	}
	parts := strings.Split(strings.ToUpper(strings.TrimSpace(instrument)), "_")
	if len(parts) != 2 {
		return 0, false
	}
	base, quote := parts[0], parts[1]
	switch {
	case quote == "USD":
		return float64(abs) * price, true
	case base == "USD":
		return float64(abs), true
	default:
		return 0, false
	}
}

// ClosedTradeInstrument returns the instrument from a trade-close transaction.
func ClosedTradeInstrument(transactions []Transaction, tradeID string) (string, bool) {
	for _, tx := range transactions {
		for _, tc := range tx.TradesClosed {
			if tc.TradeID != tradeID {
				continue
			}
			if inst := strings.TrimSpace(tx.Instrument); inst != "" {
				return inst, true
			}
		}
	}
	return "", false
}

// ClosedTradeDetails looks up close price and units from recent transactions.
func ClosedTradeDetails(transactions []Transaction, tradeID string) (exitPrice float64, unitsClosed int64, ok bool) {
	for _, tx := range transactions {
		for _, tc := range tx.TradesClosed {
			if tc.TradeID != tradeID {
				continue
			}
			if tx.Price != "" {
				if p, err := ParsePrice(tx.Price); err == nil {
					exitPrice = p
				}
			}
			if tc.Units != "" {
				if u, err := strconv.ParseInt(tc.Units, 10, 64); err == nil {
					unitsClosed = u
					if unitsClosed < 0 {
						unitsClosed = -unitsClosed
					}
				}
			}
			return exitPrice, unitsClosed, true
		}
	}
	return 0, 0, false
}
