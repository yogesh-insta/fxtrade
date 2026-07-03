package sentiment

import "time"

type Headline struct {
	Source    string    `json:"source"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary"`
	URL       string    `json:"url"`
	Published time.Time `json:"published"`
}

type CalendarEvent struct {
	Time    time.Time `json:"time"`
	Country string    `json:"country"`
	Event   string    `json:"event"`
	Impact  string    `json:"impact"`
}

type SentimentSignal struct {
	Instrument   string    `json:"instrument,omitempty"`
	Direction    string    `json:"direction"`
	Confidence   float64   `json:"confidence"`
	BaseBias     string    `json:"base_bias"`
	Drivers      []string  `json:"drivers"`
	Risks        []string  `json:"risks"`
	EventRisk    string    `json:"event_risk"`
	HoldReason   *string   `json:"hold_reason"`
	ValidMinutes int       `json:"valid_minutes"`
	AnalyzedAt   time.Time `json:"analyzed_at"`
}

func (s SentimentSignal) ValidUntil() time.Time {
	if s.ValidMinutes <= 0 {
		return s.AnalyzedAt.Add(30 * time.Minute)
	}
	return s.AnalyzedAt.Add(time.Duration(s.ValidMinutes) * time.Minute)
}

func (s SentimentSignal) IsValid(now time.Time) bool {
	return now.Before(s.ValidUntil())
}

func Empty() SentimentSignal {
	return SentimentSignal{Direction: "FLAT", EventRisk: "low", Confidence: 0.5}
}

type RawData struct {
	Headlines []Headline
	Events    []CalendarEvent
	FetchedAt time.Time
}

type AuditRecord struct {
	At         time.Time       `json:"at"`
	Instrument string          `json:"instrument,omitempty"`
	Payload    any             `json:"payload"`
	Response   SentimentSignal `json:"response"`
	Error      string          `json:"error,omitempty"`
}
