package execution

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/ym/fxtrade/internal/notify"
	"github.com/ym/fxtrade/internal/oanda"
	"github.com/ym/fxtrade/internal/risk"
)

// TradeAccounting records opens/closes idempotently (typically the position monitor).
type TradeAccounting interface {
	NoteTradeOpened(tradeID string)
	NoteTradeClosed(tradeID string, pl float64)
	RecordPartialPL(pl float64)
}

// TradeSnapshotSource optionally supplies open-trade context for close emails.
type TradeSnapshotSource interface {
	TradeSnapshot(tradeID string) (oanda.Trade, bool)
}

type Executor struct {
	client *oanda.Client
	risk   *risk.Manager
	notify notify.Notifier
	trades TradeAccounting
	dryRun bool
}

func NewExecutor(client *oanda.Client, rm *risk.Manager, n notify.Notifier) *Executor {
	return &Executor{client: client, risk: rm, notify: n}
}

func (e *Executor) SetDryRun(v bool) {
	e.dryRun = v
}

func (e *Executor) DryRun() bool {
	return e.dryRun
}

func (e *Executor) SetTradeAccounting(t TradeAccounting) {
	e.trades = t
}

func (e *Executor) PlaceMarket(ctx context.Context, req risk.EntryRequest, params MarketOrderParams) (oanda.OrderResult, error) {
	if err := e.risk.AllowEntry(ctx, req); err != nil {
		return oanda.OrderResult{}, err
	}

	order, err := BuildMarketOrder(params)
	if err != nil {
		return oanda.OrderResult{}, err
	}

	if e.dryRun {
		unitsLabel := strconv.FormatInt(params.Units, 10)
		if params.UnitsStr != "" {
			unitsLabel = params.UnitsStr
		}
		msg := fmt.Sprintf("DRY RUN: would place MARKET %s %s %s units SL=%s",
			params.Direction, params.Instrument, unitsLabel, oanda.FormatPrice(params.StopLoss))
		if params.TakeProfit != nil {
			msg += fmt.Sprintf(" TP=%s", oanda.FormatPrice(*params.TakeProfit))
		}
		slog.Info(msg, "correlation_id", req.CorrelationID)
		return oanda.OrderResult{
			TransactionID: "dry-run",
			TradeID:       "dry-run-" + req.CorrelationID,
			Instrument:    params.Instrument,
			Units:         params.Units,
			UnitsStr:      params.UnitsStr,
			FillPrice:     params.StopLoss,
		}, nil
	}

	resp, err := e.client.CreateOrder(ctx, order)
	if err != nil {
		if params.ClientOrderID != "" {
			if existing, found, lookupErr := e.client.FindOrderByClientID(ctx, params.ClientOrderID); lookupErr == nil && found {
				slog.Info("order recovered via idempotency key",
					"client_order_id", params.ClientOrderID,
					"trade_id", existing.TradeID,
				)
				if existing.TradeID != "" {
					e.noteOpenedFill(existing, params.Instrument, params.Direction)
				}
				return existing, nil
			}
		}
		notify.SendRoutine(e.notify, ctx, "fxtrade: order rejected", fmt.Sprintf("correlation_id=%s\nerror=%v\n", req.CorrelationID, err))
		return oanda.OrderResult{}, err
	}

	result, err := oanda.ParseOrderResult(resp)
	if err != nil {
		notify.SendRoutine(e.notify, ctx, "fxtrade: order rejected", fmt.Sprintf("correlation_id=%s\nerror=%v\n", req.CorrelationID, err))
		return oanda.OrderResult{}, err
	}

	e.noteOpenedFill(result, params.Instrument, params.Direction)
	open := notify.TradeOpen{
		Instrument:    params.Instrument,
		Direction:     params.Direction,
		Units:         result.Units,
		FillPrice:     result.FillPrice,
		StopLoss:      &params.StopLoss,
		TakeProfit:    params.TakeProfit,
		TradeID:       result.TradeID,
		CorrelationID: req.CorrelationID,
	}
	e.notify.Send(ctx, notify.TradeOpenSubject(params.Instrument, params.Direction, result.Units), notify.FormatTradeOpen(open))
	slog.Info("order filled",
		"correlation_id", req.CorrelationID,
		"trade_id", result.TradeID,
		"direction", params.Direction,
		"units", result.Units,
		"price", result.FillPrice,
	)

	return result, nil
}

