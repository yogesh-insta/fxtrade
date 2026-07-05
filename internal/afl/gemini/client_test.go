package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGenerateGroundedSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1beta/models/gemini-2.5-flash-lite:generateContent" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("x-goog-api-key"); got != "test-key" {
			t.Fatalf("api key header = %q", got)
		}
		var req generateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.SystemInstruction == nil || !strings.Contains(req.SystemInstruction.Parts[0].Text, "AFL") {
			t.Fatalf("missing system instruction: %+v", req.SystemInstruction)
		}
		if len(req.Tools) != 1 {
			t.Fatalf("tools = %d, want google_search", len(req.Tools))
		}
		_ = json.NewEncoder(w).Encode(generateResponse{
			Candidates: []candidate{{
				Content: content{Parts: []part{{Text: "Subject Line: [AFL Round 17 Analytics] - Essendon vs St Kilda"}}},
			}},
		})
	}))
	defer srv.Close()

	client := NewClient("test-key", DefaultModel).WithBaseURL(srv.URL + "/v1beta")
	text, err := client.GenerateGrounded(context.Background(), "AFL system", "Analyze Essendon vs St Kilda")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Essendon vs St Kilda") {
		t.Fatalf("text = %q", text)
	}
}

func TestGenerateGroundedMultiPartJSON(t *testing.T) {
	jsonOut := `{"main_bet":{"selection":"PORT H2H","market":"H2H","confidence":"high","reasons":["a","b","c"]}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(generateResponse{
			Candidates: []candidate{{
				Content: content{Parts: []part{
					{Text: "thinking about search results", Thought: true},
					{Text: jsonOut},
				}},
			}},
		})
	}))
	defer srv.Close()

	client := NewClient("test-key", DefaultModel).WithBaseURL(srv.URL + "/v1beta")
	text, err := client.GenerateGrounded(context.Background(), "sys", "user")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "PORT H2H") {
		t.Fatalf("expected JSON part, got %q", text)
	}
}

func TestGenerateGroundedAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	}))
	defer srv.Close()

	client := NewClient("bad", DefaultModel).WithBaseURL(srv.URL + "/v1beta")
	_, err := client.GenerateGrounded(context.Background(), "sys", "user")
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("expected status error, got %v", err)
	}
}
