package decision

import (
	"encoding/json"
	"testing"
)

func TestParsePreservesChoiceOrderAndStructuredContent(t *testing.T) {
	req, err := ParseRequest([]byte(`{
		"state":{"customer":"VIP","attempts":3},"model":"jev-latest",
		"questions":{"route":{"type":"choice","instructions":{"task":"Route"},
		"criteria":{"technical":{"when":"broken"},"billing":null,"sales":"Purchase"}},
		"urgent":{"type":"noul","criteria":{"true":"Immediate","false":"Can wait"}}}}`), 64)
	if err != nil {
		t.Fatal(err)
	}
	if len(req.Questions) != 2 {
		t.Fatalf("questions: %d", len(req.Questions))
	}
	route := req.Questions[0]
	if route.ID != "route" || route.Question != `{"task":"Route"}` {
		t.Fatalf("route: %+v", route)
	}
	if route.Options[0].Key != "technical" || route.Options[1].Key != "billing" || route.Options[2].Key != "sales" {
		t.Fatalf("option order: %+v", route.Options)
	}
	if route.Options[1].Text != "billing" {
		t.Fatalf("null description: %+v", route.Options[1])
	}
	if req.Questions[1].Options[0].Text != "Can wait" || req.Questions[1].Options[1].Text != "Immediate" {
		t.Fatalf("noul order: %+v", req.Questions[1].Options)
	}
}

func TestRejectsUnsupportedChoiceSize(t *testing.T) {
	criteria := make(map[string]string)
	for i := 0; i < 21; i++ {
		criteria[string(rune('a'+i))] = "option"
	}
	b, _ := json.Marshal(map[string]any{"state": "x", "model": "jev-latest", "questions": map[string]any{"q": map[string]any{"type": "choice", "criteria": criteria}}})
	_, err := ParseRequest(b, 64)
	if err == nil {
		t.Fatal("expected 422 validation error")
	}
	field, ok := err.(FieldError)
	if !ok || field.Location[3] != "criteria" {
		t.Fatalf("unexpected error: %v", err)
	}
}
