package strategy

import (
	"fmt"

	"github.com/ym/fxtrade/internal/oanda"
)

// pendingOnOtherInstruments reports whether any pending order exists outside
// the given instrument (account-wide exposure when max_open_positions is 1).
func pendingOnOtherInstruments(pending []oanda.PendingOrder, instrument string) bool {
	for _, o := range pending {
		if o.Instrument != instrument {
			return true
		}
	}
	return false
}

// pendingElsewhere returns pending orders on instruments other than the given one.
func pendingElsewhere(pending []oanda.PendingOrder, instrument string) []oanda.PendingOrder {
	var out []oanda.PendingOrder
	for _, o := range pending {
		if o.Instrument != instrument {
			out = append(out, o)
		}
	}
	return out
}

func accountExposureBlocked(openTrades []oanda.Trade, pending []oanda.PendingOrder, instrument string, maxOpen int) (bool, string) {
	if maxOpen <= 0 {
		maxOpen = 1
	}
	_, otherTrades := splitTradesByInstrument(openTrades, instrument)
	if len(otherTrades) > 0 {
		return true, fmt.Sprintf("position open on %s — account-wide cap is %d", otherTrades[0].Instrument, maxOpen)
	}
	if maxOpen == 1 && pendingOnOtherInstruments(pending, instrument) {
		other := pendingElsewhere(pending, instrument)
		return true, fmt.Sprintf("pending order on %s blocks new entries (account-wide cap is %d)", other[0].Instrument, maxOpen)
	}
	return false, ""
}
