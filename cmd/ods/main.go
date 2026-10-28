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
	"strings"
	"syscall"
	"time"

	httpapi "github.com/opendecisionserver/ods/internal/api/http"
	"github.com/opendecisionserver/ods/internal/decision"
	"github.com/opendecisionserver/ods/internal/engine/chinese"
	"github.com/opendecisionserver/ods/internal/engine/d1"
	"github.com/opendecisionserver/ods/internal/engine/julia"
)

const defaultEngine = "d1-2b"

func listenAll(addresses string) ([]net.Listener, error) {
	var listeners []net.Listener
	for _, address := range strings.Split(addresses, ",") {
		address = strings.TrimSpace(address)
		var listener net.Listener
		var err error
		if address == "" {
			err = fmt.Errorf("empty HTTP listen address")
		} else {
			listener, err = net.Listen("tcp", address)
		}
		if err != nil {
			for _, l := range listeners {
				_ = l.Close()
			}
			return nil, err
		}
		listeners = append(listeners, listener)
	}
	return listeners, nil
}

func nativePaths(engine, model, library string) (string, string) {
	if model == "" {
		model = "models/chinese-decision-2b/chinese-decision-2b-Q8_0.gguf"
		if engine == "d1-2b" {
			model = "dist/liquid-d1/models/d1-2B-Q8_0.gguf"
		}
	}
	if library == "" {
		library = "lib/darwin-arm64/llama/lib/libllama.dylib"
		if engine == "d1-2b" {
			library = "lib/darwin-arm64/d1/lib/libods_d1.dylib"
		}
	}
	return model, library
}

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
	selected := flag.String("engine", defaultEngine, "inference engine: d1-2b (default), chinese-decision, or julia")
	llamaModel := flag.String("llama-model", "", "pinned GGUF path (default depends on engine)")
	llamaConfig := flag.String("llama-config", "", "Chinese Decision config/tokenizer directory (default: model directory)")
	llamaLibrary := flag.String("llama-lib", "", "pinned native library (default depends on engine)")
	llamaThreads := flag.Int("llama-threads", 4, "native CPU helper threads")
	llamaWaiting := flag.Int("llama-max-waiting", 32, "native waiting requests")
	llamaTokens := flag.Int("llama-max-tokens", 65536, "compiled token limit: per request for d1, total reserved for Chinese")
	addr := flag.String("http", ":8080", "HTTP listen address(es), comma separated; all share one model/runtime")
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
	if *threads < 0 || *maxRuns < 1 || *maxWaiting < 0 || *maxQuestions < 1 || (*selected != "julia" && *selected != "chinese-decision" && *selected != "d1-2b") || (*selected != "julia" && (*maxQuestions > 64 || *llamaThreads < 1 || *llamaWaiting < 0 || *llamaTokens < 1)) {
		return fmt.Errorf("invalid configuration")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	api := &httpapi.Server{MaxRequestBytes: 1 << 20, MaxQuestions: *maxQuestions, Logger: logger, LogRequests: *logRequests}
	gate := newDrainHandler(api.Handler())
	httpServer := &http.Server{Addr: *addr, Handler: gate, ReadHeaderTimeout: 5 * time.Second}
	listeners, err := listenAll(*addr)
	if err != nil {
		return err
	}
	serveErrors := make(chan error, len(listeners))
	for _, listener := range listeners {
		go func(l net.Listener) { serveErrors <- httpServer.Serve(l) }(listener)
	}
	defer func() {
		for _, l := range listeners {
			_ = l.Close()
		}
	}()
	// This also covers startup failures while readiness is still false.
	defer httpServer.Close()
	var engine runtimeEngine
	modelInfo := decision.JuliaModel()
	var admission *decision.Admission
	*llamaModel, *llamaLibrary = nativePaths(*selected, *llamaModel, *llamaLibrary)
	if *selected == "d1-2b" {
		modelInfo = decision.D1Model()
		engine, err = d1.New(d1.Options{ModelPath: *llamaModel, LibraryPath: *llamaLibrary, Threads: *llamaThreads, MaxWaiting: *llamaWaiting, MaxRequestTokens: *llamaTokens})
	} else if *selected == "chinese-decision" {
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
