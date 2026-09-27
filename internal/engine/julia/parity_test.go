package julia

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/opendecisionserver/ods/internal/decision"
)

type parityCase struct {
	Request struct {
		State    json.RawMessage       `json:"state"`
		Question string                `json:"question"`
		Options  []string              `json:"options"`
		Type     decision.QuestionType `json:"type"`
	} `json:"request"`
	PytorchLogits []float64 `json:"pytorch_logits"`
}

func TestOfficialONNXParity(t *testing.T) {
	if os.Getenv("ODS_PARITY") != "1" {
		t.Skip("set ODS_PARITY=1 for full ONNX parity")
	}
	root := filepath.Join("..", "..", "..")
	model := filepath.Join(root, "models", "julia-1", "model.onnx")
	tokenizer := filepath.Join(root, "models", "julia-1", "tokenizer.json")
	fixture := filepath.Join(root, "models", "julia-1", "parity-cases.json")
	lib := filepath.Join(root, "lib", "darwin-arm64", "libonnxruntime.dylib")
	if runtime.GOOS == "linux" {
		lib = filepath.Join(root, "lib", "linux-amd64", "libonnxruntime.so")
	}
	for _, p := range []string{model, tokenizer, fixture, lib} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("missing parity asset %s: %v", p, err)
		}
	}
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	var cases []parityCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	absLib, err := filepath.Abs(lib)
	if err != nil {
		t.Fatal(err)
	}
	engine, err := New(Options{ModelPath: model, TokenizerPath: tokenizer, LibraryPath: absLib, IntraOpThreads: 4, MaxLength: 1024, HeadLength: 256})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	matched := 0
	for offset := 0; offset < len(cases); offset += 4 {
		end := min(offset+4, len(cases))
		rows := make([]decision.Input, 0, end-offset)
		for _, tc := range cases[offset:end] {
			row := decision.Input{Type: tc.Request.Type, State: tc.Request.State, Question: tc.Request.Question}
			for _, opt := range tc.Request.Options {
				row.Options = append(row.Options, decision.Option{Text: opt})
			}
			rows = append(rows, row)
		}
		got, err := engine.Decide(context.Background(), rows)
		if err != nil {
			t.Fatalf("batch %d: %v", offset, err)
		}
		for j, result := range got {
			tc := cases[offset+j]
			if len(result.Logits) != len(tc.PytorchLogits) {
				t.Fatalf("case %d: logits count", offset+j)
			}
			wantWinner := 0
			for k, v := range tc.PytorchLogits {
				if math.Abs(float64(result.Logits[k])-v) > 1e-2 {
					t.Errorf("case %d option %d: got %f want %f", offset+j, k, result.Logits[k], v)
				}
				if v > tc.PytorchLogits[wantWinner] {
					wantWinner = k
				}
			}
			if result.Winner == wantWinner {
				matched++
			} else {
				t.Errorf("case %d: winner %d want %d", offset+j, result.Winner, wantWinner)
			}
		}
	}
	if matched != len(cases) {
		t.Fatalf("matched %d/%d", matched, len(cases))
	}
	t.Logf("matched %d/%d; Session.Run calls %d", matched, len(cases), engine.RunCount())
}
