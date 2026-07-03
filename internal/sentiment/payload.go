package sentiment

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/ym/fxtrade/internal/market"
	"github.com/ym/fxtrade/internal/oanda"
)

type LLMPayload struct {
	Task            string              `json:"task"`
	AsOf            time.Time           `json:"as_of"`
	Instrument      string              `json:"instrument"`
	PriceContext    market.PriceContext `json:"price_context"`
	UpcomingEvents  []PayloadEvent      `json:"upcoming_events"`
	Headlines       []PayloadHeadline   `json:"headlines"`
	HighEventRisk   bool                `json:"high_event_risk"`
}

type PayloadEvent struct {
	Time    time.Time `json:"time"`
	Country string    `json:"country"`
	Event   string    `json:"event"`
	Impact  string    `json:"impact"`
}

type PayloadHeadline struct {
	Source  string    `json:"source"`
	Time    time.Time `json:"time"`
	Title   string    `json:"title"`
	Summary string    `json:"summary"`
}

func BuildPayload(instrument string, norm Normalized, price market.PriceContext, asOf time.Time) LLMPayload {
	if instrument == "" {
		instrument = oanda.DefaultInstrument
	}
	events := make([]PayloadEvent, 0, len(norm.Events))
	for _, e := range norm.Events {
		events = append(events, PayloadEvent{
			Time:    e.Time,
			Country: e.Country,
			Event:   e.Event,
			Impact:  e.Impact,
		})
	}

	headlines := make([]PayloadHeadline, 0, len(norm.Headlines))
	for _, h := range norm.Headlines {
		headlines = append(headlines, PayloadHeadline{
			Source:  h.Source,
			Time:    h.Published,
			Title:   h.Title,
			Summary: truncate(h.Summary, 500),
		})
	}

	return LLMPayload{
		Task:           strings.ToLower(instrument) + "_sentiment_analysis",
		AsOf:           asOf.UTC(),
		Instrument:     instrument,
		PriceContext:   price,
		UpcomingEvents: events,
		Headlines:      headlines,
		HighEventRisk:  norm.HighEventRisk,
	}
}

func (p LLMPayload) JSON() ([]byte, error) {
	return json.Marshal(p)
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
