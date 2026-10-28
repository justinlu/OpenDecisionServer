//go:build llamavalidation

// Standalone experimental validation; deliberately excluded from the ODS binary.
package main

/*
#include <stdlib.h>
#include <string.h>
#include "llama.h"

static struct llama_context * probe_context(struct llama_model * model, int sequences, int ubatch) {
    struct llama_context_params p = llama_context_default_params();
    p.n_ctx = 16384 * sequences;
    p.n_seq_max = sequences;
    p.n_batch = sequences == 1 ? 512 : 8192;
    p.n_ubatch = ubatch;
    p.n_threads = 4;
    p.n_threads_batch = 4;
    p.flash_attn_type = LLAMA_FLASH_ATTN_TYPE_DISABLED;
    p.embeddings = false;
    p.offload_kqv = true;
    p.no_perf = false;
    return llama_init_from_model(model, p);
}

static struct llama_model * probe_model(const char * path) {
    struct llama_model_params p = llama_model_default_params();
    p.n_gpu_layers = -1;
    return llama_model_load_from_file(path, p);
}

// All slices passed from Go contain scalars only and are borrowed during this
// synchronous call. llama_batch owns its native arrays; logits are copied before
// the next decode. A context has one caller throughout this experiment.
static int probe_run(struct llama_context * ctx, const int32_t * tokens,
                     const int32_t * lengths, int count, const int32_t * letters,
                     const int32_t * candidates, float * out) {
    int total = 0;
    if (count < 1 || count > (int) llama_n_seq_max(ctx)) return -100;
    for (int s = 0; s < count; ++s) {
        if (lengths[s] < 1 || lengths[s] > (int) llama_n_ctx_seq(ctx)
            || candidates[s] < 2 || candidates[s] > 26) return -101;
        total += lengths[s];
    }
    if (count > 1 && total > (int) llama_n_batch(ctx)) return -102;
    llama_memory_clear(llama_get_memory(ctx), true);
    int capacity = count == 1 && total > (int) llama_n_batch(ctx) ? (int) llama_n_batch(ctx) : total;
    struct llama_batch b = llama_batch_init(capacity, 0, 1);
    int finals[4];
    int offset = 0;
    int rc = 0;
    if (count == 1) {
        while (offset < total) {
            b.n_tokens = total - offset < capacity ? total - offset : capacity;
            for (int i = 0; i < b.n_tokens; ++i) {
                b.token[i] = tokens[offset+i]; b.pos[i] = offset+i;
                b.n_seq_id[i] = 1; b.seq_id[i][0] = 0;
                b.logits[i] = offset+i == total-1;
            }
            rc = llama_decode(ctx, b);
            if (rc != 0) goto done;
            finals[0] = b.n_tokens-1;
            offset += b.n_tokens;
        }
    } else {
        for (int s = 0; s < count; ++s) {
            for (int p = 0; p < lengths[s]; ++p) {
                b.token[offset] = tokens[offset]; b.pos[offset] = p;
                b.n_seq_id[offset] = 1; b.seq_id[offset][0] = s;
                b.logits[offset] = p == lengths[s]-1;
                ++offset;
            }
            finals[s] = offset-1;
        }
        b.n_tokens = total;
        rc = llama_decode(ctx, b);
        if (rc != 0) goto done;
    }
    for (int s = 0; s < count; ++s) {
        // Positive getter indices refer to original batch token indices, not
        // compact output rows. This getter also synchronizes the Metal work.
        const float * logits = llama_get_logits_ith(ctx, finals[s]);
        if (!logits) { rc = -103; goto done; }
        for (int j = 0; j < candidates[s]; ++j) out[s*26+j] = logits[letters[j]];
    }
done:
    if (rc != 0) {
        llama_synchronize(ctx);
        llama_memory_clear(llama_get_memory(ctx), true);
    }
    llama_batch_free(b);
    return rc;
}
*/
import "C"

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"reflect"
	"sort"
	"strings"
	"time"
	"unsafe"
)

type row struct {
	ID        string          `json:"id"`
	CaseID    string          `json:"case_id"`
	Suite     string          `json:"suite"`
	History   int             `json:"history"`
	GroupSize int             `json:"group_size"`
	State     json.RawMessage `json:"state"`
	Question  json.RawMessage `json:"question"`
	Options   [][2]string     `json:"options"`
	Prompt    string          `json:"prompt"`
	Tokens    []int32         `json:"tokens"`
	Expected  []float64       `json:"expected_probabilities"`
}
type reference struct {
	Temperature float64 `json:"temperature"`
	Letters     []int32 `json:"letters"`
	Rows        []row   `json:"rows"`
}
type metric struct {
	Samples int     `json:"samples"`
	P50     float64 `json:"p50_ms"`
	P95     float64 `json:"p95_ms"`
	Max     float64 `json:"max_ms"`
}

