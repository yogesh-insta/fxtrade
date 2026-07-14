package sentiment

// SentimentResult matches the LLM JSON schema.
type SentimentResult struct {
	SentimentScore float64  `json:"sentiment_score" validate:"gte=-1,lte=1"`
	Confidence     float64  `json:"confidence" validate:"gte=0,lte=1"`
	LowConfidence  bool     `json:"low_confidence"`
	KeyDrivers     []string `json:"key_drivers" validate:"omitempty,max=3,dive,min=1"`
	DivergenceNote *string  `json:"divergence_note"`
}

// NeutralFallback returns the default result when scoring fails or data is missing.
func NeutralFallback(note string) SentimentResult {
	n := note
	return SentimentResult{
		SentimentScore: 0,
		Confidence:     0,
		LowConfidence:  true,
		KeyDrivers:     nil,
		DivergenceNote: &n,
	}
}
