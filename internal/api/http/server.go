package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/opendecisionserver/ods/internal/decision"
)

type Server struct {
	Service                            *decision.Service
	MaxRequestBytes                    int64
	MaxQuestions                       int
	ready                              atomic.Bool
	Logger                             *slog.Logger
	LogRequests                        bool
	requests, requestNS, requestErrors atomic.Int64
}

func (s *Server) SetReady(value bool) { s.ready.Store(value) }

func (s *Server) isReady() bool {
	if !s.ready.Load() || s.Service == nil {
		return false
	}
	if engine, ok := s.Service.Engine.(interface{ Healthy() bool }); ok {
		return engine.Healthy()
	}
	return true
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "live"}) })
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		if !s.isReady() {
			writeJSON(w, 503, map[string]string{"status": "loading"})
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, r *http.Request) {
		models := []map[string]string{}
		if s.isReady() {
			model := s.Service.ModelInfo()
			models = model.Listings()
		}
		writeJSON(w, 200, map[string]any{"models": models})
	})
	mux.HandleFunc("GET /metrics", s.metrics)
	mux.HandleFunc("POST /v1/systemone", s.systemOne)
	return mux
}

func (s *Server) metrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	var active, waiting, runs, maxRuns int64
	if s.isReady() {
		if s.Service.Admission != nil {
			active = s.Service.Admission.Active()
			waiting = s.Service.Admission.Waiting()
		}
		if engine, ok := s.Service.Engine.(interface {
			RunCount() int64
			ActiveRuns() int64
			MaxActiveRuns() int64
		}); ok {
			active = engine.ActiveRuns()
			runs = engine.RunCount()
			maxRuns = engine.MaxActiveRuns()
		}
		if engine, ok := s.Service.Engine.(interface{ Waiting() int }); ok {
			waiting = int64(engine.Waiting())
		}
	}
	fmt.Fprintf(w, "ods_active_runs %d\nods_waiting_requests %d\nods_inference_runs_total %d\nods_max_concurrent_run_calls %d\n", active, waiting, runs, maxRuns)
	fmt.Fprintf(w, "ods_http_requests_total %d\nods_http_errors_total %d\nods_http_duration_seconds_total %.9f\n", s.requests.Load(), s.requestErrors.Load(), float64(s.requestNS.Load())/1e9)
	fmt.Fprint(w, processMetrics())
	if s.isReady() {
		if engine, ok := s.Service.Engine.(interface{ Metrics() string }); ok {
			fmt.Fprint(w, engine.Metrics())
		}
	}
}

func (s *Server) systemOne(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	status := 200
	questions := 0
	defer func() {
		s.requests.Add(1)
		s.requestNS.Add(time.Since(started).Nanoseconds())
		if status >= 400 {
			s.requestErrors.Add(1)
		}
		if s.Logger != nil && (s.LogRequests || status >= 500) {
			s.Logger.Info("request", "status", status, "questions", questions, "duration_ms", time.Since(started).Milliseconds())
		}
	}()
	if !s.isReady() {
		status = 503
		writeJSON(w, status, map[string]any{"detail": "model unavailable"})
		return
	}
	limit := s.MaxRequestBytes
	if limit <= 0 {
		limit = 1 << 20
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		status = 400
		writeJSON(w, status, map[string]any{"detail": "malformed request body"})
		return
	}
	if int64(len(body)) > limit {
		status = 413
		writeJSON(w, status, map[string]any{"detail": "request too large"})
		return
	}
	if !json.Valid(body) {
		status = 400
		writeJSON(w, status, map[string]any{"detail": "malformed JSON"})
		return
	}
	req, err := decision.ParseForModel(body, s.MaxQuestions, s.Service.ModelInfo())
	if err != nil {
		status = 422
		writeJSON(w, status, validation(err))
		return
	}
	questions = len(req.Questions)
	response, err := s.Service.Decide(r.Context(), req)
	if err != nil {
		var field decision.FieldError
		switch {
		case errors.As(err, &field):
			status = 422
			writeJSON(w, status, validation(err))
		case errors.Is(err, decision.ErrUnknownModel):
			status = 404
			writeJSON(w, status, map[string]any{"detail": "model not found"})
		case errors.Is(err, decision.ErrOverloaded):
			status = 429
			writeJSON(w, status, map[string]any{"detail": "server overloaded"})
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			status = 503
			writeJSON(w, status, map[string]any{"detail": "request canceled"})
		case errors.Is(err, decision.ErrUnavailable):
			status = 503
			writeJSON(w, status, map[string]any{"detail": "model unavailable"})
		default:
			status = 500
			if s.Logger != nil {
				s.Logger.Error("decision failed", "error", err)
			}
			writeJSON(w, status, map[string]any{"detail": "internal error"})
		}
		return
	}
	writeJSON(w, 200, response)
}

func validation(err error) any {
	var field decision.FieldError
	if errors.As(err, &field) {
		return map[string]any{"detail": []decision.FieldError{field}}
	}
	return map[string]any{"detail": []decision.FieldError{{Location: []any{"body"}, Message: err.Error(), Type: "value_error"}}}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
