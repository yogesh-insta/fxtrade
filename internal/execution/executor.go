package execution

import (
	"context"
	"fmt"
	"log/slog"

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

type Executor struct {
	client   *oanda.Client
	risk     *risk.Manager
	notify   notify.Notifier
	trades   TradeAccounting
}

func NewExecutor(client *oanda.Client, rm *risk.Manager, n notify.Notifier) *Executor {
	return &Executor{client: client, risk: rm, notify: n}
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

	subject := fmt.Sprintf("fxtrade: order submitted %s %s %d units", params.Direction, params.Instrument, params.Units)
	body := fmt.Sprintf("correlation_id=%s\ndirection=%s\nunits=%d\nstop_loss=%s\n",
		req.CorrelationID, params.Direction, params.Units, oanda.FormatPrice(params.StopLoss))
	if params.TakeProfit != nil {
		body += fmt.Sprintf("take_profit=%s\n", oanda.FormatPrice(*params.TakeProfit))
	}
	e.notify.Send(ctx, subject, body)

	resp, err := e.client.CreateOrder(ctx, order)
	if err != nil {
		e.notify.Send(ctx, "fxtrade: order rejected", fmt.Sprintf("correlation_id=%s\nerror=%v\n", req.CorrelationID, err))
		return oanda.OrderResult{}, err
	}

	result, err := oanda.ParseOrderResult(resp)
	if err != nil {
		e.notify.Send(ctx, "fxtrade: order rejected", fmt.Sprintf("correlation_id=%s\nerror=%v\n", req.CorrelationID, err))
		return oanda.OrderResult{}, err
	}

	e.noteOpened(result.TradeID)
	e.notify.Send(ctx, "fxtrade: order filled",
		fmt.Sprintf("correlation_id=%s\ntrade_id=%s\nfill_price=%s\nunits=%d\n",
			req.CorrelationID, result.TradeID, oanda.FormatPrice(result.FillPrice), result.Units))
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

	subject := fmt.Sprintf("fxtrade: limit submitted %s %s @ %s", params.Direction, params.Instrument, oanda.FormatPrice(params.Price))
	body := fmt.Sprintf("correlation_id=%s\ndirection=%s\nunits=%d\nprice=%s\nstop_loss=%s\n",
		req.CorrelationID, params.Direction, params.Units, oanda.FormatPrice(params.Price), oanda.FormatPrice(params.StopLoss))
	e.notify.Send(ctx, subject, body)

	resp, err := e.client.CreateOrder(ctx, order)
	if err != nil {
		e.notify.Send(ctx, "fxtrade: limit rejected", fmt.Sprintf("correlation_id=%s\nerror=%v\n", req.CorrelationID, err))
		return oanda.OrderResult{}, err
	}

	result, err := oanda.ParseCreateOrderResult(resp)
	if err != nil {
		e.notify.Send(ctx, "fxtrade: limit rejected", fmt.Sprintf("correlation_id=%s\nerror=%v\n", req.CorrelationID, err))
		return oanda.OrderResult{}, err
	}

	if result.TradeID != "" {
		e.noteOpened(result.TradeID)
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
	if _, err := e.client.CancelOrder(ctx, orderID); err != nil {
		return err
	}
	e.notify.Send(ctx, "fxtrade: order cancelled",
		fmt.Sprintf("correlation_id=%s\norder_id=%s\n", correlationID, orderID))
	slog.Info("order cancelled", "correlation_id", correlationID, "order_id", orderID)
	return nil
}

func (e *Executor) CloseTrade(ctx context.Context, tradeID, correlationID string) (float64, error) {
	return e.CloseTradeUnits(ctx, tradeID, correlationID, "ALL")
}

func (e *Executor) CloseTradeUnits(ctx context.Context, tradeID, correlationID, units string) (float64, error) {
	resp, err := e.client.CloseTrade(ctx, tradeID, units)
	if err != nil {
		return 0, err
	}

	pl, err := oanda.RealizedPL(resp)
	if err != nil {
		slog.Warn("close trade missing P&L", "trade_id", tradeID, "error", err)
	}

	if units == "ALL" {
		e.noteClosed(tradeID, pl)
	} else {
		e.notePartialPL(pl)
	}
	e.notify.Send(ctx, "fxtrade: position closed",
		fmt.Sprintf("correlation_id=%s\ntrade_id=%s\nunits=%s\nrealized_pl=%.2f\n", correlationID, tradeID, units, pl))
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
