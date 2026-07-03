package oanda

import "time"

const DefaultInstrument = "AUD_USD"

type AccountSummary struct {
	Account struct {
		ID      string `json:"id"`
		Balance string `json:"balance"`
		NAV     string `json:"NAV"`
	} `json:"account"`
}

type PricingResponse struct {
	Time   string        `json:"time"`
	Prices []ClientPrice `json:"prices"`
}

type ClientPrice struct {
	Type       string      `json:"type"`
	Time       string      `json:"time"`
	Instrument string      `json:"instrument"`
	Bids       []PriceTick `json:"bids"`
	Asks       []PriceTick `json:"asks"`
	Status     string      `json:"status"`
	Tradeable  bool        `json:"tradeable"`
}

type PriceTick struct {
	Price     string `json:"price"`
	Liquidity int64  `json:"liquidity"`
}

type CandlesResponse struct {
	Instrument  string   `json:"instrument"`
	Granularity string   `json:"granularity"`
	Candles     []Candle `json:"candles"`
}

type Candle struct {
	Complete bool      `json:"complete"`
	Time     string    `json:"time"`
	Volume   int       `json:"volume"`
	Mid      OHLCPrice `json:"mid"`
}

type OHLCPrice struct {
	O string `json:"o"`
	H string `json:"h"`
	L string `json:"l"`
	C string `json:"c"`
}

type PriceUpdate struct {
	Instrument string
	Bid        float64
	Ask        float64
	Spread     float64
	Time       time.Time
	Tradeable  bool
}

const (
	OrderTypeMarket     = "MARKET"
	OrderTypeLimit      = "LIMIT"
	TimeInForceFOK      = "FOK"
	TimeInForceGTC      = "GTC"
	PositionFillDefault = "DEFAULT"
)

type CreateOrderRequest struct {
	Order OrderSpec `json:"order"`
}

type OrderSpec struct {
	Type             string            `json:"type"`
	Instrument       string            `json:"instrument"`
	Units            string            `json:"units"`
	Price            string            `json:"price,omitempty"`
	TimeInForce      string            `json:"timeInForce"`
	PositionFill     string            `json:"positionFill"`
	StopLossOnFill   *OnFillStopLoss   `json:"stopLossOnFill,omitempty"`
	TakeProfitOnFill *OnFillTakeProfit `json:"takeProfitOnFill,omitempty"`
}

type OnFillStopLoss struct {
	Price       string `json:"price"`
	TimeInForce string `json:"timeInForce"`
}

type OnFillTakeProfit struct {
	Price       string `json:"price"`
	TimeInForce string `json:"timeInForce"`
}

type CreateOrderResponse struct {
	OrderCreateTransaction *Transaction `json:"orderCreateTransaction"`
	OrderFillTransaction   *Transaction `json:"orderFillTransaction"`
	OrderCancelTransaction *Transaction `json:"orderCancelTransaction"`
	RelatedTransactionIDs  []string     `json:"relatedTransactionIDs"`
	LastTransactionID      string       `json:"lastTransactionID"`
}

type Transaction struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Instrument  string `json:"instrument"`
	Units       string `json:"units"`
	Price       string `json:"price"`
	Pl          string `json:"pl"`
	TradeOpened *struct {
		TradeID string `json:"tradeID"`
		Units   string `json:"units"`
		Price   string `json:"price"`
	} `json:"tradeOpened"`
	TradeReduced *struct {
		TradeID string `json:"tradeID"`
		Units   string `json:"units"`
	} `json:"tradeReduced"`
	TradesClosed []struct {
		TradeID string `json:"tradeID"`
		Units   string `json:"units"`
		RealizedPL string `json:"realizedPL"`
	} `json:"tradesClosed"`
}

type OpenTradesResponse struct {
	Trades []Trade `json:"trades"`
}

type Trade struct {
	ID            string `json:"id"`
	Instrument    string `json:"instrument"`
	CurrentUnits  string `json:"currentUnits"`
	Price         string `json:"price"`
	UnrealizedPL  string `json:"unrealizedPL"`
	OpenTime      string `json:"openTime"`
	StopLossOrder *struct {
		ID    string `json:"id"`
		Price string `json:"price"`
	} `json:"stopLossOrder"`
	TakeProfitOrder *struct {
		ID    string `json:"id"`
		Price string `json:"price"`
	} `json:"takeProfitOrder"`
}

type CloseTradeRequest struct {
	Units string `json:"units"`
}

type CloseTradeResponse struct {
	OrderCreateTransaction *Transaction `json:"orderCreateTransaction"`
	OrderFillTransaction   *Transaction `json:"orderFillTransaction"`
	LastTransactionID      string       `json:"lastTransactionID"`
}

type OpenPositionsResponse struct {
	Positions []Position `json:"positions"`
}

type Position struct {
	Instrument string `json:"instrument"`
	Long       *struct {
		Units string `json:"units"`
	} `json:"long"`
	Short *struct {
		Units string `json:"units"`
	} `json:"short"`
}

type OrderResult struct {
	TransactionID string
	TradeID       string
	OrderID       string
	Instrument    string
	Units         int64
	FillPrice     float64
}

type PendingOrdersResponse struct {
	Orders []PendingOrder `json:"orders"`
}

type PendingOrder struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Instrument string `json:"instrument"`
	Units      string `json:"units"`
	Price      string `json:"price"`
	State      string `json:"state"`
	CreateTime string `json:"createTime"`
}

type CancelOrderResponse struct {
	OrderCancelTransaction *Transaction `json:"orderCancelTransaction"`
	LastTransactionID      string       `json:"lastTransactionID"`
}

type TransactionsResponse struct {
	Transactions []Transaction `json:"transactions"`
}
