package d1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/opendecisionserver/ods/internal/decision"
	"sync"
	"sync/atomic"
	"time"
)

type native interface {
	Run([]byte, int) ([]decision.Result, error)
	Close() error
	Info() map[string]any
}
type job struct {
	ctx     context.Context
	payload []byte
	done    chan reply
	queued  time.Time
}
type reply struct {
	results []decision.Result
	err     error
}
type Engine struct {
	mu                                                              sync.Mutex
	n                                                               native
	queue                                                           []*job
	current                                                         bool
	stopped                                                         bool
	failed                                                          error
	maxWaiting, budget                                              int
	wake, done                                                      chan struct{}
	closeErr                                                        error
	info                                                            map[string]any
	runs, active, rejected, canceled, failures, waitNS, inferenceNS atomic.Int64
}

func newEngine(n native, waiting, budget int) *Engine {
	e := &Engine{n: n, maxWaiting: waiting, budget: budget, wake: make(chan struct{}, 1), done: make(chan struct{}), info: n.Info()}
	go e.loop()
	return e
}
func encode(rows []decision.Input) ([]byte, error) {
	if len(rows) < 1 || len(rows) > 64 {
		return nil, fmt.Errorf("requires 1-64 questions")
	}
	var b bytes.Buffer
	b.WriteString(`{"state":`)
	b.Write(rows[0].State)
	b.WriteString(`,"questions":{`)
	for i, row := range rows {
		if !bytes.Equal(row.State, rows[0].State) || !json.Valid(row.RawQuestion) {
			return nil, fmt.Errorf("invalid d1 question")
		}
		if i > 0 {
			b.WriteByte(',')
		}
		key, _ := json.Marshal(row.ID)
		b.Write(key)
		b.WriteByte(':')
		b.Write(row.RawQuestion)
	}
	b.WriteString(`}}`)
	if b.Len() > 1<<20 {
		return nil, fmt.Errorf("native request exceeds 1 MiB")
	}
	return b.Bytes(), nil
}
func (e *Engine) Decide(ctx context.Context, rows []decision.Input) ([]decision.Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	payload, err := encode(rows)
	if err != nil {
		return nil, decision.FieldError{Location: []any{"body"}, Message: err.Error(), Type: "value_error"}
	}
	j := &job{ctx: ctx, payload: payload, done: make(chan reply, 1), queued: time.Now()}
	e.mu.Lock()
	if e.stopped {
		e.mu.Unlock()
		return nil, decision.ErrUnavailable
	}
	jobs := len(e.queue)
	if e.current {
		jobs++
	}
	if jobs >= e.maxWaiting+1 {
		e.mu.Unlock()
		e.rejected.Add(1)
		return nil, decision.ErrOverloaded
	}
	e.queue = append(e.queue, j)
	e.signal()
	e.mu.Unlock()
	select {
	case result := <-j.done:
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return result.results, result.err
	case <-ctx.Done():
		e.canceled.Add(1)
		e.mu.Lock()
		for i, pending := range e.queue {
			if pending == j {
				e.queue = append(e.queue[:i], e.queue[i+1:]...)
				break
			}
		}
		e.mu.Unlock()
		return nil, ctx.Err()
	}
}
func (e *Engine) signal() {
	select {
	case e.wake <- struct{}{}:
	default:
	}
}
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopped = true
	for _, j := range e.queue {
		j.done <- reply{err: decision.ErrUnavailable}
	}
	e.queue = nil
	e.signal()
}
func (e *Engine) Close() error          { e.Stop(); <-e.done; return e.closeErr }
func (e *Engine) Done() <-chan struct{} { return e.done }
func (e *Engine) Healthy() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return !e.stopped && e.failed == nil
}
func (e *Engine) Waiting() int      { e.mu.Lock(); defer e.mu.Unlock(); return len(e.queue) }
func (e *Engine) RunCount() int64   { return e.runs.Load() }
func (e *Engine) ActiveRuns() int64 { return e.active.Load() }
func (e *Engine) MaxActiveRuns() int64 {
	if e.runs.Load() > 0 {
		return 1
	}
	return 0
}
func (e *Engine) loop() {
	defer func() { e.closeErr = errors.Join(e.failed, e.n.Close()); close(e.done) }()
	for {
		e.mu.Lock()
		if len(e.queue) == 0 {
			stop := e.stopped
			e.mu.Unlock()
			if stop {
				return
			}
			<-e.wake
			continue
		}
		j := e.queue[0]
		e.queue = e.queue[1:]
		e.current = true
		e.mu.Unlock()
		if j.ctx.Err() != nil {
			j.done <- reply{err: j.ctx.Err()}
			e.mu.Lock()
			e.current = false
			e.mu.Unlock()
			continue
		}
		e.waitNS.Add(time.Since(j.queued).Nanoseconds())
		start := time.Now()
		e.active.Store(1)
		results, err := e.n.Run(j.payload, e.budget)
		e.active.Store(0)
		e.inferenceNS.Add(time.Since(start).Nanoseconds())
		e.runs.Add(int64(len(results)))
		e.mu.Lock()
		e.current = false
		var field decision.FieldError
		if err != nil && !errors.As(err, &field) {
			e.failed = err
			e.stopped = true
			e.failures.Add(1)
			for _, pending := range e.queue {
				pending.done <- reply{err: decision.ErrUnavailable}
			}
			e.queue = nil
		}
		e.mu.Unlock()
		if j.ctx.Err() != nil {
			j.done <- reply{err: j.ctx.Err()}
		} else {
			j.done <- reply{results, err}
		}
	}
}
func (e *Engine) Warmup(ctx context.Context) error {
	req, err := decision.ParseForModel([]byte(`{"state":"用户需要帮助","questions":{"warmup":{"type":"noul","instructions":"是否需要帮助？"}}}`), 64, decision.D1Model())
	if err != nil {
		return err
	}
	_, err = e.Decide(ctx, req.Questions)
	return err
}
func (e *Engine) RuntimeInfo() map[string]any {
	out := map[string]any{}
	for k, v := range e.info {
		out[k] = v
	}
	out["max_waiting"] = e.maxWaiting
	out["max_request_compiled_tokens"] = e.budget
	return out
}
func (e *Engine) Metrics() string {
	return fmt.Sprintf("ods_native_rejections_total %d\nods_native_cancellations_total %d\nods_native_errors_total %d\nods_native_queue_seconds_total %.9f\nods_native_inference_seconds_total %.9f\n", e.rejected.Load(), e.canceled.Load(), e.failures.Load(), float64(e.waitNS.Load())/1e9, float64(e.inferenceNS.Load())/1e9)
}
