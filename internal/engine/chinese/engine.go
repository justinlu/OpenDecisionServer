package chinese

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/opendecisionserver/ods/internal/decision"
)

type native interface {
	Tokenize(string) ([]int32, error)
	Logits([]int32, int) ([]float32, error)
	Close() error
}
type encoded struct {
	ids   []int32
	count int
}
type reply struct {
	results []decision.Result
	err     error
}
type job struct {
	ctx                         context.Context
	inputs                      []encoded
	results                     []decision.Result
	index, tokens               int
	running, finished, canceled bool
	queued                      time.Time
	done                        chan reply
}

type Engine struct {
	mu                                                    sync.Mutex
	native                                                native
	temperature                                           float64
	maxWaiting, maxTokens                                 int
	reserved                                              int // all accepted compiled tokens, including the active request
	current                                               *job
	queue                                                 []*job
	stopped                                               bool
	failed                                                error
	wake                                                  chan struct{}
	done                                                  chan struct{}
	compileSlots                                          chan struct{}
	closeErr                                              error
	runs, active, maxActive, rejected, canceled, failures atomic.Int64
	compileNS, waitNS, inferenceNS                        atomic.Int64
}

func newEngine(n native, temperature float64, maxWaiting, maxTokens int) *Engine {
	e := &Engine{native: n, temperature: temperature, maxWaiting: maxWaiting, maxTokens: maxTokens, wake: make(chan struct{}, 1), done: make(chan struct{}), compileSlots: make(chan struct{}, maxWaiting+1)}
	go e.loop()
	return e
}
func (e *Engine) signal() {
	select {
	case e.wake <- struct{}{}:
	default:
	}
}
func (e *Engine) Healthy() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return !e.stopped && e.failed == nil
}
func (e *Engine) Waiting() int         { e.mu.Lock(); defer e.mu.Unlock(); return len(e.queue) }
func (e *Engine) RunCount() int64      { return e.runs.Load() }
func (e *Engine) ActiveRuns() int64    { return e.active.Load() }
func (e *Engine) MaxActiveRuns() int64 { return e.maxActive.Load() }

func (e *Engine) Decide(ctx context.Context, rows []decision.Input) ([]decision.Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case e.compileSlots <- struct{}{}:
		defer func() { <-e.compileSlots }()
	default:
		e.rejected.Add(1)
		return nil, decision.ErrOverloaded
	}
	e.mu.Lock()
	if e.stopped {
		e.mu.Unlock()
		return nil, decision.ErrUnavailable
	}
	capacity := e.maxWaiting + 1
	jobs := len(e.queue)
	if e.current != nil {
		jobs++
	}
	if jobs >= capacity {
		e.mu.Unlock()
		e.rejected.Add(1)
		return nil, decision.ErrOverloaded
	}
	if len(rows) < 1 || len(rows) > 64 {
		e.mu.Unlock()
		return nil, decision.FieldError{Location: []any{"body", "questions"}, Message: "requires 1-64 questions", Type: "value_error"}
	}
	started := time.Now()
	j := &job{ctx: ctx, done: make(chan reply, 1), results: make([]decision.Result, len(rows))}
	// Compile the entire request before scheduling its first question. Holding mu
	// prevents native.Close from racing tokenizer access during shutdown. The
	// llama vocabulary tokenizer is thread-safe with respect to active inference.
	for _, row := range rows {
		prompt, err := Prompt(row)
		var ids []int32
		if err == nil {
			ids, err = e.native.Tokenize(prompt)
		}
		if err == nil && (len(ids) == 0 || len(ids) > 16384) {
			err = fmt.Errorf("encoded prompt requires 1-16384 tokens; got %d", len(ids))
		}
		if err != nil {
			e.mu.Unlock()
			return nil, decision.FieldError{Location: []any{"body", "questions", row.ID}, Message: err.Error(), Type: "value_error"}
		}
		j.inputs = append(j.inputs, encoded{ids, len(row.Options)})
		j.tokens += len(ids)
	}
	e.compileNS.Add(time.Since(started).Nanoseconds())
	if ctx.Err() != nil {
		e.mu.Unlock()
		return nil, ctx.Err()
	}
	if j.tokens > e.maxTokens-e.reserved {
		e.mu.Unlock()
		e.rejected.Add(1)
		return nil, decision.ErrOverloaded
	}
	e.reserved += j.tokens
	j.queued = time.Now()
	e.queue = append(e.queue, j)
	e.signal()
	e.mu.Unlock()
	select {
	case r := <-j.done:
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return r.results, r.err
	case <-ctx.Done():
		e.mu.Lock()
		if !j.finished {
			j.canceled = true
			e.canceled.Add(1)
			if !j.running {
				for i, queued := range e.queue {
					if queued == j {
						e.queue = append(e.queue[:i], e.queue[i+1:]...)
						break
					}
				}
				e.finish(j, ctx.Err())
			}
		}
		e.mu.Unlock()
		return nil, ctx.Err()
	}
}

