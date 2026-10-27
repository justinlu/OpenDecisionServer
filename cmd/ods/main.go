package main

import (
	"context"
	"errors"
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
	"github.com/opendecisionserver/ods/internal/engine/chinese"
	"github.com/opendecisionserver/ods/internal/engine/julia"
)

func defaultLibrary() string {
	if runtime.GOOS == "darwin" {
		return "lib/darwin-arm64/libonnxruntime.dylib"
	}
	return "lib/linux-amd64/libonnxruntime.so"
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	selected := flag.String("engine", "julia", "inference engine: julia or chinese-decision")
	llamaModel := flag.String("llama-model", "models/chinese-decision-2b/chinese-decision-2b-Q8_0.gguf", "pinned Chinese Decision GGUF path")
	llamaConfig := flag.String("llama-config", "", "Chinese Decision config/tokenizer directory (default: model directory)")
	llamaLibrary := flag.String("llama-lib", "lib/darwin-arm64/llama/lib/libllama.dylib", "pinned standalone llama.cpp dynamic library")
	llamaThreads := flag.Int("llama-threads", 4, "native CPU helper threads")
	llamaWaiting := flag.Int("llama-max-waiting", 32, "native waiting requests")
	llamaTokens := flag.Int("llama-max-tokens", 65536, "maximum reserved compiled native tokens")
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
	if *threads < 0 || *maxRuns < 1 || *maxWaiting < 0 || *maxQuestions < 1 || (*selected != "julia" && *selected != "chinese-decision") || (*selected == "chinese-decision" && (*maxQuestions > 64 || *llamaThreads < 1 || *llamaWaiting < 0 || *llamaTokens < 1)) {
		return fmt.Errorf("invalid configuration")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	api := &httpapi.Server{MaxRequestBytes: 1 << 20, MaxQuestions: *maxQuestions, Logger: logger, LogRequests: *logRequests}
	gate := newDrainHandler(api.Handler())
	httpServer := &http.Server{Addr: *addr, Handler: gate, ReadHeaderTimeout: 5 * time.Second}
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- httpServer.Serve(listener) }()
	// This also covers startup failures while readiness is still false.
	defer httpServer.Close()
	var engine runtimeEngine
	modelInfo := decision.JuliaModel()
	var admission *decision.Admission
	if *selected == "chinese-decision" {
		modelInfo = decision.ChineseModel()
		engine, err = chinese.New(chinese.Options{ModelPath: *llamaModel, ConfigDir: *llamaConfig, LibraryPath: *llamaLibrary, Threads: *llamaThreads, MaxWaiting: *llamaWaiting, MaxQueuedTokens: *llamaTokens})
	} else {
		var libPath string
		libPath, err = filepath.Abs(*library)
		if err == nil {
			engine, err = julia.New(julia.Options{ModelPath: *model, TokenizerPath: *tokenizer, LibraryPath: libPath, IntraOpThreads: *threads, MaxLength: *maxLength, HeadLength: *headLength})
		}
		admission = decision.NewAdmission(*maxRuns, *maxWaiting)
	}
	if err != nil {
		return err
	}
	warmCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	if err := engine.Warmup(warmCtx); err != nil {
		cancel()
		_ = engine.Close()
		return fmt.Errorf("warmup: %w", err)
	}
	cancel()
	api.Service = &decision.Service{Model: modelInfo, Engine: engine, Admission: admission}
	api.SetReady(true)
	logger.Info("ready", "addr", *addr, "model", modelInfo.ID, "engine", *selected)
	if info, ok := engine.(interface{ RuntimeInfo() map[string]any }); ok {
		logger.Info("native runtime", "configuration", info.RuntimeInfo())
	}
	cause := waitServer(ctx, serveErrors, unhealthy(engine))
	return errors.Join(cause, shutdown(httpServer, gate, engine, api, 30*time.Second))
}
func unhealthy(engine runtimeEngine) <-chan struct{} {
	if e, ok := engine.(interface{ Done() <-chan struct{} }); ok {
		return e.Done()
	}
	return nil
}
