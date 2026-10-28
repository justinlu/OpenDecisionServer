package main

import (
	"bytes"
	"testing"
)

func TestRequestBodiesUseModelFlag(t *testing.T) {
	for _, body := range requestBodies("d1-2b") {
		if !bytes.Contains(body, []byte(`"model":"d1-2b"`)) {
			t.Fatalf("body missing model: %s", body)
		}
	}
}

func TestValidateExpectsConfiguredModel(t *testing.T) {
	body := requestBodies("d1-2b")[0]
	resp := []byte(`{"model":"d1-2b","answers":{"urgent":{"type":"noul"}},"usage":{"input_tokens":4}}`)
	if err := validate(body, resp, "d1-2b"); err != nil {
		t.Fatal(err)
	}
	if err := validate(body, resp, "julia-1"); err == nil {
		t.Fatal("expected model mismatch error")
	}
}
