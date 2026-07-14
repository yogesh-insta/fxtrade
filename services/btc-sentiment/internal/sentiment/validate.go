package sentiment

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

// ParseAndValidate unmarshals LLM output into SentimentResult and validates fields.
func ParseAndValidate(raw string) (SentimentResult, error) {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)

	var result SentimentResult
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return SentimentResult{}, fmt.Errorf("parse: %w", err)
	}
	if err := validate.Struct(result); err != nil {
		return SentimentResult{}, fmt.Errorf("validate: %w", err)
	}
	if result.KeyDrivers == nil {
		result.KeyDrivers = []string{}
	}
	return result, nil
}
