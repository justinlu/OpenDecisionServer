package decision

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeEngine struct {
	active    atomic.Int64
	maxActive atomic.Int64
}

func (f *fakeEngine) Decide(ctx context.Context, rows []Input) ([]Result, error) {
	active := f.active.Add(1)
	defer f.active.Add(-1)
	for {
		old := f.maxActive.Load()
		if active <= old || f.maxActive.CompareAndSwap(old, active) {
			break
		}
	}
	select {
	case <-time.After(5 * time.Millisecond):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	out := make([]Result, len(rows))
	for i, row := range rows {
		probs := make([]float64, len(row.Options))
		probs[len(probs)-1] = 1
		out[i] = Result{Probabilities: probs, Winner: len(probs) - 1, InputTokens: 10}
	}
	return out, nil
}

func TestSingleEngineHandlesConcurrentRequests(t *testing.T) {
	engine := &fakeEngine{}
	service := Service{Engine: engine, Admission: NewAdmission(4, 128)}
	const clients = 64
	var wg sync.WaitGroup
	errCh := make(chan error, clients)
	for i := 0; i < clients; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := Request{Model: "jev-latest", Questions: []Input{{ID: "q", Type: Noul, State: json.RawMessage(`"x"`), Options: []Option{{Text: "false"}, {Text: "true"}}}}}
			resp, err := service.Decide(context.Background(), req)
			if err != nil {
				errCh <- err
				return
			}
			answer := resp.Answers["q"].(map[string]any)
			if answer["noul"] != float64(1) || resp.Usage.InputTokens != 10 {
				errCh <- errors.New("wrong response")
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
	if got := engine.maxActive.Load(); got < 2 || got > 4 {
		t.Fatalf("active runs: %d", got)
	}
	if service.Admission.Active() != 0 || service.Admission.Waiting() != 0 {
		t.Fatal("admission counters leaked")
	}
}

func TestAdmissionRejectsFullQueueAndCancellation(t *testing.T) {
	a := NewAdmission(1, 0)
	release, err := a.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Acquire(context.Background()); !errors.Is(err, ErrOverloaded) {
		t.Fatalf("wanted overload, got %v", err)
	}
	release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.Acquire(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("wanted cancellation, got %v", err)
	}
}
