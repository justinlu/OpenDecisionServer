package julia

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sync/atomic"

	ort "github.com/microsoft/onnxruntime/go/onnxruntime"
	"github.com/opendecisionserver/ods/internal/decision"
)

type Engine struct {
	encoder       *Encoder
	session       *ort.Session
	runs          atomic.Int64
	activeRuns    atomic.Int64
	maxActiveRuns atomic.Int64
}

type Options struct {
	ModelPath      string
	TokenizerPath  string
	LibraryPath    string
	IntraOpThreads int
	MaxLength      int
	HeadLength     int
}

func New(opts Options) (*Engine, error) {
	encoder, err := NewEncoder(opts.TokenizerPath, opts.MaxLength, opts.HeadLength)
	if err != nil {
		return nil, fmt.Errorf("load tokenizer: %w", err)
	}
	ort.SetSharedLibraryPath(opts.LibraryPath)
	if err := ort.Init(); err != nil {
		return nil, fmt.Errorf("initialize ONNX Runtime: %w", err)
	}
	loaded := false
	defer func() {
		if !loaded {
			_ = ort.Shutdown()
		}
	}()
	if err := ort.DisableTelemetry(); err != nil {
		return nil, fmt.Errorf("disable ONNX Runtime telemetry: %w", err)
	}
	so, err := ort.NewSessionOptions()
	if err != nil {
		return nil, err
	}
	defer so.Close()
	if err := so.SetIntraOpNumThreads(opts.IntraOpThreads); err != nil {
		return nil, err
	}
	if err := so.SetGraphOptimizationLevel(ort.GraphOptimizationLevelAll); err != nil {
		return nil, err
	}
	session, err := ort.NewSession(opts.ModelPath, so)
	if err != nil {
		return nil, fmt.Errorf("load Julia model: %w", err)
	}
	loaded = true
	return &Engine{encoder: encoder, session: session}, nil
}

func (e *Engine) Close() error {
	if err := e.session.Close(); err != nil {
		return err
	}
	return ort.Shutdown()
}

func (e *Engine) RunCount() int64      { return e.runs.Load() }
func (e *Engine) ActiveRuns() int64    { return e.activeRuns.Load() }
func (e *Engine) MaxActiveRuns() int64 { return e.maxActiveRuns.Load() }

func (e *Engine) Warmup(ctx context.Context) error {
	state := json.RawMessage(`"The customer needs help now."`)
	rows := []decision.Input{
		{Type: decision.Noul, State: state, Question: "Does this need attention?", Options: []decision.Option{{Text: "false"}, {Text: "true"}}},
		{Type: decision.Choice, State: state, Question: "Which team?", Options: []decision.Option{{Text: "Support"}, {Text: "Billing"}}},
		{Type: decision.Score, State: state, Question: "How urgent?", Options: []decision.Option{{Text: "Can wait"}, {Text: "Urgent"}}},
	}
	_, err := e.Decide(ctx, rows)
	return err
}

func (e *Engine) Decide(ctx context.Context, rows []decision.Input) ([]decision.Result, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	enc := make([]Encoded, len(rows))
	length, count := 0, 0
	for i, row := range rows {
		item, err := e.encoder.Encode(row)
		if err != nil {
			return nil, decision.FieldError{Location: []any{"body", "questions", row.ID}, Message: err.Error(), Type: "value_error"}
		}
		enc[i] = item
		if len(item.IDs) > length {
			length = len(item.IDs)
		}
		if len(item.Markers) > count {
			count = len(item.Markers)
		}
	}
	length = (length + 7) / 8 * 8
	batch := len(rows)
	ids := make([]int64, batch*length)
	attention := make([]int64, batch*length)
	positions := make([]int64, batch*count)
	mask := make([]bool, batch*count)
	qtype := make([]int64, batch)
	for i, item := range enc {
		copy(ids[i*length:], item.IDs)
		for j := range item.IDs {
			attention[i*length+j] = 1
		}
		copy(positions[i*count:], item.Markers)
		for j := range item.Markers {
			mask[i*count+j] = true
		}
		qtype[i] = item.QType
	}
	inputs := make(map[string]*ort.Tensor, 5)
	defer func() {
		for _, t := range inputs {
			_ = t.Close()
		}
	}()
	create := func(name string, shape []int64, data any) error {
		var t *ort.Tensor
		var err error
		switch v := data.(type) {
		case []int64:
			t, err = ort.CreateTensor(shape, v)
		case []bool:
			t, err = ort.CreateTensor(shape, v)
		default:
			return fmt.Errorf("unsupported tensor type")
		}
		if err == nil {
			inputs[name] = t
		}
		return err
	}
	shape := []int64{int64(batch), int64(length)}
	markerShape := []int64{int64(batch), int64(count)}
	if err := create("input_ids", shape, ids); err != nil {
		return nil, err
	}
	if err := create("attention_mask", shape, attention); err != nil {
		return nil, err
	}
	if err := create("marker_pos", markerShape, positions); err != nil {
		return nil, err
	}
	if err := create("marker_mask", markerShape, mask); err != nil {
		return nil, err
	}
	if err := create("qtype", []int64{int64(batch)}, qtype); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	e.runs.Add(1)
	active := e.activeRuns.Add(1)
	defer e.activeRuns.Add(-1)
	for {
		old := e.maxActiveRuns.Load()
		if active <= old || e.maxActiveRuns.CompareAndSwap(old, active) {
			break
		}
	}
	outputs, err := e.session.Run(ctx, inputs, []string{"logits"})
	if err != nil {
		return nil, err
	}
	defer func() {
		for _, t := range outputs {
			_ = t.Close()
		}
	}()
	logits, err := ort.TensorData[float32](outputs["logits"])
	if err != nil {
		return nil, err
	}
	if len(logits) < batch*count {
		return nil, fmt.Errorf("invalid logits shape")
	}
	results := make([]decision.Result, batch)
	for i, item := range enc {
		n := len(item.Markers)
		values := append([]float32(nil), logits[i*count:i*count+n]...)
		probs, winner := softmax(values)
		results[i] = decision.Result{Logits: values, Probabilities: probs, Winner: winner, InputTokens: len(item.IDs)}
	}
	return results, nil
}

func softmax(logits []float32) ([]float64, int) {
	peak := float64(logits[0])
	winner := 0
	for i := 1; i < len(logits); i++ {
		if float64(logits[i]) > peak {
			peak = float64(logits[i])
			winner = i
		}
	}
	probs := make([]float64, len(logits))
	sum := 0.0
	for i, v := range logits {
		probs[i] = math.Exp(float64(v) - peak)
		sum += probs[i]
	}
	for i := range probs {
		probs[i] /= sum
	}
	return probs, winner
}
