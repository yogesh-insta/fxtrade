package sentiment_test

import (
	"testing"

	"github.com/ym/fxtrade/services/btc-sentiment/internal/sentiment"
)

func TestParseAndValidate(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "valid", raw: `{"sentiment_score":0.2,"confidence":0.7,"low_confidence":false,"key_drivers":["a"],"divergence_note":null}`},
		{name: "score out of range", raw: `{"sentiment_score":2,"confidence":0.7,"low_confidence":false,"key_drivers":[]}`, wantErr: true},
		{name: "not json", raw: `nope`, wantErr: true},
		{name: "zero ok", raw: `{"sentiment_score":0,"confidence":0,"low_confidence":true,"key_drivers":[]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := sentiment.ParseAndValidate(tt.raw)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatal(err)
			}
		})
	}
}
