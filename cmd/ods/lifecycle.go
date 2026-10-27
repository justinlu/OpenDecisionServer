package main

import (
	"context"
	"errors"
	httpapi "github.com/opendecisionserver/ods/internal/api/http"
	"github.com/opendecisionserver/ods/internal/decision"
	"net/http"
	"sync"
	"time"
)

type runtimeEngine interface {
	decision.Engine
	Warmup(context.Context) error
	Close() error
}

func waitServer(ctx context.Context, serveErrors <-chan error, engineDone <-chan struct{}) error {
	select {
	case <-ctx.Done():
		return nil
	case err := <-serveErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-engineDone:
		return nil // failed native worker returns its cause from Close
	}
}

// Admission is closed under the same lock as WaitGroup.Add, so shutdown's Wait
// can never race a newly admitted handler, including one still reading its body.
type drainHandler struct {
	mu     sync.Mutex
	closed bool
	active sync.WaitGroup
	next   http.Handler
}

func newDrainHandler(next http.Handler) *drainHandler { return &drainHandler{next: next} }
func (g *drainHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	if g.closed {
		g.mu.Unlock()
		http.Error(w, "server shutting down", 503)
		return
	}
	g.active.Add(1)
	g.mu.Unlock()
	defer g.active.Done()
	g.next.ServeHTTP(w, r)
}
func (g *drainHandler) stop() { g.mu.Lock(); g.closed = true; g.mu.Unlock() }
func shutdown(server *http.Server, gate *drainHandler, engine runtimeEngine, api *httpapi.Server, timeout time.Duration) error {
	api.SetReady(false)
	gate.stop()
	if stopper, ok := engine.(interface{ Stop() }); ok {
		stopper.Stop()
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	err := server.Shutdown(ctx)
	if err != nil {
		_ = server.Close()
	} // cancel connections, including stalled body readers
	gate.active.Wait()
	return errors.Join(err, engine.Close()) // native Close additionally joins GPU work
}