func (e *Executor) PlaceLimit(ctx context.Context, req risk.EntryRequest, params LimitOrderParams) (oanda.OrderResult, error) {
	if err := e.risk.AllowEntry(ctx, req); err != nil {
		return oanda.OrderResult{}, err
	}

	order, err := BuildLimitOrder(params)
	if err != nil {
		return oanda.OrderResult{}, err
	}

	if e.dryRun {
		slog.Info("DRY RUN: would place LIMIT order",
			"correlation_id", req.CorrelationID,
			"direction", params.Direction,
			"instrument", params.Instrument,
			"units", params.Units,
			"price", params.Price,
			"stop_loss", params.StopLoss,
		)
		return oanda.OrderResult{
			TransactionID: "dry-run",
			OrderID:       "dry-run-" + req.CorrelationID,
			Instrument:    params.Instrument,
			Units:         params.Units,
			FillPrice:     params.Price,
		}, nil
	}

	subject := fmt.Sprintf("fxtrade: limit submitted %s %s @ %s", params.Direction, params.Instrument, oanda.FormatPrice(params.Price))
	body := fmt.Sprintf("correlation_id=%s\ndirection=%s\nunits=%d\nprice=%s\nstop_loss=%s\n",
		req.CorrelationID, params.Direction, params.Units, oanda.FormatPrice(params.Price), oanda.FormatPrice(params.StopLoss))
	e.notify.Send(ctx, subject, body)

	resp, err := e.client.CreateOrder(ctx, order)
	if err != nil {
		notify.SendRoutine(e.notify, ctx, "fxtrade: limit rejected", fmt.Sprintf("correlation_id=%s\nerror=%v\n", req.CorrelationID, err))
		return oanda.OrderResult{}, err
	}

	result, err := oanda.ParseCreateOrderResult(resp)
	if err != nil {
		notify.SendRoutine(e.notify, ctx, "fxtrade: limit rejected", fmt.Sprintf("correlation_id=%s\nerror=%v\n", req.CorrelationID, err))
		return oanda.OrderResult{}, err
	}

	if result.TradeID != "" {
		e.noteOpenedFill(result, params.Instrument, params.Direction)
		open := notify.TradeOpen{
			Instrument:    params.Instrument,
			Direction:     params.Direction,
			Units:         result.Units,
			FillPrice:     result.FillPrice,
			StopLoss:      &params.StopLoss,
			TakeProfit:    params.TakeProfit,
			TradeID:       result.TradeID,
			CorrelationID: req.CorrelationID,
		}
		e.notify.Send(ctx, notify.TradeOpenSubject(params.Instrument, params.Direction, result.Units), notify.FormatTradeOpen(open))
	}
	slog.Info("limit order placed",
		"correlation_id", req.CorrelationID,
		"order_id", result.OrderID,
		"trade_id", result.TradeID,
		"direction", params.Direction,
		"price", params.Price,
	)
	return result, nil
}

func (e *Executor) CancelOrder(ctx context.Context, orderID, correlationID string) error {
	if e.dryRun {
		slog.Info("DRY RUN: would cancel order", "correlation_id", correlationID, "order_id", orderID)
		return nil
	}
	if _, err := e.client.CancelOrder(ctx, orderID); err != nil {
		return err
	}
	notify.SendRoutine(e.notify, ctx, "fxtrade: order cancelled",
		fmt.Sprintf("correlation_id=%s\norder_id=%s\n", correlationID, orderID))
	slog.Info("order cancelled", "correlation_id", correlationID, "order_id", orderID)
	return nil
}

func (e *Executor) CloseTrade(ctx context.Context, tradeID, correlationID string) (float64, error) {
	return e.CloseTradeUnits(ctx, tradeID, correlationID, "ALL")
}

