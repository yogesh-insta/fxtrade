package notify

import (
	"fmt"
	"math"
	"strings"

	"github.com/ym/fxtrade/internal/oanda"
)

// TradeOpen describes a new position fill.
type TradeOpen struct {
	Instrument    string
	Direction     string
	Units         int64
	FillPrice     float64
	StopLoss      *float64
	TakeProfit    *float64
	TradeID       string
	CorrelationID string
}

// TradeClose describes a closed or partially closed position.
type TradeClose struct {
	Instrument    string
	Direction     string
	Units         int64
	EntryPrice    float64
	ExitPrice     float64
	RealizedPL    float64
	PLKnown       bool
	Reason        string
	TradeID       string
	CorrelationID string
	Partial       bool
}

func FormatTradeOpen(ev TradeOpen) string {
	dir := strings.ToUpper(strings.TrimSpace(ev.Direction))
	if dir == "" {
		dir = oanda.TradeDirection(ev.Units)
	}
	absUnits := ev.Units
	if absUnits < 0 {
		absUnits = -absUnits
	}

	var b strings.Builder
	fmt.Fprintf(&b, "=== TRADE OPENED ===\n\n")
	fmt.Fprintf(&b, "Instrument:  %s\n", ev.Instrument)
	fmt.Fprintf(&b, "Direction:   %s\n", dir)
	fmt.Fprintf(&b, "Size:        %s units\n", formatUnits(absUnits))
	if ev.FillPrice > 0 {
		fmt.Fprintf(&b, "Entry:       %s\n", oanda.FormatPrice(ev.FillPrice))
		if usd, ok := oanda.EstimateNotionalUSD(ev.Instrument, ev.Units, ev.FillPrice); ok {
			fmt.Fprintf(&b, "Notional:    ~$%s\n", formatMoney(usd))
		}
	}
	if ev.StopLoss != nil {
		fmt.Fprintf(&b, "Stop loss:   %s\n", oanda.FormatPrice(*ev.StopLoss))
	}
	if ev.TakeProfit != nil {
		fmt.Fprintf(&b, "Take profit: %s\n", oanda.FormatPrice(*ev.TakeProfit))
	}
	if ev.TradeID != "" {
		fmt.Fprintf(&b, "\nTrade ID:    %s\n", ev.TradeID)
	}
	if ev.CorrelationID != "" {
		fmt.Fprintf(&b, "Ref:         %s\n", ev.CorrelationID)
	}
	return b.String()
}

func FormatTradeClose(ev TradeClose) string {
	dir := strings.ToUpper(strings.TrimSpace(ev.Direction))
	if dir == "" && ev.Units != 0 {
		dir = oanda.TradeDirection(ev.Units)
	}
	absUnits := ev.Units
	if absUnits < 0 {
		absUnits = -absUnits
	}

	kind := "TRADE CLOSED"
	if ev.Partial {
		kind = "PARTIAL CLOSE"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "=== %s ===\n\n", kind)
	if ev.Instrument != "" {
		fmt.Fprintf(&b, "Instrument:  %s\n", ev.Instrument)
	}
	if dir != "" {
		fmt.Fprintf(&b, "Direction:   %s\n", dir)
	}
	if absUnits > 0 {
		fmt.Fprintf(&b, "Size:        %s units\n", formatUnits(absUnits))
	}
	if ev.EntryPrice > 0 {
		fmt.Fprintf(&b, "Entry:       %s\n", oanda.FormatPrice(ev.EntryPrice))
	}
	if ev.ExitPrice > 0 {
		fmt.Fprintf(&b, "Exit:        %s\n", oanda.FormatPrice(ev.ExitPrice))
		if ev.Instrument != "" && absUnits > 0 {
			if usd, ok := oanda.EstimateNotionalUSD(ev.Instrument, ev.Units, ev.ExitPrice); ok {
				fmt.Fprintf(&b, "Notional:    ~$%s\n", formatMoney(usd))
			}
		}
	}
	b.WriteString("\n")
	b.WriteString(FormatPL(ev.RealizedPL, ev.PLKnown))
	if ev.Reason != "" {
		fmt.Fprintf(&b, "\nReason:      %s\n", ev.Reason)
	}
	if ev.TradeID != "" {
		fmt.Fprintf(&b, "\nTrade ID:    %s\n", ev.TradeID)
	}
	if ev.CorrelationID != "" {
		fmt.Fprintf(&b, "Ref:         %s\n", ev.CorrelationID)
	}
	return b.String()
}

func FormatPL(pl float64, known bool) string {
	if !known {
		return "Result:      P/L unknown (OANDA transaction not found yet)\n"
	}
	label := "LOSS"
	if pl > 0 {
		label = "PROFIT"
	} else if pl == 0 {
		label = "BREAKEVEN"
	}
	sign := ""
	if pl > 0 {
		sign = "+"
	}
	return fmt.Sprintf("Result:      %s %s$%s\n", label, sign, formatMoney(math.Abs(pl)))
}

func TradeOpenSubject(instrument, direction string, units int64) string {
	dir := strings.ToUpper(strings.TrimSpace(direction))
	if dir == "" {
		dir = oanda.TradeDirection(units)
	}
	absUnits := units
	if absUnits < 0 {
		absUnits = -absUnits
	}
	return fmt.Sprintf("fxtrade: OPEN %s %s %s units", dir, instrument, formatUnits(absUnits))
}

func TradeCloseSubject(instrument string, pl float64, known bool, partial bool) string {
	if partial {
		return fmt.Sprintf("fxtrade: PARTIAL CLOSE %s", instrument)
	}
	if !known {
		return fmt.Sprintf("fxtrade: CLOSED %s (P/L unknown)", instrument)
	}
	label := "breakeven"
	if pl > 0 {
		label = fmt.Sprintf("+$%s", formatMoney(pl))
	} else if pl < 0 {
		label = fmt.Sprintf("-$%s", formatMoney(math.Abs(pl)))
	}
	return fmt.Sprintf("fxtrade: CLOSED %s (%s)", instrument, label)
}

func formatUnits(units int64) string {
	return fmt.Sprintf("%d", units)
}

func formatMoney(v float64) string {
	if math.Abs(v) >= 1000 {
		return fmt.Sprintf("%.2f", v)
	}
	return fmt.Sprintf("%.2f", v)
}
