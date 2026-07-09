package oanda

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTransactionsSinceAllFollowsPages(t *testing.T) {
	var baseURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v3/accounts/test/transactions":
			w.Header().Set("Content-Type", "application/json")
			payload := map[string]any{
				"transactions": []map[string]any{
					{"id": "1", "type": "ORDER_FILL", "tradesClosed": []map[string]string{
						{"tradeID": "42", "realizedPL": "1.50"},
					}},
				},
				"pages": []string{baseURL + "/page2"},
			}
			_ = json.NewEncoder(w).Encode(payload)
		case "/page2":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"transactions":[{"id":"3","type":"ORDER_FILL","tradesClosed":[{"tradeID":"99","realizedPL":"5.00"}]}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	baseURL = srv.URL

	client := NewClient(srv.URL, "test", "token")
	txs, err := client.TransactionsSinceAll(context.Background(), time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(txs) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(txs))
	}
	pl, ok := ClosedTradePL(txs, "42")
	if !ok || pl != 1.5 {
		t.Fatalf("trade 42 pl=%v ok=%v", pl, ok)
	}
	pl, ok = ClosedTradePL(txs, "99")
	if !ok || pl != 5.0 {
		t.Fatalf("trade 99 pl=%v ok=%v", pl, ok)
	}
}
