package strategy

const (
	ModeRange      = "RANGE"
	ModeTrend      = "TREND"
	ModeStandAside = "STAND_ASIDE"
)

type RangeBand struct {
	Valid              bool
	High               float64
	Low                float64
	Mid                float64
	WidthPips          float64
	TouchesHigh        int
	TouchesLow         int
	SupportZoneLow     float64
	SupportZoneHigh    float64
	ResistanceZoneLow  float64
	ResistanceZoneHigh float64
	BuyLimitPrice      float64
	SellLimitPrice     float64
}

type Decision struct {
	Mode   string
	Action string
	Reason string
	Details map[string]any
}

type TradeIntent struct {
	Direction    string
	OrderType    string // MARKET or LIMIT
	Units        int64
	EntryPrice   float64
	StopLoss     float64
	TakeProfit   *float64
	Confidence   float64
	CorrelationID string
}
