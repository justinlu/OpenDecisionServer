//go:build llama && darwin && arm64 && cgo

package chinese

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/opendecisionserver/ods/internal/decision"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMetalGoldenAndLifecycle(t *testing.T) {
	if os.Getenv("ODS_LLAMA_PARITY") != "1" {
		t.Skip("set ODS_LLAMA_PARITY=1 for pinned Metal acceptance")
	}
	e, err := New(Options{ModelPath: os.Getenv("ODS_LLAMA_MODEL"), ConfigDir: os.Getenv("ODS_LLAMA_CONFIG"), LibraryPath: os.Getenv("ODS_LLAMA_LIB"), Threads: 4, MaxWaiting: 32})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	data, err := os.ReadFile("testdata/prompt-goldens.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Request       json.RawMessage
		Prompt        string
		Tokens        []int32
		Probabilities []float64
	}
	if err = json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		req, err := decision.ParseForModel(c.Request, 64, decision.ChineseModel())
		if err != nil {
			t.Fatal(err)
		}
		actual, err := Prompt(req.Questions[0])
		if err != nil || actual != c.Prompt {
			t.Fatalf("prompt mismatch %v", err)
		}
		// No requests are running; the worker is idle and the vocabulary is immutable.
		ids, err := e.native.Tokenize(actual)
		if err != nil || len(ids) != len(c.Tokens) {
			t.Fatalf("token length mismatch %v", err)
		}
		for i, id := range ids {
			if id != c.Tokens[i] {
				t.Fatalf("token %d: %d != %d", i, id, c.Tokens[i])
			}
		}
		out, err := e.Decide(context.Background(), req.Questions)
		if err != nil {
			t.Fatal(err)
		}
		for i, p := range out[0].Probabilities {
			if math.Abs(p-c.Probabilities[i]) > 1e-4 {
				t.Fatalf("probability %g != %g", p, c.Probabilities[i])
			}
		}
	}
	if path := os.Getenv("ODS_LLAMA_REFERENCE"); path != "" {
		checkCorpusTokens(t, e, path)
	}
	first, _ := decision.ParseForModel(cases[0].Request, 64, decision.ChineseModel())
	a, _ := e.Decide(context.Background(), first.Questions)
	other, _ := decision.ParseForModel(cases[len(cases)-1].Request, 64, decision.ChineseModel())
	e.Decide(context.Background(), other.Questions)
	again, err := e.Decide(context.Background(), first.Questions)
	if err != nil {
		t.Fatal(err)
	}
	for i, p := range a[0].Probabilities {
		if p != again[0].Probabilities[i] {
			t.Fatal("A/B/A state changed")
		}
	}
	tooLong := input("long")
	tooLong.State = json.RawMessage(`"` + strings.Repeat("中", 17000) + `"`)
	if _, err := e.Decide(context.Background(), []decision.Input{tooLong}); err == nil {
		t.Fatal("overlength input accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	before := e.RunCount()
	go func() { _, err := e.Decide(ctx, other.Questions); finished <- err }()
	deadline := time.Now().Add(time.Second)
	for e.RunCount() == before && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-finished
	if err = e.Close(); err != nil {
		t.Fatal(err)
	}
	if e.ActiveRuns() != 0 {
		t.Fatal("native not drained")
	}
}

func checkCorpusTokens(t *testing.T, e *Engine, path string) {
	t.Helper()
	var reference struct {
		Rows []struct {
			ID     string  `json:"id"`
			Prompt string  `json:"prompt"`
			Tokens []int32 `json:"tokens"`
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &reference); err != nil {
		t.Fatal(err)
	}
	expected := map[string]int{}
	for i, r := range reference.Rows {
		expected[r.ID] = i
	}
	checked := 0
	for _, name := range []string{"custom", "toksuite"} {
		f, err := os.Open("../../../tests/evaluation/" + name + ".jsonl")
		if err != nil {
			t.Fatal(err)
		}
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 4096), 1<<20)
		for scanner.Scan() {
			var c struct {
				ID      string          `json:"id"`
				Request json.RawMessage `json:"request"`
			}
			if err = json.Unmarshal(scanner.Bytes(), &c); err != nil {
				t.Fatal(err)
			}
			req, err := decision.ParseForModel(c.Request, 64, decision.ChineseModel())
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range req.Questions {
				index, ok := expected[c.ID+":"+row.ID]
				if !ok {
					t.Fatal("reference missing", c.ID)
				}
				want := reference.Rows[index]
				prompt, err := Prompt(row)
				if err != nil || prompt != want.Prompt {
					t.Fatal("corpus prompt mismatch", c.ID, err)
				}
				ids, err := e.native.Tokenize(prompt)
				if err != nil || !reflect.DeepEqual(ids, want.Tokens) {
					t.Fatal("corpus token mismatch", c.ID, err)
				}
				checked++
			}
		}
		if err = scanner.Err(); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
	if checked != 989 {
		t.Fatal("unexpected corpus coverage", checked)
	}
	t.Logf("Corpus prompt/token parity: %d", checked)
}
