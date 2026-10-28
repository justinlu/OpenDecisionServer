package d1

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/opendecisionserver/ods/internal/decision"
	"sync/atomic"
	"testing"
	"time"
)

type blockedNative struct {
	entered chan struct{}
	release chan struct{}
	closed  atomic.Bool
}

func (n *blockedNative) Run([]byte, int) ([]decision.Result, error) {
	close(n.entered)
	<-n.release
	return []decision.Result{{Answer: json.RawMessage(`{"type":"noul","noul":0.9}`), InputTokens: 2}}, nil
}
func (n *blockedNative) Close() error         { n.closed.Store(true); return nil }
func (n *blockedNative) Info() map[string]any { return map[string]any{} }
func TestCanceledCallDrainsBeforeClose(t *testing.T) {
	n := &blockedNative{entered: make(chan struct{}), release: make(chan struct{})}
	e := newEngine(n, 0, 65536)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := e.Decide(ctx, []decision.Input{{ID: "q", State: json.RawMessage(`"hi"`), RawQuestion: json.RawMessage(`{"type":"noul","instructions":"x"}`)}})
		done <- err
	}()
	<-n.entered
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	closed := make(chan struct{})
	go func() { e.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("freed active native call")
	case <-time.After(20 * time.Millisecond):
	}
	if n.closed.Load() {
		t.Fatal("native closed early")
	}
	close(n.release)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("failed to drain")
	}
	if !n.closed.Load() {
		t.Fatal("native not closed")
	}
}

type failedNative struct{ closed atomic.Bool }

func (n *failedNative) Run([]byte, int) ([]decision.Result, error) {
	return nil, errors.New("backend command-buffer failed")
}
func (n *failedNative) Close() error         { n.closed.Store(true); return nil }
func (n *failedNative) Info() map[string]any { return map[string]any{} }
func TestBackendFailureRevokesReadiness(t *testing.T) {
	n := &failedNative{}
	e := newEngine(n, 1, 65536)
	rows := []decision.Input{{ID: "q", State: json.RawMessage(`"hi"`), RawQuestion: json.RawMessage(`{"type":"noul","instructions":"x"}`)}}
	if _, err := e.Decide(context.Background(), rows); err == nil {
		t.Fatal("returned success after backend failure")
	}
	if e.Healthy() {
		t.Fatal("failed engine still ready")
	}
	if _, err := e.Decide(context.Background(), rows); !errors.Is(err, decision.ErrUnavailable) {
		t.Fatal(err)
	}
	if e.Close() == nil || !n.closed.Load() {
		t.Fatal("failure was lost or native not freed")
	}
}
