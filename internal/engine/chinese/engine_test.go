package chinese

import (
	"context"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/opendecisionserver/ods/internal/decision"
)

type fakeNative struct {
	entered chan struct{}
	release chan struct{}
	active  atomic.Int64
	closed  atomic.Bool
	calls   atomic.Int64
	fail    bool
	mu      sync.Mutex
	order   []int32
}

func (f *fakeNative) Tokenize(s string) ([]int32, error) {
	if len(s) == 1 {
		return []int32{int32(s[0] - 'A' + 32)}, nil
	}
	return []int32{int32(len(s))}, nil
}
func (f *fakeNative) Logits(ids []int32, count int) ([]float32, error) {
	if f.closed.Load() || f.active.Add(1) != 1 {
		panic("invalid native ownership")
	}
	defer f.active.Add(-1)
	f.calls.Add(1)
	f.mu.Lock()
	f.order = append(f.order, ids[0])
	f.mu.Unlock()
	if f.entered != nil {
		f.entered <- struct{}{}
		<-f.release
	}
	if f.fail {
		return nil, errors.New("native failed")
	}
	values := make([]float32, count)
	values[count-1] = 2
	return values, nil
}
func (f *fakeNative) Close() error {
	if f.active.Load() != 0 {
		panic("freed active native")
	}
	f.closed.Store(true)
	return nil
}
func input(id string) decision.Input {
	return decision.Input{ID: id, Type: decision.Noul, State: []byte(`"中文"`), Question: "是否？", Options: []decision.Option{{Key: "false", Text: "否"}, {Key: "true", Text: "是"}}}
}
func wait(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
}
func TestAtomicValidationAndLimits(t *testing.T) {
	f := &fakeNative{}
	e := newEngine(f, 1, 2, 100)
	defer e.Close()
	bad := input("bad")
	bad.Options = bad.Options[:1]
	if _, err := e.Decide(context.Background(), []decision.Input{input("ok"), bad}); err == nil {
		t.Fatal("invalid accepted")
	}
	if f.calls.Load() != 0 {
		t.Fatal("partially executed request")
	}
	e.maxTokens = 0
	if _, err := e.Decide(context.Background(), []decision.Input{input("limit")}); !errors.Is(err, decision.ErrOverloaded) {
		t.Fatalf("%v", err)
	}
}
func TestCancelAndCloseWaitForNative(t *testing.T) {
	f := &fakeNative{entered: make(chan struct{}, 3), release: make(chan struct{})}
	e := newEngine(f, 1, 1, 100)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := e.Decide(ctx, []decision.Input{input("a")}); done <- err }()
	wait(t, f.entered)
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	closed := make(chan struct{})
	go func() { e.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("Close freed running native")
	case <-time.After(20 * time.Millisecond):
	}
	close(f.release)
	wait(t, closed)
	if !f.closed.Load() {
		t.Fatal("not closed")
	}
	if _, err := e.Decide(context.Background(), []decision.Input{input("b")}); err == nil {
		t.Fatal("accepted after close")
	}
	e.Close()
}
func TestQueuedCancellationAndBudgetRelease(t *testing.T) {
	f := &fakeNative{entered: make(chan struct{}, 3), release: make(chan struct{})}
	e := newEngine(f, 1, 1, 100)
	running := make(chan struct{})
	go func() { e.Decide(context.Background(), []decision.Input{input("active")}); close(running) }()
	wait(t, f.entered)
	ctx, cancel := context.WithCancel(context.Background())
	queued := make(chan error, 1)
	go func() { _, err := e.Decide(ctx, []decision.Input{input("queued")}); queued <- err }()
	deadline := time.Now().Add(time.Second)
	for e.Waiting() != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if e.Waiting() != 1 {
		t.Fatal("not queued")
	}
	if _, err := e.Decide(context.Background(), []decision.Input{input("over")}); !errors.Is(err, decision.ErrOverloaded) {
		t.Fatalf("overload %v", err)
	}
	cancel()
	if err := <-queued; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if e.Waiting() != 0 {
		t.Fatal("canceled job retained")
	}
	close(f.release)
	wait(t, running)
	e.Close()
	if e.reserved != 0 || f.calls.Load() != 1 {
		t.Fatalf("budget %d calls %d", e.reserved, f.calls.Load())
	}
}
func TestNativeFailureRemovesReadiness(t *testing.T) {
	f := &fakeNative{fail: true}
	e := newEngine(f, 1, 1, 100)
	if _, err := e.Decide(context.Background(), []decision.Input{input("q")}); err == nil {
		t.Fatal("ignored native failure")
	}
	if e.Healthy() {
		t.Fatal("unhealthy engine ready")
	}
	e.Close()
}
func TestSoftmaxRejectsInvalidLogitsAndUsesTemperature(t *testing.T) {
	p, err := probabilities([]float32{0, 2}, 2)
	if err != nil || p[1] < .731 || p[1] > .732 {
		t.Fatalf("%v %v", p, err)
	}
	if _, err := probabilities([]float32{0, 2}, 0); err == nil {
		t.Fatal("invalid temperature")
	}
	if _, err := probabilities([]float32{0, float32(math.Inf(-1))}, 1); err == nil {
		t.Fatal("nonfinite logit accepted")
	}
}

func TestRequestsRunRoundRobin(t *testing.T) {
	f := &fakeNative{entered: make(chan struct{}, 8), release: make(chan struct{})}
	e := newEngine(f, 1, 2, 100)
	defer e.Close()
	a, b := input("a"), input("b")
	a.Question = "a"
	b.Question = "bbbb"
	finished := make(chan struct{}, 2)
	go func() {
		_, err := e.Decide(context.Background(), []decision.Input{a, a, a})
		if err != nil {
			t.Error(err)
		}
		finished <- struct{}{}
	}()
	wait(t, f.entered)
	go func() {
		_, err := e.Decide(context.Background(), []decision.Input{b, b})
		if err != nil {
			t.Error(err)
		}
		finished <- struct{}{}
	}()
	deadline := time.Now().Add(time.Second)
	for e.Waiting() != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if e.Waiting() != 1 {
		close(f.release)
		t.Fatal("second request not queued")
	}
	for i := 0; i < 5; i++ {
		f.release <- struct{}{}
		if i < 4 {
			wait(t, f.entered)
		}
	}
	wait(t, finished)
	wait(t, finished)
	pa, _ := Prompt(a)
	pb, _ := Prompt(b)
	want := []int32{int32(len(pa)), int32(len(pb)), int32(len(pa)), int32(len(pb)), int32(len(pa))}
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, id := range want {
		if f.order[i] != id {
			t.Fatalf("order %v, want %v", f.order, want)
		}
	}
}
