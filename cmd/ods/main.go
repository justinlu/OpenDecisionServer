package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	httpapi "github.com/opendecisionserver/ods/internal/api/http"
	"github.com/opendecisionserver/ods/internal/decision"
	"github.com/opendecisionserver/ods/internal/engine/julia"
)

func defaultLibrary() string {
	if runtime.GOOS == "darwin" {
		return "lib/darwin-arm64/libonnxruntime.dylib"
	}
	return "lib/linux-amd64/libonnxruntime.so"
}

func main() {
	addr := flag.String("http", ":8080", "HTTP listen address")
	model := flag.String("model", "models/julia-1/model.onnx", "Julia ONNX graph path")
	tokenizer := flag.String("tokenizer", "models/julia-1/tokenizer.json", "Julia tokenizer.json path")
	library := flag.String("ort-lib", defaultLibrary(), "ONNX Runtime native library path")
	threads := flag.Int("ort-threads", 4, "ORT intra-op threads")
	maxRuns := flag.Int("max-runs", 4, "maximum simultaneous Session.Run calls")
	maxWaiting := flag.Int("max-waiting", 128, "maximum requests waiting for a run slot")
	maxQuestions := flag.Int("max-questions", 64, "maximum questions per request")
	maxLength := flag.Int("max-length", 1024, "Julia maximum encoded sequence length")
	headLength := flag.Int("head-length", 256, "Julia question and option token budget")
	logRequests := flag.Bool("log-requests", false, "log metadata for every request")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if *threads < 0 || *maxRuns < 1 || *maxWaiting < 0 || *maxQuestions < 1 {
		logger.Error("invalid configuration")
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	api := &httpapi.Server{MaxRequestBytes: 1 << 20, MaxQuestions: *maxQuestions, Logger: logger, LogRequests: *logRequests}
	httpServer := &http.Server{Addr: *addr, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second}
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		fatal(logger, err)
	}
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- httpServer.Serve(listener) }()
	libPath, err := filepath.Abs(*library)
	if err != nil {
		fatal(logger, err)
	}
	engine, err := julia.New(julia.Options{ModelPath: *model, TokenizerPath: *tokenizer, LibraryPath: libPath,
		IntraOpThreads: *threads, MaxLength: *maxLength, HeadLength: *headLength})
	if err != nil {
		fatal(logger, err)
	}
	defer func() {
		if err := engine.Close(); err != nil {
			logger.Error("close engine", "error", err)
		}
	}()
	warmCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	if err := engine.Warmup(warmCtx); err != nil {
		cancel()
		fatal(logger, fmt.Errorf("warmup: %w", err))
	}
	cancel()
	api.Service = &decision.Service{Engine: engine, Admission: decision.NewAdmission(*maxRuns, *maxWaiting)}
	api.SetReady(true)
	logger.Info("ready", "addr", *addr, "model", *model, "max_runs", *maxRuns, "ort_threads", *threads)
	select {
	case <-ctx.Done():
	case err := <-serveErrors:
		if err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server failed", "error", err)
		}
	}
	api.SetReady(false)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP shutdown", "error", err)
	}
}

func fatal(logger *slog.Logger, err error) {
	logger.Error("startup failed", "error", err)
	os.Exit(1)
}
