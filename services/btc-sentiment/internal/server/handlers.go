package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/ym/fxtrade/services/btc-sentiment/internal/agent"
)

// Handler serves Cloud Run HTTP endpoints.
type Handler struct {
	Agent *agent.Agent
	Log   *slog.Logger
}

type runRequest struct {
	Now string `json:"now"`
}

// Register mounts routes on mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", h.healthz)
	mux.HandleFunc("POST /run-sentiment-pass", h.runSentimentPass)
}

func (h *Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (h *Handler) runSentimentPass(w http.ResponseWriter, r *http.Request) {
	log := h.Log
	if log == nil {
		log = slog.Default()
	}

	now := time.Now().UTC()
	if r.Body != nil && r.ContentLength != 0 {
		var req runRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		if req.Now != "" {
			t, err := time.Parse(time.RFC3339, req.Now)
			if err != nil {
				http.Error(w, "now must be RFC3339", http.StatusBadRequest)
				return
			}
			now = t.UTC()
		}
	}

	result, err := h.Agent.Run(r.Context(), now)
	if err != nil {
		log.Error("agent run failed", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(result); err != nil {
		log.Error("encode response failed", "err", err)
	}
}
