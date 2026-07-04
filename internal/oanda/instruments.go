package oanda

import (
	"context"
	"fmt"
	"strings"
)

type Instrument struct {
	Name             string  `json:"name"`
	Type             string  `json:"type"`
	DisplayName      string  `json:"displayName"`
	PipLocation      int     `json:"pipLocation"`
	DisplayPrecision int     `json:"displayPrecision"`
	TradeUnitsPrecision int  `json:"tradeUnitsPrecision"`
	MinimumTradeSize string  `json:"minimumTradeSize"`
	MarginRate       string  `json:"marginRate"`
}

type InstrumentsResponse struct {
	Instruments []Instrument `json:"instruments"`
	LastTransactionID string   `json:"lastTransactionID"`
}

func (c *Client) ListInstruments(ctx context.Context) ([]Instrument, error) {
	path := fmt.Sprintf("/v3/accounts/%s/instruments", c.accountID)
	var out InstrumentsResponse
	if err := c.getJSON(ctx, path, nil, &out); err != nil {
		return nil, err
	}
	return out.Instruments, nil
}

// InstrumentPipSize returns price units per pip for an instrument.
func InstrumentPipSize(instrument string, pipLocation int) float64 {
	if pipLocation == -2 {
		return 0.01
	}
	if pipLocation == -1 {
		return 0.1
	}
	upper := strings.ToUpper(instrument)
	if strings.HasPrefix(upper, "XAU") || strings.HasPrefix(upper, "XAG") {
		return 0.01
	}
	if strings.Contains(upper, "JPY") {
		return 0.01
	}
	return 0.0001
}

// SpreadPipsFor converts raw spread to pips/points for instrument.
func SpreadPipsFor(instrument string, pipLocation int, spread float64) float64 {
	pip := InstrumentPipSize(instrument, pipLocation)
	if pip <= 0 {
		return spread
	}
	return spread / pip
}

func InstrumentClass(name string, instType string) string {
	upper := strings.ToUpper(name)
	if name == "BTC_USD" || strings.HasPrefix(upper, "ETH_") {
		return "CRYPTO"
	}
	switch instType {
	case "METAL":
		return "METAL"
	case "CFD":
		if strings.Contains(upper, "USD") && (strings.Contains(upper, "NAS") || strings.Contains(upper, "US") ||
			strings.Contains(upper, "SPX") || strings.Contains(upper, "UK") || strings.Contains(upper, "DE") ||
			strings.Contains(upper, "EU") || strings.Contains(upper, "JP") || strings.Contains(upper, "AU") ||
			strings.Contains(upper, "HK") || strings.Contains(upper, "CN")) {
			return "INDEX"
		}
		if strings.HasSuffix(upper, "_USD") && (strings.Contains(upper, "BCO") || strings.Contains(upper, "WTI") || strings.Contains(upper, "WTICO")) {
			return "ENERGY"
		}
		return "INDEX"
	default:
		if strings.Contains(upper, "JPY") {
			return "JPY"
		}
		return "FX"
	}
}
