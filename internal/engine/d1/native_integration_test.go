//go:build llama && darwin && arm64 && cgo

package d1

import (
	"context"
	"encoding/json"
	"github.com/opendecisionserver/ods/internal/decision"
	"os"
	"testing"
)

func TestNativeD1Text(t *testing.T) {
	if os.Getenv("ODS_D1_NATIVE_TEST") != "1" {
		t.Skip("requires pinned Metal model and bridge")
	}
	e, err := New(Options{ModelPath: "../../../dist/liquid-d1/models/d1-2B-Q8_0.gguf", LibraryPath: "../../../lib/darwin-arm64/d1/lib/libods_d1.dylib", Threads: 4, MaxWaiting: 2, MaxRequestTokens: 65536})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	payload := []byte(`{"state":"我要取现金","questions":{"business":{"type":"choice","instructions":"用户要办理什么业务？","criteria":{"withdraw":"取款","deposit":"存款","transfer":"转账"}}}}`)
	req, err := decision.ParseForModel(payload, 64, decision.D1Model())
	if err != nil {
		t.Fatal(err)
	}
	first, err := e.Decide(context.Background(), req.Questions)
	if err != nil {
		t.Fatal(err)
	}
	req.Questions[0].State = json.RawMessage(`"我想存钱"`)
	if _, err = e.Decide(context.Background(), req.Questions); err != nil {
		t.Fatal(err)
	}
	req.Questions[0].State = json.RawMessage(`"我要取现金"`)
	last, err := e.Decide(context.Background(), req.Questions)
	if err != nil {
		t.Fatal(err)
	}
	if string(first[0].Answer) != string(last[0].Answer) {
		t.Fatal("A/B/A state leaked")
	}
	var answer struct {
		Choice string `json:"choice"`
	}
	json.Unmarshal(first[0].Answer, &answer)
	if answer.Choice != "withdraw" {
		t.Fatalf("%s", first[0].Answer)
	}
}