func (e *Executor) CloseTradeUnits(ctx context.Context, tradeID, correlationID, units string) (float64, error) {
	if e.dryRun {
		slog.Info("DRY RUN: would close trade", "correlation_id", correlationID, "trade_id", tradeID, "units", units)
		return 0, nil
	}
	resp, err := e.client.CloseTrade(ctx, tradeID, units)
	if err != nil {
		return 0, err
	}

	details := oanda.CloseFillDetails(resp)
	pl := details.RealizedPL
	plKnown := details.PLKnown
	if !plKnown {
		slog.Warn("close trade missing P&L", "trade_id", tradeID)
	}

	closeEv := e.buildCloseEvent(tradeID, correlationID, units, details, pl, plKnown)
	if units == "ALL" {
		e.noteClosed(tradeID, pl)
	} else {
		e.notePartialPL(pl)
	}
	e.notify.Send(ctx,
		notify.TradeCloseSubject(closeEv.Instrument, pl, plKnown, closeEv.Partial),
		notify.FormatTradeClose(closeEv))
	slog.Info("position closed", "correlation_id", correlationID, "trade_id", tradeID, "units", units, "pl", pl)

	return pl, nil
}

func (e *Executor) noteOpened(tradeID string) {
	if e.trades != nil {
		e.trades.NoteTradeOpened(tradeID)
	} else {
		e.risk.RecordTradeOpened()
	}
}

// openTradeSeeder is optionally implemented by TradeAccounting (position monitor)
// to seed known state so SL/TP hits before the next poll are still recorded.
type openTradeSeeder interface {
	SeedOpenedTrade(t oanda.Trade)
}

func (e *Executor) noteOpenedFill(result oanda.OrderResult, instrument, direction string) {
	if result.TradeID == "" {
		return
	}
	units := result.UnitsStr
	if units == "" && result.Units != 0 {
		units = strconv.FormatInt(result.Units, 10)
	}
	// Prefer signed units matching OANDA openTrades (SHORT negative).
	if direction == "SHORT" && units != "" && !strings.HasPrefix(units, "-") {
		units = "-" + units
	}
	t := oanda.Trade{
		ID:           result.TradeID,
		Instrument:   instrument,
		CurrentUnits: units,
		Price:        oanda.FormatPrice(result.FillPrice),
	}
	if s, ok := e.trades.(openTradeSeeder); ok {
		s.SeedOpenedTrade(t)
		return
	}
	e.noteOpened(result.TradeID)
}

func (e *Executor) noteClosed(tradeID string, pl float64) {
	if e.trades != nil {
		e.trades.NoteTradeClosed(tradeID, pl)
	} else {
		e.risk.RecordTradeClosed(pl)
	}
}

func (e *Executor) notePartialPL(pl float64) {
	if e.trades != nil {
		e.trades.RecordPartialPL(pl)
	} else if pl != 0 {
		e.risk.RecordTradeClosed(pl)
	}
}

func (e *Executor) buildCloseEvent(tradeID, correlationID, units string, details oanda.CloseDetails, pl float64, plKnown bool) notify.TradeClose {
	ev := notify.TradeClose{
		Instrument:    details.Instrument,
		Units:         details.UnitsClosed,
		ExitPrice:     details.ExitPrice,
		RealizedPL:    pl,
		PLKnown:       plKnown,
		TradeID:       tradeID,
		CorrelationID: correlationID,
		Partial:       units != "ALL",
	}
	if ev.Units < 0 {
		ev.Units = -ev.Units
	}
	if snap, ok := e.tradeSnapshot(tradeID); ok {
		if ev.Instrument == "" {
			ev.Instrument = snap.Instrument
		}
		if entry, err := oanda.ParsePrice(snap.Price); err == nil {
			ev.EntryPrice = entry
		}
		if u, err := parseUnits(snap.CurrentUnits); err == nil {
			ev.Direction = oanda.TradeDirection(u)
			if ev.Units == 0 {
				ev.Units = u
				if ev.Units < 0 {
					ev.Units = -ev.Units
				}
			}
		}
	}
	if ev.Reason == "" {
		ev.Reason = closeReason(correlationID, ev.Partial)
	}
	if strings.Contains(correlationID, "tp1-") {
		ev.Reason = "TP1 partial close at range midpoint"
	}
	return ev
}

func (e *Executor) tradeSnapshot(tradeID string) (oanda.Trade, bool) {
	if e.trades == nil {
		return oanda.Trade{}, false
	}
	src, ok := e.trades.(TradeSnapshotSource)
	if !ok {
		return oanda.Trade{}, false
	}
	return src.TradeSnapshot(tradeID)
}

func closeReason(correlationID string, partial bool) string {
	if partial {
		return "partial close"
	}
	if correlationID == "" {
		return "position closed"
	}
	if strings.Contains(correlationID, "force_flat") {
		return "force flat (end of session)"
	}
	return correlationID
}

func parseUnits(s string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
}
