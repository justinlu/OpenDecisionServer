package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	httpapi "github.com/opendecisionserver/ods/internal/api/http"
	"github.com/opendecisionserver/ods/internal/decision"
)

func TestServeFailurePropagates(t *testing.T) {
	want := errors.New("listener failed")
	errorsCh := make(chan error, 1)
	errorsCh <- want
	if err := waitServer(context.Background(), errorsCh, nil); !errors.Is(err, want) {
		t.Fatalf("lost Serve failure: %v", err)
	}
	errorsCh <- http.ErrServerClosed
	if err := waitServer(context.Background(), errorsCh, nil); err != nil {
		t.Fatal(err)
	}
}

type shutdownEngine struct {
	closed  atomic.Bool
	stopped chan struct{}
}

func (*shutdownEngine) Decide(context.Context, []decision.Input) ([]decision.Result, error) {
	return nil, nil
}
func (*shutdownEngine) Warmup(context.Context) error { return nil }
func (e *shutdownEngine) Stop()                      { close(e.stopped) }
func (e *shutdownEngine) Close() error               { e.closed.Store(true); return nil }

func TestShutdownTimeoutStillWaitsForHandlerBeforeFree(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	gate := newDrainHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(entered); <-release }))
	server := httptest.NewServer(gate)
	defer server.Close()
	go func() {
		resp, err := http.Get(server.URL)
		if err == nil {
			resp.Body.Close()
		}
	}()
	<-entered
	engine := &shutdownEngine{stopped: make(chan struct{})}
	api := &httpapi.Server{}
	done := make(chan struct{})
	go func() { shutdown(server.Config, gate, engine, api, 5*time.Millisecond); close(done) }()
	<-engine.stopped
	select {
	case <-done:
		t.Fatal("shutdown skipped active handler")
	case <-time.After(20 * time.Millisecond):
	}
	if engine.closed.Load() {
		t.Fatal("engine freed while handler active")
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not finish")
	}
	if !engine.closed.Load() {
		t.Fatal("engine not closed")
	}
}

func TestDefaultD1Paths(t *testing.T) {
	if defaultEngine != "d1-2b" {
		t.Fatal(defaultEngine)
	}
	model, lib := nativePaths("d1-2b", "", "")
	if model != "dist/liquid-d1/models/d1-2B-Q8_0.gguf" || lib != "lib/darwin-arm64/d1/lib/libods_d1.dylib" {
		t.Fatalf("%s %s", model, lib)
	}
	model, lib = nativePaths("chinese-decision", "custom.gguf", "custom.dylib")
	if model != "custom.gguf" || lib != "custom.dylib" {
		t.Fatal("explicit paths ignored")
	}
}

func TestListenAllRollsBackOnConflict(t *testing.T) {
	listeners, err := listenAll("127.0.0.1:0,127.0.0.1:0")
	if err != nil || len(listeners) != 2 {
		t.Fatalf("%v %v", listeners, err)
	}
	defer func() {
		for _, l := range listeners {
			l.Close()
		}
	}()
	if _, err = listenAll("127.0.0.1:0," + listeners[1].Addr().String()); err == nil {
		t.Fatal("accepted occupied address")
	}
}
