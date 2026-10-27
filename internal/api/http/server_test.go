package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/opendecisionserver/ods/internal/decision"
)

type fixedEngine struct{}

func (fixedEngine) Decide(_ context.Context, rows []decision.Input) ([]decision.Result, error) {
	out := make([]decision.Result, len(rows))
	for i, row := range rows {
		probs := make([]float64, len(row.Options))
		for j := range probs {
			probs[j] = 1 / float64(len(probs))
		}
		out[i] = decision.Result{Probabilities: probs, Winner: 0, InputTokens: 4}
	}
	return out, nil
}

func TestSystemOneWireContract(t *testing.T) {
	api := &Server{Service: &decision.Service{Engine: fixedEngine{}, Admission: decision.NewAdmission(4, 128)}, MaxQuestions: 64}
	api.SetReady(true)
	body := []byte(`{"model":"jev-latest","state":{"message":"help"},"questions":{"urgent":{"type":"noul"},"route":{"type":"choice","criteria":{"billing":"Payment","support":"Help"}},"score":{"type":"score","criteria":["low","high"]}}}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/systemone", bytes.NewReader(body))
	api.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["model"] != "julia-1" {
		t.Fatalf("model: %v", response["model"])
	}
	answers := response["answers"].(map[string]any)
	if len(answers) != 3 {
		t.Fatalf("answers: %v", answers)
	}
	if answers["urgent"].(map[string]any)["noul"] != 0.5 {
		t.Fatalf("noul: %v", answers["urgent"])
	}
	if answers["route"].(map[string]any)["choice"] != "billing" {
		t.Fatalf("choice: %v", answers["route"])
	}
	if answers["score"].(map[string]any)["score"] != 0.5 {
		t.Fatalf("score: %v", answers["score"])
	}
	if response["usage"].(map[string]any)["input_tokens"] != float64(12) {
		t.Fatalf("usage: %v", response["usage"])
	}
}

func TestInvalidQuestionReturns422(t *testing.T) {
	api := &Server{Service: &decision.Service{Engine: fixedEngine{}, Admission: decision.NewAdmission(4, 128)}, MaxQuestions: 64}
	api.SetReady(true)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/systemone", bytes.NewBufferString(`{"model":"jev-latest","state":"x","questions":{"q":{"type":"choice","criteria":{"only":"one"}}}}`))
	api.Handler().ServeHTTP(w, r)
	if w.Code != 422 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Detail []decision.FieldError `json:"detail"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Detail) != 1 || len(body.Detail[0].Location) != 4 {
		t.Fatalf("detail: %+v", body.Detail)
	}
}

func TestHTTPStatusContract(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		ready   bool
		maxSize int64
		want    int
	}{
		{"malformed JSON", `{`, true, 0, 400},
		{"unknown model", `{"model":"missing","state":"x","questions":{"q":{"type":"noul"}}}`, true, 0, 404},
		{"loading", `{"model":"jev-latest","state":"x","questions":{"q":{"type":"noul"}}}`, false, 0, 503},
		{"oversized", `{"model":"jev-latest","state":"x","questions":{"q":{"type":"noul"}}}`, true, 8, 413},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			api := &Server{Service: &decision.Service{Engine: fixedEngine{}, Admission: decision.NewAdmission(4, 128)}, MaxQuestions: 64, MaxRequestBytes: tc.maxSize}
			api.SetReady(tc.ready)
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/v1/systemone", bytes.NewBufferString(tc.body))
			api.Handler().ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d, want %d: %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

func TestChineseModelPolicyAndResponse(t *testing.T) {
	api := &Server{Service: &decision.Service{Model: decision.ChineseModel(), Engine: fixedEngine{}}, MaxQuestions: 64}
	api.SetReady(true)
	handler := api.Handler()
	models := httptest.NewRecorder()
	handler.ServeHTTP(models, httptest.NewRequest("GET", "/v1/models", nil))
	if !bytes.Contains(models.Body.Bytes(), []byte("chinese-decision-2b")) || bytes.Contains(models.Body.Bytes(), []byte("julia-1")) {
		t.Fatal(models.Body.String())
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("POST", "/v1/systemone", bytes.NewBufferString(`{"model":"chinese-decision-2b","state":null,"questions":{"q":{"type":"noul","instructions":"是否？"}}}`)))
	if w.Code != 200 || !bytes.Contains(w.Body.Bytes(), []byte(`"probabilities":{"false":0.5,"true":0.5}`)) {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	api.SetReady(false)
	models = httptest.NewRecorder()
	handler.ServeHTTP(models, httptest.NewRequest("GET", "/v1/models", nil))
	if bytes.Contains(models.Body.Bytes(), []byte("chinese-decision-2b")) {
		t.Fatal("unavailable model advertised")
	}
}

func TestJuliaModelMetadataRemainsCompatible(t *testing.T) {
	api := &Server{Service: &decision.Service{Engine: fixedEngine{}, Admission: decision.NewAdmission(1, 0)}, MaxQuestions: 64}
	api.SetReady(true)
	w := httptest.NewRecorder()
	api.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/v1/models", nil))
	if !bytes.Contains(w.Body.Bytes(), []byte(`"release_date":"2026-09-25"`)) || !bytes.Contains(w.Body.Bytes(), []byte("Local Julia-1 ONNX CPU decision model")) {
		t.Fatal(w.Body.String())
	}
}

func TestChineseTiesPreserveCandidateWireOrder(t *testing.T) {
	api := &Server{Service: &decision.Service{Model: decision.ChineseModel(), Engine: fixedEngine{}}, MaxQuestions: 64}
	api.SetReady(true)
	w := httptest.NewRecorder()
	api.Handler().ServeHTTP(w, httptest.NewRequest("POST", "/v1/systemone", bytes.NewBufferString(`{"model":"chinese-decision-2b","state":"x","questions":{"q":{"type":"choice","instructions":"选择","criteria":{"z":"first","a":"second"}}}}`)))
	var response struct {
		Answers map[string]struct {
			Choice        string
			Probabilities json.RawMessage
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	q := response.Answers["q"]
	if q.Choice != "z" || string(q.Probabilities) != `{"z":0.5,"a":0.5}` {
		t.Fatalf("choice %s, probabilities %s", q.Choice, q.Probabilities)
	}
}