func (e *Engine) finish(j *job, err error) {
	if j.finished {
		return
	}
	j.finished = true
	e.reserved -= j.tokens
	var results []decision.Result
	if err == nil {
		results = j.results
	}
	j.done <- reply{results, err}
}
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopped = true
	for _, j := range e.queue {
		e.finish(j, decision.ErrUnavailable)
	}
	e.queue = nil
	e.signal()
}
func (e *Engine) Close() error { e.Stop(); <-e.done; return e.closeErr }
func (e *Engine) loop() {
	defer func() { e.closeErr = e.native.Close(); close(e.done) }()
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
		j.running = true
		e.current = j
		if j.ctx.Err() != nil {
			e.finish(j, j.ctx.Err())
			e.current = nil
			j.running = false
			e.mu.Unlock()
			continue
		}
		e.waitNS.Add(time.Since(j.queued).Nanoseconds())
		item := j.inputs[j.index]
		e.mu.Unlock()
		start := time.Now()
		e.active.Store(1)
		e.maxActive.Store(1)
		e.runs.Add(1)
		logits, err := e.native.Logits(item.ids, item.count)
		e.active.Store(0)
		e.inferenceNS.Add(time.Since(start).Nanoseconds())
		var probs []float64
		if err == nil && len(logits) != item.count {
			err = fmt.Errorf("native candidate count mismatch")
		}
		if err == nil {
			probs, err = probabilities(logits, e.temperature)
		}
		e.mu.Lock()
		j.running = false
		e.current = nil
		switch {
		case err != nil:
			e.failures.Add(1)
			e.failed = err
			e.stopped = true
			e.finish(j, err)
			for _, pending := range e.queue {
				e.finish(pending, decision.ErrUnavailable)
			}
			e.queue = nil
		case j.canceled || j.ctx.Err() != nil:
			e.finish(j, j.ctx.Err())
		default:
			best := 0
			for i, p := range probs {
				if p > probs[best] {
					best = i
				}
			}
			j.results[j.index] = decision.Result{Logits: logits, Probabilities: probs, Winner: best, InputTokens: len(item.ids)}
			j.index++
			if j.index == len(j.inputs) {
				e.finish(j, nil)
			} else if e.stopped {
				e.finish(j, decision.ErrUnavailable)
			} else {
				j.queued = time.Now()
				e.queue = append(e.queue, j)
			}
		}
		e.mu.Unlock()
	}
}

func probabilities(logits []float32, temperature float64) ([]float64, error) {
	if temperature <= 0 || math.IsNaN(temperature) || math.IsInf(temperature, 0) || len(logits) < 2 || len(logits) > 26 {
		return nil, errors.New("invalid calibrated distribution")
	}
	out := make([]float64, len(logits))
	maximum := math.Inf(-1)
	for i, x := range logits {
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
			return nil, errors.New("nonfinite candidate logit")
		}
		out[i] = float64(x) / temperature
		if out[i] > maximum {
			maximum = out[i]
		}
	}
	total := 0.
	for i, x := range out {
		out[i] = math.Exp(x - maximum)
		total += out[i]
	}
	for i := range out {
		out[i] /= total
		if math.IsNaN(out[i]) || math.IsInf(out[i], 0) {
			return nil, errors.New("nonfinite probability")
		}
	}
	return out, nil
}

func (e *Engine) Warmup(ctx context.Context) error {
	_, err := e.Decide(ctx, []decision.Input{{ID: "warmup", Type: decision.Noul, State: []byte(`"用户请求帮助"`), Question: "是否需要帮助？", Options: []decision.Option{{Key: "false", Text: "否"}, {Key: "true", Text: "是"}}}})
	return err
}
func (e *Engine) Metrics() string {
	e.mu.Lock()
	tokens, waiting := e.reserved, len(e.queue)
	e.mu.Unlock()
	return fmt.Sprintf("ods_native_waiting_requests %d\nods_native_reserved_tokens %d\nods_native_rejections_total %d\nods_native_cancellations_total %d\nods_native_errors_total %d\nods_native_compile_seconds_total %.9f\nods_native_queue_seconds_total %.9f\nods_native_inference_seconds_total %.9f\n", waiting, tokens, e.rejected.Load(), e.canceled.Load(), e.failures.Load(), float64(e.compileNS.Load())/1e9, float64(e.waitNS.Load())/1e9, float64(e.inferenceNS.Load())/1e9)
}