func metrics(values []float64) metric {
	v := append([]float64(nil), values...)
	sort.Float64s(v)
	return metric{len(v), v[int(math.Ceil(.50*float64(len(v))))-1], v[int(math.Ceil(.95*float64(len(v))))-1], v[len(v)-1]}
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
func nativeTokens(model *C.struct_llama_model, s string) []int32 {
	text := C.CString(s)
	defer C.free(unsafe.Pointer(text))
	vocab := C.llama_model_get_vocab(model)
	n := C.llama_tokenize(vocab, text, C.int32_t(len(s)), nil, 0, false, true)
	if n >= 0 {
		panic("tokenizer size query failed")
	}
	tokens := make([]int32, int(-n))
	n = C.llama_tokenize(vocab, text, C.int32_t(len(s)), (*C.int32_t)(unsafe.Pointer(&tokens[0])), C.int32_t(len(tokens)), false, true)
	if int(n) != len(tokens) {
		panic("tokenizer length mismatch")
	}
	return tokens
}
func run(ctx *C.struct_llama_context, rows []row, ref reference) ([][]float64, [][]float32) {
	var tokens, lengths, candidates []int32
	for _, r := range rows {
		tokens = append(tokens, r.Tokens...)
		lengths = append(lengths, int32(len(r.Tokens)))
		candidates = append(candidates, int32(len(r.Options)))
	}
	out := make([]float32, len(rows)*26)
	rc := C.probe_run(ctx, (*C.int32_t)(unsafe.Pointer(&tokens[0])), (*C.int32_t)(unsafe.Pointer(&lengths[0])), C.int(len(rows)), (*C.int32_t)(unsafe.Pointer(&ref.Letters[0])), (*C.int32_t)(unsafe.Pointer(&candidates[0])), (*C.float)(unsafe.Pointer(&out[0])))
	if rc != 0 {
		panic(fmt.Sprintf("native decode %d for %s", int(rc), rows[0].ID))
	}
	probs := make([][]float64, len(rows))
	logits := make([][]float32, len(rows))
	for s, r := range rows {
		logits[s] = append([]float32(nil), out[s*26:s*26+len(r.Options)]...)
		values := make([]float64, len(r.Options))
		max := math.Inf(-1)
		for i, l := range logits[s] {
			if math.IsNaN(float64(l)) || math.IsInf(float64(l), 0) {
				panic("nonfinite logit")
			}
			values[i] = float64(l) / ref.Temperature
			if values[i] > max {
				max = values[i]
			}
		}
		total := 0.
		for i := range values {
			values[i] = math.Exp(values[i] - max)
			total += values[i]
		}
		for i := range values {
			values[i] /= total
			if math.IsNaN(values[i]) || math.IsInf(values[i], 0) {
				panic("nonfinite probability")
			}
		}
		probs[s] = values
	}
	return probs, logits
}
func winner(p []float64) int {
	j := 0
	for i := range p {
		if p[i] > p[j] {
			j = i
		}
	}
	return j
}
func delta(a, b []float64) float64 {
	if len(a) != len(b) {
		panic("probability length mismatch")
	}
	d := 0.
	for i := range a {
		d = math.Max(d, math.Abs(a[i]-b[i]))
	}
	return d
}
func renderString(r row) (string, bool) {
	var state, question string
	if json.Unmarshal(r.State, &state) != nil || json.Unmarshal(r.Question, &question) != nil || string(r.State) == "null" || string(r.Question) == "null" {
		return "", false
	}
	var options []string
	for i, o := range r.Options {
		options = append(options, fmt.Sprintf("%c: %s — %s", 'A'+i, o[0], o[1]))
	}
	return "<|im_start|>user\n待判断的上下文：\n" + state + "\n\n问题：\n" + question + "\n\n候选项：\n" + strings.Join(options, "\n") + "\n只返回候选项对应的字母。<|im_end|>\n<|im_start|>assistant\n<think>\n\n</think>\n\n", true
}
func main() {
	referencePath := flag.String("reference", "", "Python references")
	modelPath := flag.String("model", "", "GGUF")
	output := flag.String("output", "", "output directory (must not exist)")
	samples := flag.Int("samples", 120, "measured iterations per native latency group")
	ubatch := flag.Int("ubatch", 512, "physical microbatch size for multi-sequence context")
	serialOnly := flag.Bool("serial-only", false, "validate the production serial path without the experimental multi-sequence path")
	flag.Parse()
	if *samples < 1 || *ubatch < 4 || *referencePath == "" || *modelPath == "" || *output == "" {
		panic("invalid flags")
	}
	data, err := os.ReadFile(*referencePath)
	must(err)
	var ref reference
	must(json.Unmarshal(data, &ref))
	if len(ref.Letters) != 26 || !(ref.Temperature > 0) || math.IsInf(ref.Temperature, 0) {
		panic("invalid reference")
	}
	must(os.Mkdir(*output, 0755))
	stream, err := os.Create(*output + "/parity.jsonl")
	must(err)
	defer stream.Close()
	enc := json.NewEncoder(stream)
	C.llama_backend_init()
	defer C.llama_backend_free()
	path := C.CString(*modelPath)
	defer C.free(unsafe.Pointer(path))
	model := C.probe_model(path)
	if model == nil {
		panic("model load failed")
	}
	defer C.llama_model_free(model)
	summary := map[string]any{"gpu_offload_supported": bool(C.llama_supports_gpu_offload()), "temperature": ref.Temperature, "samples_per_latency_group": *samples, "ubatch_requested": *ubatch, "probability_tolerance": 1e-4, "required_winner_agreement": 1.0}
	rendered, skipped := 0, 0
	for i, l := range ref.Letters {
		if !reflect.DeepEqual(nativeTokens(model, string(rune('A'+i))), []int32{l}) {
			panic("letter token mismatch")
		}
	}
	for _, r := range ref.Rows {
		if !reflect.DeepEqual(nativeTokens(model, r.Prompt), r.Tokens) {
			panic("tokenizer mismatch " + r.ID)
		}
		if actual, ok := renderString(r); ok {
			if actual != r.Prompt {
				panic("prompt mismatch " + r.ID)
			}
			rendered++
		} else {
			skipped++
		}
	}
	summary["tokenizer_rows"] = len(ref.Rows)
	summary["go_string_render_rows"] = rendered
	summary["structured_render_not_implemented"] = skipped
	fmt.Println("prompt and token checks passed")
	ctx := C.probe_context(model, 1, 512)
	if ctx == nil {
		panic("serial context failed")
	}
	summary["serial_context"] = map[string]any{"n_ctx": int(C.llama_n_ctx(ctx)), "n_ctx_seq": int(C.llama_n_ctx_seq(ctx)), "n_batch": int(C.llama_n_batch(ctx)), "n_ubatch": int(C.llama_n_ubatch(ctx)), "n_seq_max": int(C.llama_n_seq_max(ctx))}
	serial := map[string][]float64{}
	serialLogits := map[string][]float32{}
	oldMax, oldMismatch, oldCount := 0., 0, 0
	for i, r := range ref.Rows {
		p, l := run(ctx, []row{r}, ref)
		serial[r.ID] = p[0]
		serialLogits[r.ID] = l[0]
		if len(r.Expected) > 0 {
			oldCount++
			oldMax = math.Max(oldMax, delta(p[0], r.Expected))
			if winner(p[0]) != winner(r.Expected) {
				oldMismatch++
			}
		}
		must(enc.Encode(map[string]any{"phase": "serial", "id": r.ID, "probabilities": p[0], "logits": l[0]}))
		if i%200 == 0 {
			fmt.Println("serial", i)
		}
	}
	summary["python_reference_rows"] = oldCount
	summary["python_max_probability_delta"] = oldMax
	summary["python_winner_mismatches"] = oldMismatch
	// A/B/A explicitly exercises hybrid memory clearing between unrelated jobs.
	first := ref.Rows[0]
	between, betweenLogits := run(ctx, []row{ref.Rows[len(ref.Rows)-1]}, ref)
	again, againLogits := run(ctx, []row{first}, ref)
	if *serialOnly {
		must(enc.Encode(map[string]any{"phase": "aba-between", "id": ref.Rows[len(ref.Rows)-1].ID, "probabilities": between[0], "logits": betweenLogits[0]}))
		must(enc.Encode(map[string]any{"phase": "aba-repeat", "id": first.ID, "probabilities": again[0], "logits": againLogits[0]}))
	}
	summary["serial_aba_max_probability_delta"] = delta(serial[first.ID], again[0])
	// Latency is synchronous native completion + result copy/softmax. Prompts and
	// token IDs are prepared outside the timer; HTTP and scheduling are absent.
	groups := map[string][]row{}
	var keys []string
	for _, r := range ref.Rows {
		if r.Suite == "voice" {
			if _, ok := groups[r.CaseID]; !ok {
				keys = append(keys, r.CaseID)
			}
			groups[r.CaseID] = append(groups[r.CaseID], r)
		}
	}
	samplesFile, err := os.Create(*output + "/latency.jsonl")
	must(err)
	defer samplesFile.Close()
	sampleEncoder := json.NewEncoder(samplesFile)
	latency := map[string]any{}
	bench := func(context *C.struct_llama_context, mode string, batched bool) {
		for _, history := range []int{0, 4, 12} {
			for _, size := range []int{1, 3} {
				var workloads [][]row
				for _, k := range keys {
					rows := groups[k]
					if rows[0].History == history && len(rows) == size {
						workloads = append(workloads, rows)
					}
				}
				if len(workloads) != 3 {
					panic("unexpected voice workload")
				}
				var times []float64
				for i := -6; i < *samples; i++ {
					rows := workloads[(i+6)%len(workloads)]
					start := time.Now()
					if batched {
						run(context, rows, ref)
					} else {
						for _, r := range rows {
							run(context, []row{r}, ref)
						}
					}
					ms := float64(time.Since(start).Nanoseconds()) / 1e6
					if i >= 0 {
						times = append(times, ms)
						must(sampleEncoder.Encode(map[string]any{"mode": mode, "history": history, "questions": size, "case_id": rows[0].CaseID, "iteration": i, "latency_ms": ms}))
					}
				}
				name := fmt.Sprintf("%s/h%d/q%d", mode, history, size)
				latency[name] = metrics(times)
				fmt.Println(name, metrics(times))
			}
		}
	}
	bench(ctx, "serial", false)
	C.llama_free(ctx)
	if *serialOnly {
		summary["serial_only"] = true
		summary["latency"] = latency
		summary["parity_pass"] = oldMismatch == 0 && oldMax <= 1e-4 && summary["serial_aba_max_probability_delta"].(float64) <= 1e-4
		data, err = json.MarshalIndent(summary, "", "  ")
		must(err)
		must(os.WriteFile(*output+"/summary.json", append(data, '\n'), 0644))
		fmt.Println(string(data))
		return
	}
	ctx = C.probe_context(model, 4, C.int(*ubatch))
	if ctx == nil {
		panic("batch context failed")
	}
	defer C.llama_free(ctx)
	summary["batch_context"] = map[string]any{"n_ctx": int(C.llama_n_ctx(ctx)), "n_ctx_seq": int(C.llama_n_ctx_seq(ctx)), "n_batch": int(C.llama_n_batch(ctx)), "n_ubatch": int(C.llama_n_ubatch(ctx)), "n_seq_max": int(C.llama_n_seq_max(ctx))}
	batchMax, batchLogitMax, batchMismatch, batchCount := 0., 0., 0, 0
	// Consecutive groups with alternating sizes exercise uneven prompt lengths,
	// different sequence IDs, and per-job state clearing across all test rows.
	for start, batchIndex := 0, 0; start < len(ref.Rows); batchIndex++ {
		size := 2 + batchIndex%3
		if start+size > len(ref.Rows) {
			size = len(ref.Rows) - start
		}
		rows := ref.Rows[start : start+size]
		p, l := run(ctx, rows, ref)
		for i, r := range rows {
			batchCount++
			d := delta(serial[r.ID], p[i])
			batchMax = math.Max(batchMax, d)
			for j := range l[i] {
				batchLogitMax = math.Max(batchLogitMax, math.Abs(float64(l[i][j]-serialLogits[r.ID][j])))
			}
			if winner(serial[r.ID]) != winner(p[i]) {
				batchMismatch++
			}
			must(enc.Encode(map[string]any{"phase": "batch", "id": r.ID, "batch_size": size, "probabilities": p[i], "logits": l[i], "max_probability_delta": d}))
		}
		start += size
	}
	// Nonconsecutive order checks output-index mapping and repeated A in two slots.
	repeated := []row{ref.Rows[0], ref.Rows[80], ref.Rows[len(ref.Rows)-1], ref.Rows[0]}
	p, _ := run(ctx, repeated, ref)
	permutationMax := 0.
	for i, r := range repeated {
		permutationMax = math.Max(permutationMax, delta(serial[r.ID], p[i]))
	}
	summary["batch_rows"] = batchCount
	summary["batch_max_probability_delta"] = batchMax
	summary["batch_max_logit_delta"] = batchLogitMax
	summary["batch_winner_mismatches"] = batchMismatch
	summary["repeated_permutation_max_probability_delta"] = permutationMax
	bench(ctx, "batch", true)
	summary["latency"] = latency
	summary["parity_pass"] = oldMismatch == 0 && oldMax <= 1e-4 && batchMismatch == 0 && batchMax <= 1e-4 && permutationMax <= 1e-4 && summary["serial_aba_max_probability_delta"].(float64) <= 1e-4
	data, err = json.MarshalIndent(summary, "", "  ")
	must(err)
	must(os.WriteFile(*output+"/summary.json", append(data, '\n'), 0644))
	fmt.Println(string(data))
}
