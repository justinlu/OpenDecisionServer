package decision

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
)

var ErrOverloaded = errors.New("decision server overloaded")
var ErrUnknownModel = errors.New("model not found")
var ErrUnavailable = errors.New("decision engine unavailable")

type Admission struct {
	slots      chan struct{}
	maxWaiting int64
	waiting    atomic.Int64
	active     atomic.Int64
}

func NewAdmission(maxRuns, maxWaiting int) *Admission {
	return &Admission{slots: make(chan struct{}, maxRuns), maxWaiting: int64(maxWaiting)}
}

func (a *Admission) Acquire(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case a.slots <- struct{}{}:
		a.active.Add(1)
		return a.release, nil
	default:
	}
	for {
		n := a.waiting.Load()
		if n >= a.maxWaiting {
			return nil, ErrOverloaded
		}
		if a.waiting.CompareAndSwap(n, n+1) {
			break
		}
	}
	defer a.waiting.Add(-1)
	select {
	case a.slots <- struct{}{}:
		if err := ctx.Err(); err != nil {
			<-a.slots
			return nil, err
		}
		a.active.Add(1)
		return a.release, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (a *Admission) release()       { <-a.slots; a.active.Add(-1) }
func (a *Admission) Active() int64  { return a.active.Load() }
func (a *Admission) Waiting() int64 { return a.waiting.Load() }

type Service struct {
	Model     Model
	Engine    Engine
	Admission *Admission
}

type Response struct {
	Model   string         `json:"model"`
	Answers map[string]any `json:"answers"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (s *Service) Decide(ctx context.Context, req Request) (Response, error) {
	var response Response
	model := s.ModelInfo()
	if !model.Accepts(req.Model) {
		return response, ErrUnknownModel
	}
	if s.Admission != nil {
		release, err := s.Admission.Acquire(ctx)
		if err != nil {
			return response, err
		}
		defer release()
	}
	results, err := s.Engine.Decide(ctx, req.Questions)
	if err != nil {
		return response, err
	}
	if len(results) != len(req.Questions) {
		return response, fmt.Errorf("engine result count mismatch")
	}
	response.Model = model.ID
	response.Answers = make(map[string]any, len(results))
	for i, row := range req.Questions {
		result := results[i]
		if len(result.Probabilities) != len(row.Options) || result.Winner < 0 || result.Winner >= len(row.Options) {
			return Response{}, fmt.Errorf("engine option count mismatch")
		}
		response.Usage.InputTokens += result.InputTokens
		switch row.Type {
		case Noul:
			response.Answers[row.ID] = map[string]any{"type": "noul", "noul": result.Probabilities[1]}
			if model.Chinese {
				response.Answers[row.ID] = map[string]any{"type": "noul", "noul": result.Probabilities[1], "probabilities": map[string]float64{"false": result.Probabilities[0], "true": result.Probabilities[1]}, "confidence": confidence(result.Probabilities)}
			}
		case Choice:
			probs := make(map[string]float64, len(row.Options))
			for j, option := range row.Options {
				probs[option.Key] = result.Probabilities[j]
			}
			response.Answers[row.ID] = map[string]any{"type": "choice", "choice": row.Options[result.Winner].Key,
				"probabilities": probs, "confidence": confidence(result.Probabilities)}
		case Score:
			probs := make(map[string]float64, len(row.Options))
			legend := make(map[string]any, len(row.Options))
			score := 0.0
			for j, option := range row.Options {
				key := strconv.Itoa(j)
				probs[key] = result.Probabilities[j]
				score += float64(j) * result.Probabilities[j]
				// RawMessage preserves structured rubric content in the response.
				legend[key] = option.Raw
			}
			response.Answers[row.ID] = map[string]any{"type": "score", "score": score,
				"probabilities": probs, "legend": legend, "confidence": confidence(result.Probabilities)}
		}
		if model.Chinese {
			response.Answers[row.ID].(map[string]any)["probabilities"] = probabilityObject{row.Options, result.Probabilities}
		}
	}
	return response, nil
}

func confidence(probs []float64) float64 {
	n := len(probs)
	if n < 2 {
		return 0
	}
	pmax := probs[0]
	for _, p := range probs[1:] {
		if p > pmax {
			pmax = p
		}
	}
	v := (float64(n)*pmax - 1) / float64(n-1)
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
